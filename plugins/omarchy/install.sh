#!/usr/bin/env bash

set -euo pipefail
shopt -u patsub_replacement 2>/dev/null || true

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
source_dir="$script_dir/dyike.monux"
service_template="$script_dir/monux.service.in"
config_root="${XDG_CONFIG_HOME:-$HOME/.config}"
plugin_dir="$config_root/omarchy/plugins/dyike.monux"
shell_config="$config_root/omarchy/shell.json"
systemd_user_dir="$config_root/systemd/user"
service_file="$systemd_user_dir/monux.service"
monux_executable="${MONUX_EXECUTABLE:-monux}"
monux_config="${MONUX_CONFIG:-}"
install_server="${MONUX_INSTALL_SERVER:-1}"
server_listen="${MONUX_HTTP_LISTEN:-0.0.0.0:8765}"
server_token="${MONUX_HTTP_TOKEN:-}"
primary_input="${MONUX_PRIMARY_INPUT:-linux}"
secondary_input="${MONUX_SECONDARY_INPUT:-mac}"
tertiary_input="${MONUX_TERTIARY_INPUT:-windows}"

if [[ ! -f "$shell_config" ]]; then
  echo "monux Omarchy installer: missing $shell_config" >&2
  echo "Create the Omarchy shell configuration before installing the widget." >&2
  exit 1
fi

if ! command -v "$monux_executable" >/dev/null 2>&1; then
  echo "monux Omarchy installer: executable not found: $monux_executable" >&2
  echo "Build and install the monux CLI before installing the widget." >&2
  exit 1
fi
resolved_executable=$(command -v "$monux_executable")
if [[ "$resolved_executable" != /* ]]; then
  resolved_executable=$(cd -- "$(dirname -- "$resolved_executable")" && pwd)/$(basename -- "$resolved_executable")
fi
resolved_config="${monux_config:-$config_root/monux/config.yaml}"

if [[ -n "$monux_config" ]]; then
  "$monux_executable" --config "$monux_config" init
else
  "$monux_executable" init
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

if [[ "$install_server" != "0" && "$install_server" != "false" ]]; then
  systemd_quote() {
    local value="$1"
    if [[ "$value" == *$'\n'* || "$value" == *$'\r'* ]]; then
      echo "monux Omarchy installer: systemd values must not contain newlines" >&2
      exit 1
    fi
    value=${value//\\/\\\\}
    value=${value//\"/\\\"}
    value=${value//%/%%}
    printf '"%s"' "$value"
  }

  executable_arg=$(systemd_quote "$resolved_executable")
  config_arg=$(systemd_quote "$resolved_config")
  listen_arg=$(systemd_quote "$server_listen")
  token_environment=""
  if [[ -n "$server_token" ]]; then
    token_environment="Environment=$(systemd_quote "MONUX_HTTP_TOKEN=$server_token")"
  elif [[ "$server_listen" != 127.0.0.1:* && "$server_listen" != localhost:* && "$server_listen" != \[::1\]:* ]]; then
    echo "warning: monux.service will listen on $server_listen without authentication" >&2
    echo "Set MONUX_HTTP_TOKEN when installing if this is not a trusted LAN." >&2
  fi

  unit_content=$(<"$service_template")
  unit_content=${unit_content//@EXECUTABLE@/$executable_arg}
  unit_content=${unit_content//@CONFIG@/$config_arg}
  unit_content=${unit_content//@LISTEN@/$listen_arg}
  unit_content=${unit_content//@TOKEN_ENV@/$token_environment}

  mkdir -p "$systemd_user_dir"
  temporary_unit=$(mktemp "$systemd_user_dir/monux.service.XXXXXX")
  trap 'rm -f -- "$temporary_unit"' EXIT
  printf '%s\n' "$unit_content" >"$temporary_unit"
  chmod 0600 "$temporary_unit"
  mv -- "$temporary_unit" "$service_file"
  trap - EXIT

  systemctl --user daemon-reload
  systemctl --user enable monux.service
  systemctl --user stop monux.service >/dev/null 2>&1 || true

  server_port=${server_listen##*:}
  server_port=${server_port%]}
  if [[ "$server_port" =~ ^[0-9]+$ ]] && command -v ss >/dev/null 2>&1; then
    port_users=$(ss -H -ltnp "sport = :$server_port" 2>/dev/null || true)
    if [[ -n "$port_users" ]]; then
      existing_pid=$(printf '%s\n' "$port_users" | sed -n 's/.*pid=\([0-9][0-9]*\).*/\1/p' | head -n 1)
      existing_executable=""
      existing_owner=""
      existing_is_server=false
      if [[ -n "$existing_pid" && -r "/proc/$existing_pid/cmdline" ]]; then
        existing_executable=$(readlink "/proc/$existing_pid/exe" 2>/dev/null || true)
        existing_executable=${existing_executable% (deleted)}
        existing_owner=$(stat -c '%u' "/proc/$existing_pid" 2>/dev/null || true)
        if tr '\0' '\n' <"/proc/$existing_pid/cmdline" | grep -Fxq 'serve'; then
          existing_is_server=true
        fi
      fi
      expected_executable=$(readlink -f "$resolved_executable")
      if [[ "$existing_executable" == "$expected_executable" && "$existing_owner" == "$(id -u)" && "$existing_is_server" == "true" ]]; then
        echo "Migrating existing monux serve process $existing_pid to monux.service"
        kill -TERM "$existing_pid"
        for _ in {1..20}; do
          port_users=$(ss -H -ltnp "sport = :$server_port" 2>/dev/null || true)
          if [[ -z "$port_users" ]]; then
            break
          fi
          sleep 0.1
        done
        if [[ -n "$port_users" ]]; then
          echo "monux Omarchy installer: existing process $existing_pid did not release TCP port $server_port" >&2
          exit 1
        fi
      fi
      if [[ -n "$port_users" ]]; then
        echo "monux Omarchy installer: TCP port $server_port is already in use:" >&2
        printf '%s\n' "$port_users" >&2
        echo "Stop the existing listener or set MONUX_HTTP_LISTEN to another address." >&2
        exit 1
      fi
    fi
  fi

  systemctl --user restart monux.service
  health_address="$server_listen"
  health_address=${health_address/0.0.0.0/127.0.0.1}
  health_address=${health_address/\[::\]/[::1]}
  service_ready=false
  for _ in {1..10}; do
    if systemctl --user is-active --quiet monux.service && curl -fsS --max-time 1 "http://$health_address/healthz" >/dev/null 2>&1; then
      service_ready=true
      break
    fi
    sleep 0.2
  done
  if [[ "$service_ready" != "true" ]]; then
    echo "monux Omarchy installer: monux.service did not become healthy" >&2
    journalctl --user -u monux.service -n 20 --no-pager >&2 || true
    exit 1
  fi
  echo "Enabled and verified monux.service on $server_listen"
else
  echo "Skipped monux.service installation (MONUX_INSTALL_SERVER=$install_server)"
fi

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
