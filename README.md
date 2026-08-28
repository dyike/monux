# monux

`monux` switches one DDC/CI-capable monitor between named inputs. The
first version targets Linux and delegates DDC communication to `ddcutil`; Mac,
ESP32, and a desktop UI can later reuse the same switching service.

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
the DDC implementation.
