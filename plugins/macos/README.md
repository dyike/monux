# Monux macOS menu bar app

The native AppKit menu bar app packages the `monux` CLI as its private helper.
It does not require SwiftBar or another tray framework.

The menu shows the current monitor input and every named input from the Monux
configuration. Selecting an input runs the same peer-aware
`monux switch <name>` command as the CLI. Opening the menu uses cached data and
does not start a refresh. The app refreshes the input list and status every five
minutes, and also provides manual refresh, configuration-file access, and quit
actions. The cache includes the configured inputs and last successful current
input, so it is restored after the app restarts.

If the active video path cannot answer a DDC status read, the menu leaves the
current input unavailable (or keeps the last successful selection) and keeps
every configured input enabled. The read error remains available as a tooltip;
explicit loading and switching failures still appear as warnings.

## Install

From the repository root on macOS:

```bash
./plugins/macos/install.sh
```

The installer:

1. Builds the current Monux CLI and runs `monux init`.
2. Builds and ad-hoc signs `~/Applications/Monux.app`.
3. Installs and starts
   `~/Library/LaunchAgents/com.dyike.monux.menubar.plist` so the icon returns
   at login.

The app has `LSUIElement` enabled, so it appears only in the menu bar and not
in the Dock. It reads the ordinary `~/.config/monux/config.yaml` file and keeps
the packaged CLI isolated from shell `PATH` differences.

To package an existing CLI or use another configuration:

```bash
MONUX_EXECUTABLE="$HOME/.local/bin/monux" \
MONUX_CONFIG="$HOME/.config/monux/config.yaml" \
  ./plugins/macos/install.sh
```

Installation variables:

| Variable | Purpose | Default |
| --- | --- | --- |
| `MONUX_EXECUTABLE` | Existing CLI copied into the app; otherwise build the repository source | empty |
| `MONUX_CONFIG` | Configuration used by the menu bar helper | `~/.config/monux/config.yaml` |
| `MONUX_APP_DIR` | Parent directory for `Monux.app` | `~/Applications` |
| `MONUX_START_AT_LOGIN` | Set to `0` or `false` to skip the LaunchAgent | `1` |
| `MONUX_SKIP_INIT` | Set to `1` only when configuration is managed separately | `0` |

## Uninstall

```bash
./plugins/macos/uninstall.sh
```

Uninstalling removes the application and LaunchAgent. It deliberately keeps
the Monux configuration.
