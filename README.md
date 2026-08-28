# monux

`monux` switches one DDC/CI monitor between computers from macOS, Linux, or
Windows. The same Go CLI is installed on every machine, and each installation
talks to the monitor locally through its own video connection.

There is no SSH, daemon, network dependency, `ddcutil`, or `m1ddc` runtime
dependency for local CLI use. The optional built-in HTTP server can expose the
same switcher to another computer or an ESP32.

## Topology

```text
macOS + monux ──── USB-C/DisplayPort/HDMI ──┐
Linux + monux ──── DisplayPort/HDMI ────────┼── Monitor
Windows + monux ── DisplayPort/HDMI ────────┘
```

From any installed machine:

```bash
monux switch mac
monux switch linux
monux switch windows
```

Input names are user-defined configuration labels. Only the inputs present in
the configuration are available.

## Native platform backends

| Platform | Detection | DDC transport | External runtime tool |
| --- | --- | --- | --- |
| Linux | DRM/I2C sysfs | `/dev/i2c-*` + DDC/CI frames | None |
| Windows | Win32 monitor enumeration | `Dxva2.dll` VCP API | None |
| macOS | CoreGraphics | CoreDisplay `IOAVService` | None |

The platform-independent package in `internal/ddc` owns DDC/CI frame encoding,
checksums, capabilities reassembly, VCP reply parsing, and VCP code `0x60`.
Platform files only perform discovery and transport.

See [platform support](docs/platform-support.md) for implementation details and
current hardware-validation status.

An optional Omarchy bar plugin lives in
[`plugins/omarchy`](plugins/omarchy). It keeps the CLI as the control
backend and adds current-input status plus click-to-switch behavior to the top
bar.

## Build

Go 1.24 or newer is required to build from source.

Linux and Windows do not require CGO:

```bash
go build -o monux ./cmd/monux
```

macOS uses a small built-in CGO bridge to Apple system frameworks. Install
Apple Command Line Tools, then build on the Mac:

```bash
xcode-select --install
CGO_ENABLED=1 go build -o monux ./cmd/monux
```

No third-party monitor-control application is required after the binary is
built.

## Detect

Run detection separately on each machine:

```bash
monux detect
```

Example Linux output:

```text
15  card0-DP-1 (DELL P2415Q)
```

Example Windows output:

```text
1   Dell P2415Q
```

The first column is the platform-local monitor identifier used by
`monitor.id`.

## Configure

The default per-user configuration path is:

- Linux: `~/.config/monux/config.yaml`
- macOS: `~/Library/Application Support/monux/config.yaml`
- Windows: `%AppData%\monux\config.yaml`

Generate the configuration on each computer while the monitor is showing that
computer:

```bash
monux init
```

`init` detects the monitor's platform-local ID, reads its current VCP input and
capabilities, creates the parent directory, and writes the configuration. It is
safe to rerun: existing input names are preserved, while a changed Linux I2C
bus or other platform-local monitor ID is refreshed automatically.

The current input is named `linux`, `mac`, or `windows` according to the local
platform. Other discovered values are given connector names such as
`displayport-2` and `hdmi-1`. A monitor cannot identify which operating system
is connected to a port, so supply known remote names during initialization:

```bash
monux init --input mac=0x11 --input linux=0x0f
```

Repeat `--input name=value` as needed. With multiple detected monitors, select
one explicitly:

```bash
monux init --monitor 23
```

The resulting file looks like:

```yaml
monitor:
  id: "23"

inputs:
  mac: 0x11
  linux: 0x0f
  windows: 0x10
```

`monitor.id` is local to the operating system, so it may be `23` on Linux and
`1` on macOS or Windows. The named `inputs` section can otherwise be identical
on every machine. Manual editing remains supported.

Override the default path with `--config`/`-c` or `MONUX_CONFIG`.

## Discover input ports

Ask the monitor which VCP `0x60` input values its firmware reports:

```bash
monux inputs
```

When no configuration exists and exactly one monitor is detected, `inputs`
selects it automatically. With multiple monitors, configure `monitor.id` to
make the selection explicit. Configuration is still required for named
commands such as `monux switch mac`.

Example:

```text
VALUE  CONNECTOR      REPORTED  CURRENT  NAME
0x0f   DisplayPort 1  yes       no       linux
0x10   DisplayPort 2  yes       no       windows
0x11   HDMI 1         yes       yes      mac
```

`REPORTED=yes` means the value came from the monitor's DDC/CI capabilities
string. `CURRENT=yes` is the input currently selected by VCP `0x60`. `NAME`
comes from the local YAML configuration. The connector label is the standard
meaning of the value; monitor firmware may use a vendor-specific value, which
is shown as `Unknown` rather than guessed.

If a monitor or connection cannot return a capabilities string, the command
prints a warning and still lists configured values with `REPORTED=unknown`.
This fallback keeps switching usable without claiming that a port was
detected.

## Use

