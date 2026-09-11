#!/usr/bin/env bash

set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repository_root=$(cd -- "$script_dir/../.." && pwd)
application_root="${MONUX_APP_DIR:-$HOME/Applications}"
application_path="$application_root/Monux.app"
launch_agents_root="${MONUX_LAUNCH_AGENTS_DIR:-$HOME/Library/LaunchAgents}"
launch_agent="$launch_agents_root/com.dyike.monux.menubar.plist"
server_launch_agent="$launch_agents_root/com.dyike.monux.server.plist"
config_path="${MONUX_CONFIG:-$HOME/.config/monux/config.yaml}"
source_executable="${MONUX_EXECUTABLE:-}"
start_at_login="${MONUX_START_AT_LOGIN:-1}"
launch_application="${MONUX_LAUNCH:-1}"
manage_launch_agent="${MONUX_MANAGE_LAUNCH_AGENT:-1}"
skip_init="${MONUX_SKIP_INIT:-0}"
install_server="${MONUX_INSTALL_SERVER:-1}"
server_listen="${MONUX_HTTP_LISTEN:-0.0.0.0:8765}"
server_token="${MONUX_HTTP_TOKEN:-}"
logs_root="${MONUX_LOG_DIR:-$HOME/Library/Logs/Monux}"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "monux macOS installer: this installer must run on macOS" >&2
  exit 1
fi
if ! command -v xcrun >/dev/null 2>&1 || ! xcrun --find swiftc >/dev/null 2>&1; then
  echo "monux macOS installer: Apple Command Line Tools with Swift are required" >&2
  exit 1
fi

temporary_root=$(mktemp -d)
trap 'rm -rf -- "$temporary_root"' EXIT
staged_app="$temporary_root/Monux.app"
mkdir -p "$staged_app/Contents/MacOS" "$staged_app/Contents/Helpers" "$staged_app/Contents/Resources"

