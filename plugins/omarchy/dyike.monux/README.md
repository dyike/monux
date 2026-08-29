# Monux Omarchy plugin

This manifest-backed Omarchy bar widget calls the local `monux` CLI. Its
native-style popup follows the same `Panel` / `KeyboardPanel` interaction model
as Omarchy's built-in Display widget.

Interactions:

- Left click: open or close the control panel.
- Right click: refresh the current input immediately.
- Inputs tab: view the current name/value and switch between the three configured
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

The installer runs `monux init` before installing the widget. This creates the
default configuration automatically, or refreshes a changed platform-local
monitor ID while preserving existing input names. On first use, keep the
monitor on this Linux machine so the current input is named `linux`. A monitor
cannot identify the operating system attached to an inactive port; add known
remote mappings first when their standard connector names are not sufficient:

```bash
monux init --input linux=displayport-1 --input mac=hdmi-1 \
  --input windows=displayport-2
```

For a repository build or another non-PATH installation, pass explicit paths
on the first install:

```bash
MONUX_EXECUTABLE="$HOME/.local/bin/monux" \
MONUX_CONFIG="$HOME/.config/monux/config.yaml" \
./plugins/omarchy/install.sh
```

The optional `MONUX_PRIMARY_INPUT`, `MONUX_SECONDARY_INPUT`, and
`MONUX_TERTIARY_INPUT` installer variables default to `linux`, `mac`, and
`windows`. These variables initialize a new bar entry. For an existing entry,
the installer preserves its settings and only adds a missing tertiary input.

The installer copies this directory to
`~/.config/omarchy/plugins/dyike.monux/`, validates it with Omarchy, and adds
`dyike.monux` to the right section of the current bar layout before
`omarchy.monitor`. If the shell is running, the installer uses Omarchy's guarded
restart command so Quickshell cannot keep an older cached widget.

The installer also writes `~/.config/systemd/user/monux.service`, enables it
for the user session, and starts or restarts it immediately. The service uses
the same executable and configuration as the widget and listens on
`0.0.0.0:8765` by default so a configured Mac peer can reach Linux. The QML
widget itself does not spawn or supervise the HTTP server.

When the same installed Monux executable already has a user-owned foreground
`serve` process on that port, the installer stops that exact process and
migrates it to `monux.service`. It never terminates an unrelated executable;
other port conflicts stop installation with the listener details.

Installation variables for the service:

| Variable | Purpose | Default |
| --- | --- | --- |
| `MONUX_INSTALL_SERVER` | Set to `0` or `false` to skip the user service | `1` |
| `MONUX_HTTP_LISTEN` | Service listen address | `0.0.0.0:8765` |
| `MONUX_HTTP_TOKEN` | Incoming Bearer token stored in the user unit | empty |

An empty token exposes monitor switching to the local network. That is only
appropriate on a trusted LAN. To install with authentication:

```bash
MONUX_HTTP_TOKEN='replace-with-a-shared-secret' \
  ./plugins/omarchy/install.sh
```

Configure the same value as the peer's `token`. Inspect or restart the service
with:

```bash
systemctl --user status monux.service
systemctl --user restart monux.service
journalctl --user -u monux.service
```

The server loads configuration at startup. Restart `monux.service` after
manually editing peers or inputs outside the installer.

Configure plugin settings in `~/.config/omarchy/shell.json`, for example:

```json
{
  "id": "dyike.monux",
  "executable": "/home/me/.local/bin/monux",
  "configPath": "/home/me/.config/monux/config.yaml",
  "primaryInput": "linux",
  "secondaryInput": "mac",
  "tertiaryInput": "windows",
  "refreshIntervalSec": 10,
  "showLabel": true
}
```

The `monux` executable and its configuration must work from a non-interactive
process before the widget can control the monitor.

When `peers` are configured in Monux, the widget needs no peer-specific QML.
Its ordinary `monux status` and `monux switch <name>` calls automatically use
the active peer whenever this Linux machine's local DDC connection is inactive.
