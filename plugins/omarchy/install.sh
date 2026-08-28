#!/usr/bin/env bash

set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
source_dir="$script_dir/dyike.monux"
config_root="${XDG_CONFIG_HOME:-$HOME/.config}"
plugin_dir="$config_root/omarchy/plugins/dyike.monux"
shell_config="$config_root/omarchy/shell.json"
monux_executable="${MONUX_EXECUTABLE:-monux}"
monux_config="${MONUX_CONFIG:-}"
primary_input="${MONUX_PRIMARY_INPUT:-linux}"
secondary_input="${MONUX_SECONDARY_INPUT:-mac}"
tertiary_input="${MONUX_TERTIARY_INPUT:-windows}"

if [[ ! -f "$shell_config" ]]; then
  echo "monux Omarchy installer: missing $shell_config" >&2
  echo "Create the Omarchy shell configuration before installing the widget." >&2
  exit 1
fi

omarchy plugin validate "$source_dir"

mkdir -p "$plugin_dir"
install -m 0644 "$source_dir/manifest.json" "$plugin_dir/manifest.json"
install -m 0644 "$source_dir/Monux.qml" "$plugin_dir/Monux.qml"
install -m 0644 "$source_dir/README.md" "$plugin_dir/README.md"

backup="$shell_config.bak.monux.$(date +%Y%m%d%H%M%S)"
cp -- "$shell_config" "$backup"
temporary=$(mktemp "$shell_config.monux.XXXXXX")
trap 'rm -f -- "$temporary"' EXIT

jq \
  --arg executable "$monux_executable" \
  --arg configPath "$monux_config" \
  --arg primaryInput "$primary_input" \
  --arg secondaryInput "$secondary_input" \
  --arg tertiaryInput "$tertiary_input" '
  def upsert_before($items; $entry; $before):
    ($items | map(
      if .id == $entry.id and (has("tertiaryInput") | not)
      then . + {"tertiaryInput":$entry.tertiaryInput}
      else .
      end
    )) as $updated
    | if any($updated[]?; .id == $entry.id) then
      $updated
    else
      ($updated | map(.id) | index($before)) as $index
      | if $index == null then $updated + [$entry]
        else $updated[0:$index] + [$entry] + $updated[$index:]
        end
    end;
  .bar.layout.right = upsert_before(
    (.bar.layout.right // []);
    {
      "id":"dyike.monux",
      "executable":$executable,
      "configPath":$configPath,
      "primaryInput":$primaryInput,
      "secondaryInput":$secondaryInput,
      "tertiaryInput":$tertiaryInput
    };
    "omarchy.monitor"
  )
' "$shell_config" >"$temporary"
chmod --reference="$shell_config" "$temporary"
mv -- "$temporary" "$shell_config"
trap - EXIT

if omarchy-shell shell ping >/dev/null 2>&1; then
  # A rescan discovers new manifests, but an updated QML entry point can remain
  # in Quickshell's component cache. Prefer Omarchy's guarded restart command so
  # the installed widget is always the version that is visible in the bar.
  if command -v omarchy-restart-shell >/dev/null 2>&1 && omarchy-restart-shell; then
    echo "Restarted the Omarchy shell and loaded dyike.monux."
  elif omarchy-shell shell rescanPlugins >/dev/null 2>&1; then
    echo "Rescanned the running Omarchy shell (restart was unavailable)."
  else
    echo "Could not reload the running Omarchy shell; restart it to load the plugin." >&2
  fi
else
  echo "Omarchy shell is not running; the plugin will load on its next start."
fi

echo "Installed dyike.monux into $plugin_dir"
echo "Updated $shell_config (backup: $backup)"