if [[ -n "$source_executable" ]]; then
  if [[ "$source_executable" != */* ]]; then
    source_executable=$(command -v "$source_executable" || true)
  fi
  if [[ -z "$source_executable" || ! -x "$source_executable" ]]; then
    echo "monux macOS installer: executable not found: ${MONUX_EXECUTABLE:-}" >&2
    exit 1
  fi
  cp -- "$source_executable" "$staged_app/Contents/Helpers/monux"
else
  (
    cd -- "$repository_root"
    CGO_ENABLED=1 go build -o "$staged_app/Contents/Helpers/monux" ./cmd/monux
  )
fi
chmod 0755 "$staged_app/Contents/Helpers/monux"

if [[ "$skip_init" != "1" && "$skip_init" != "true" ]]; then
  "$staged_app/Contents/Helpers/monux" --config "$config_path" init
fi

xcrun swiftc \
  -swift-version 5 \
  -O \
  -framework AppKit \
  -framework Foundation \
  "$script_dir/MonuxMenuBar.swift" \
  -o "$staged_app/Contents/MacOS/MonuxMenuBar"
cp -- "$script_dir/Info.plist" "$staged_app/Contents/Info.plist"
printf '%s\n' "$config_path" >"$staged_app/Contents/Resources/config-path"
chmod 0755 "$staged_app/Contents/MacOS/MonuxMenuBar"
chmod 0644 "$staged_app/Contents/Info.plist" "$staged_app/Contents/Resources/config-path"
codesign --force --deep --sign - "$staged_app" >/dev/null

mkdir -p "$application_root"
backup_path=""
if [[ -e "$application_path" ]]; then
  backup_path="$application_root/.Monux.app.backup.$(date +%Y%m%d%H%M%S)"
  mv -- "$application_path" "$backup_path"
fi
if ! mv -- "$staged_app" "$application_path"; then
  if [[ -n "$backup_path" && -e "$backup_path" ]]; then
    mv -- "$backup_path" "$application_path"
  fi
  exit 1
fi

domain="gui/$(id -u)"
label="com.dyike.monux.menubar"
server_label="com.dyike.monux.server"
if [[ "$manage_launch_agent" != "0" && "$manage_launch_agent" != "false" ]]; then
  if launchctl print "$domain/$label" >/dev/null 2>&1; then
    launchctl bootout "$domain/$label"
  fi
  if launchctl print "$domain/$server_label" >/dev/null 2>&1; then
    launchctl bootout "$domain/$server_label"
  fi

  if [[ "$start_at_login" != "0" && "$start_at_login" != "false" ]]; then
    mkdir -p "$launch_agents_root"
    temporary_agent="$temporary_root/com.dyike.monux.menubar.plist"
    plutil -create xml1 "$temporary_agent"
    plutil -insert Label -string "$label" "$temporary_agent"
    plutil -insert ProgramArguments -array "$temporary_agent"
    plutil -insert ProgramArguments.0 -string "$application_path/Contents/MacOS/MonuxMenuBar" "$temporary_agent"
    plutil -insert RunAtLoad -bool true "$temporary_agent"
    plutil -insert ProcessType -string Interactive "$temporary_agent"
    install -m 0644 "$temporary_agent" "$launch_agent"
    launchctl bootstrap "$domain" "$launch_agent"
    launchctl kickstart -k "$domain/$label"
    echo "Installed and started login item: $launch_agent"

    if [[ "$install_server" != "0" && "$install_server" != "false" ]]; then
      mkdir -p "$logs_root"
      temporary_server_agent="$temporary_root/com.dyike.monux.server.plist"
      plutil -create xml1 "$temporary_server_agent"
      plutil -insert Label -string "$server_label" "$temporary_server_agent"
      plutil -insert ProgramArguments -array "$temporary_server_agent"
      plutil -insert ProgramArguments.0 -string "$application_path/Contents/Helpers/monux" "$temporary_server_agent"
      plutil -insert ProgramArguments.1 -string "--config" "$temporary_server_agent"
      plutil -insert ProgramArguments.2 -string "$config_path" "$temporary_server_agent"
      plutil -insert ProgramArguments.3 -string "serve" "$temporary_server_agent"
      plutil -insert ProgramArguments.4 -string "--listen" "$temporary_server_agent"
      plutil -insert ProgramArguments.5 -string "$server_listen" "$temporary_server_agent"
      plutil -insert RunAtLoad -bool true "$temporary_server_agent"
      plutil -insert KeepAlive -bool true "$temporary_server_agent"
      plutil -insert ProcessType -string Background "$temporary_server_agent"
      plutil -insert StandardOutPath -string "$logs_root/server.log" "$temporary_server_agent"
      plutil -insert StandardErrorPath -string "$logs_root/server.log" "$temporary_server_agent"
      if [[ -n "$server_token" ]]; then
        plutil -insert EnvironmentVariables -dict "$temporary_server_agent"
        plutil -insert EnvironmentVariables.MONUX_HTTP_TOKEN -string "$server_token" "$temporary_server_agent"
      elif [[ "$server_listen" != 127.0.0.1:* && "$server_listen" != localhost:* && "$server_listen" != \[::1\]:* ]]; then
        echo "warning: macOS Monux server will listen on $server_listen without authentication" >&2
        echo "Set MONUX_HTTP_TOKEN when installing if this is not a trusted LAN." >&2
      fi
      install -m 0600 "$temporary_server_agent" "$server_launch_agent"
      launchctl bootstrap "$domain" "$server_launch_agent"
      launchctl kickstart -k "$domain/$server_label"
      echo "Installed and started peer server: $server_launch_agent ($server_listen)"
    elif [[ -f "$server_launch_agent" ]]; then
      rm -f -- "$server_launch_agent"
    fi
  else
    rm -f -- "$launch_agent"
    rm -f -- "$server_launch_agent"
  fi
fi

if [[ "$manage_launch_agent" == "0" || "$manage_launch_agent" == "false" ||
      "$start_at_login" == "0" || "$start_at_login" == "false" ]]; then
  if [[ "$launch_application" != "0" && "$launch_application" != "false" ]]; then
    open -gja "$application_path"
  fi
fi

if [[ -n "$backup_path" && -e "$backup_path" ]]; then
  rm -rf -- "$backup_path"
fi

echo "Installed Monux menu bar app: $application_path"
echo "Configuration: $config_path"
