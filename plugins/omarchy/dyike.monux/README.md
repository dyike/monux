# Monux Omarchy plugin

This manifest-backed Omarchy bar widget calls the local `monux` CLI. Its
native-style popup follows the same `Panel` / `KeyboardPanel` interaction model
as Omarchy's built-in Display widget.

Interactions:

- Left click: open or close the control panel.
- Right click: refresh the current input immediately.
- Inputs tab: view the current name/value and switch between the two configured
  computer inputs.
- Info tab: inspect the DDC transport, CLI/config paths, and refresh interval.
- Keyboard: `h/l` navigates tabs or choices, `j/k` changes section, Enter
  activates, `r` refreshes, and Escape closes.

While its panel is open, the widget polls `monux status` every ten seconds by
default. It stops polling as soon as the panel closes. All command arguments
are passed directly through Quickshell's `Process` API; no shell or SSH command
is involved.

Some monitors accept input changes but do not reliably report their current
VCP 60 value. In that case the panel shows a neutral availability note and
keeps all input buttons enabled; only an actual switch failure is shown as an
error.

Install from the repository root:

```bash
./plugins/omarchy/install.sh
```

For a repository build or another non-PATH installation, pass explicit paths
on the first install:

```bash
MONUX_EXECUTABLE="$HOME/.local/bin/monux" \
MONUX_CONFIG="$HOME/.config/monux/config.yaml" \
./plugins/omarchy/install.sh
```

The optional `MONUX_PRIMARY_INPUT` and `MONUX_SECONDARY_INPUT` installer
variables default to `linux` and `mac`. These variables only initialize a new
bar entry; subsequent reruns preserve the existing widget settings.

The installer copies this directory to
`~/.config/omarchy/plugins/dyike.monux/`, validates it with Omarchy, and adds
`dyike.monux` to the right section of the current bar layout before
`omarchy.monitor`. If the shell is running, the installer uses Omarchy's guarded
restart command so Quickshell cannot keep an older cached widget.

Configure plugin settings in `~/.config/omarchy/shell.json`, for example:

```json
{
  "id": "dyike.monux",
  "executable": "/home/me/.local/bin/monux",
  "configPath": "/home/me/.config/monux/config.yaml",
  "primaryInput": "linux",
  "secondaryInput": "mac",
  "refreshIntervalSec": 10,
  "showLabel": true
}
```

The `monux` executable and its configuration must work from a non-interactive
process before the widget can control the monitor.
