#!/usr/bin/env bash

# Optional workaround for shared monitors that disturb the other computer's
# displays when Linux turns its display output off and back on.
set -euo pipefail

mode="${1:-}"
if [[ "$mode" != enable && "$mode" != disable ]]; then
  echo "Usage: bash $0 enable|disable [local-lock-plugin-directory]" >&2
  exit 1
fi

plugin_dir="${2:-$HOME/.config/omarchy/plugins/${USER:-$(id -un)}.lock}"
command -v python3 >/dev/null

if [[ ! -d "$plugin_dir" ]]; then
  if [[ "$mode" == disable ]]; then
    echo "No local lock plugin to restore: $plugin_dir"
    exit 0
  fi
  if [[ $# -ge 2 ]]; then
    echo "Local lock plugin does not exist: $plugin_dir" >&2
    exit 1
  fi
  omarchy plugin clone omarchy.lock
fi

python3 - "$mode" "$plugin_dir" <<'PY'
import datetime
import json
import pathlib
import sys

mode, directory = sys.argv[1:]
plugin = pathlib.Path(directory).resolve()
user_plugins = (pathlib.Path.home() / ".config/omarchy/plugins").resolve()
if not plugin.is_relative_to(user_plugins):
    sys.exit("Only local plugins under ~/.config/omarchy/plugins may be changed")
manifest = json.loads((plugin / "manifest.json").read_text())
if manifest.get("omarchy", {}).get("clonedFrom") != "omarchy.lock":
    sys.exit("Expected a local clone of omarchy.lock")

path = plugin / "Service.qml"
content = path.read_text()
normal = 'command: ["bash", "-c", "omarchy-brightness-keyboard off; omarchy-brightness-display off"]'
keep_on = 'command: ["bash", "-c", "omarchy-brightness-keyboard off"]'
old, new = (normal, keep_on) if mode == "enable" else (keep_on, normal)
if content.count(new) == 1 and old not in content:
    print(f"Already {mode}d: {path}")
elif content.count(old) == 1 and new not in content:
    stamp = datetime.datetime.now().strftime("%Y%m%d-%H%M%S-%f")
    backup = path.with_name(f"Service.qml.bak.keep-display-on.{stamp}")
    backup.write_text(content)
    path.write_text(content.replace(old, new, 1))
    print(f"Updated {path}; backup: {backup}")
else:
    sys.exit("Unrecognized lock plugin command; no changes made")
PY

omarchy-shell shell rescanPlugins
echo "Keep display on while locked: $mode"
