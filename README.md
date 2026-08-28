# monux

`monux` switches one DDC/CI monitor between computers from macOS, Linux, or
Windows. The same Go CLI is installed on every machine, and each installation
talks to the monitor locally through its own video connection.

There is no SSH, daemon, network dependency, `ddcutil`, or `m1ddc` runtime
dependency.

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
checksums, VCP reply parsing, and VCP code `0x60`. Platform files only perform
discovery and transport.

See [platform support](docs/platform-support.md) for implementation details and
current hardware-validation status.

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

Copy and edit `config.example.yaml`:

```yaml
monitor:
  id: "15"

inputs:
  mac: 0x11
  linux: 0x0f
  windows: 0x12
```

`monitor.id` is local to the operating system, so it may be `15` on Linux and
`1` on macOS or Windows. The named `inputs` section can otherwise be identical
on every machine.

Override the default path with `--config`/`-c` or `MONUX_CONFIG`.

## Use

```bash
monux detect
monux status
monux switch mac
monux switch linux
monux set 0x0f
```

Common MCCS input-source values are:

| Connector | Hex | Decimal |
| --- | ---: | ---: |
| DisplayPort 1 | `0x0f` | 15 |
| DisplayPort 2 | `0x10` | 16 |
| HDMI 1 | `0x11` | 17 |
| HDMI 2 | `0x12` | 18 |
| USB-C | `0x1b` | 27 |

Monitor firmware can use different or vendor-specific values. Confirm the
actual Dell P2415Q inputs before relying on automation.

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
Switcher ── named inputs
 │
 ▼
monitor.Controller
 ├── native_linux.go  ── DDC/CI over /dev/i2c
 ├── native_windows.go ─ Win32 physical monitor API
 └── native_darwin.go ── CoreDisplay IOAVService
```

The service layer does not know which operating system it is running on. New
platform behavior stays behind `monitor.Controller`, while protocol tests stay
portable.
