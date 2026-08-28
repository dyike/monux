# monux

`monux` switches one DDC/CI-capable monitor between a Mac and a Linux machine.
The first version runs on Linux and delegates DDC communication to `ddcutil`.
The Mac is a target input and can trigger Linux remotely; it does not need a
native DDC backend in this phase.

## Mac + Linux setup

The intended first-phase topology is:

```text
Mac ─────────────── HDMI/USB-C ──┐
                                 ├── Monitor
Linux + monux ──── DisplayPort ──┘
```

`monux` is installed on Linux, which remains powered on and owns monitor
control. The input names are configuration labels: `switch mac` selects the
connector used by the Mac, while `switch linux` selects the connector used by
Linux.

From Linux:

```bash
monux switch mac
monux switch linux
```

From the Mac, the current version can call the Linux CLI over SSH. Use the
absolute path to the Linux binary if it is not in the non-interactive SSH
`PATH`:

```bash
ssh <linux-user>@<linux-host> /absolute/path/to/monux switch linux
ssh <linux-user>@<linux-host> /absolute/path/to/monux switch mac
```

For convenient macOS commands, add aliases to `~/.zshrc` and replace the host
and path with real values:

```bash
alias display-linux='ssh linux-host /absolute/path/to/monux switch linux'
alias display-mac='ssh linux-host /absolute/path/to/monux switch mac'
```

No Go program or `ddcutil` installation is required on the Mac for this
workflow. A later `monux serve` command will replace SSH with a small HTTP API
for macOS, ESP32, and other remote controllers.

### Required hardware check

Some monitor and connection combinations may not accept DDC commands from an
inactive input. Test the complete round trip before relying on the Linux-only
controller:

1. On Linux, run `monux switch mac`.
2. After the monitor displays macOS, connect to Linux over SSH.
3. Run `monux switch linux` through that SSH session.

If step 3 succeeds, the Linux-only architecture works for the setup. If it
does not, the project will need either a native macOS controller backend or an
external DDC controller; adding the HTTP server alone would not fix that
hardware limitation.

## Requirements

- Go 1.24 or newer
- `ddcutil`
- Permission to access the monitor's `/dev/i2c-*` device
- DDC/CI enabled in the monitor's on-screen settings

On Debian/Ubuntu, install the runtime dependency with:

```bash
sudo apt install ddcutil
```

On Arch Linux/Omarchy:

```bash
sudo pacman -S ddcutil
```

## Build

```bash
go build -o monux ./cmd/monux
```

Or install it into your Go binary directory:

```bash
go install ./cmd/monux
```

## Configure

First detect the display and note its I2C bus:

```bash
./monux detect
```

Copy the example configuration to the default location and adjust the bus and
input values:

```bash
mkdir -p ~/.config/monux
cp config.example.yaml ~/.config/monux/config.yaml
```

The default path can be overridden with `--config`/`-c` or the
`MONUX_CONFIG` environment variable.

## Use

```bash
./monux status
./monux switch mac
./monux switch linux
./monux set 0x0f
```

Given the example configuration, `switch mac` runs the equivalent of:

```bash
ddcutil --bus 15 setvcp 60 0x11
```

Input values vary by monitor and connector. Verify them with `ddcutil`; do not
assume the example values match every Dell P2415Q setup.

## Troubleshooting

If `detect` reports that no `/dev/i2c` devices exist, first load the kernel
module and retry:

```bash
sudo modprobe i2c-dev
ls -l /dev/i2c-*
monux detect
```

If the device files exist but access is denied, configure your distribution's
`i2c` group or udev permissions, then sign in again. Running the entire daemon
as root is not the intended long-term setup.

If `status` works but switching does not, check that DDC/CI is enabled in the
monitor menu and confirm the input value manually:

```bash
ddcutil --bus 15 getvcp 60
ddcutil --bus 15 setvcp 60 0x11
```

## Architecture

The command layer loads configuration and delegates named switching to
`internal/service.Switcher`. The service only knows the `monitor.Controller`
interface; `internal/monitor.DDCUtil` is the initial Linux backend. A future
`serve` command can call the same service from an HTTP handler without changing
the DDC implementation. macOS and ESP32 are remote controllers in that design,
not additional owners of the monitor-control logic.