```bash
monux detect
monux init
monux inputs
monux status
monux switch mac
monux switch linux
monux set 0x0f
monux serve
```

Common MCCS input-source values are:

| Connector | Hex | Decimal |
| --- | ---: | ---: |
| DisplayPort 1 | `0x0f` | 15 |
| DisplayPort 2 | `0x10` | 16 |
| HDMI 1 | `0x11` | 17 |
| HDMI 2 | `0x12` | 18 |
| USB-C | `0x1b` | 27 |

Monitor firmware can use different or vendor-specific values. Use
`monux inputs` on each connected machine before relying on automation.

On the validated Dell P2415Q, the monitor reports `0x0f`, `0x10`, and `0x11`.
Its second DisplayPort value corresponds to the monitor's second DP-family
connector (for example Mini DisplayPort); DDC/CI does not expose the physical
socket label itself.

## HTTP API

The HTTP server runs in the foreground and uses the same configuration and
native monitor backend as the CLI:

```bash
monux serve
```

It listens on `127.0.0.1:8765` by default. Query it locally with:

```bash
curl http://127.0.0.1:8765/healthz
curl http://127.0.0.1:8765/api/v1/status
curl http://127.0.0.1:8765/api/v1/inputs
curl http://127.0.0.1:8765/api/v1/capabilities
curl -X POST http://127.0.0.1:8765/api/v1/switch/linux
curl -X POST http://127.0.0.1:8765/api/v1/set/0x0f
```

Example status response:

```json
{"name":"mac","input":"0x11","value":17,"connector":"HDMI 1"}
```

For access from an ESP32 or another machine, bind to the LAN and set a token:

```bash
MONUX_HTTP_TOKEN='replace-with-a-secret' \
  monux serve --listen 0.0.0.0:8765
```

Clients then send the token as a Bearer credential:

```bash
curl -H 'Authorization: Bearer replace-with-a-secret' \
  http://linux-ip:8765/api/v1/status

curl -X POST \
  -H 'Authorization: Bearer replace-with-a-secret' \
  http://linux-ip:8765/api/v1/switch/mac
```

The token is not encrypted by plain HTTP. Use it only on a trusted LAN, or put
the loopback-only server behind a TLS reverse proxy on an untrusted network.

`GET /healthz` remains unauthenticated so a supervisor can check process
health. DDC operations from concurrent HTTP requests are serialized. The
server handles `SIGINT` and `SIGTERM` with a graceful shutdown; use systemd,
launchd, or another platform supervisor when it should run as a daemon.

After switching from Linux to Mac, the most reliable way to switch back is to
call the server running on the currently displayed Mac:

```bash
curl -X POST http://127.0.0.1:8765/api/v1/switch/linux
```

For an ESP32, use the Mac's LAN address and Bearer token instead. Run `monux`
on both computers: Linux handles the request while Linux is displayed, and Mac
handles it while Mac is displayed. Some monitors accept DDC commands over an
inactive input, but clients must not depend on that hardware-specific behavior.

`GET /api/v1/inputs` is a fast list of configured names. The slower
`GET /api/v1/capabilities` performs native DDC/CI discovery and merges the
monitor-reported values, current input, connector labels, and configured
names. `POST /api/v1/set/{value}` exposes the CLI's raw `set` operation and
should be reserved for diagnostics; normal clients should switch by name.

Environment variables:

| Variable | Purpose | Default |
| --- | --- | --- |
| `MONUX_HTTP_LISTEN` | HTTP listen address | `127.0.0.1:8765` |
| `MONUX_HTTP_TOKEN` | Optional Bearer token | empty |

## Linux permissions

The native Linux backend requires `i2c-dev` and permission to open the selected
device:

```bash
sudo modprobe i2c-dev
ls -l /dev/i2c-*
monux detect
```

Configure the distribution's `i2c` group or udev permissions if access is
denied. Running the whole program as root is not the intended setup.

## Verify the handoff

1. On Linux, run `monux switch mac` and confirm that macOS appears.
2. On the Mac, run `monux switch linux` and confirm that Linux appears.
3. If Windows is connected, switch to it and then switch away from Windows.
4. Repeat each direction several times before adding keyboard shortcuts.

Some monitors accept DDC only from the active input. Installing `monux` on
every machine supports the normal handoff because the visible machine sends
the command that switches away from itself. Whether an inactive machine can
also issue a successful command is hardware-dependent.

## Architecture

```text
CLI
 │
 ▼
Switcher ── named inputs ◀── HTTP API
 │
 ▼
monitor.Controller
 ├── native_linux.go  ── DDC/CI over /dev/i2c
 ├── native_windows.go ─ Win32 physical monitor API
 └── native_darwin.go ── CoreDisplay IOAVService
```

The service layer does not know which operating system it is running on. Input
capabilities are exposed by the platform-native `monitor.Backend`; switching
stays behind `monitor.Controller`, while protocol tests stay portable.
