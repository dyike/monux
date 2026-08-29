#!/usr/bin/env bash

set -euo pipefail

application_root="${MONUX_APP_DIR:-$HOME/Applications}"
application_path="$application_root/Monux.app"
launch_agents_root="${MONUX_LAUNCH_AGENTS_DIR:-$HOME/Library/LaunchAgents}"
launch_agent="$launch_agents_root/com.dyike.monux.menubar.plist"
domain="gui/$(id -u)"
label="com.dyike.monux.menubar"

if launchctl print "$domain/$label" >/dev/null 2>&1; then
  launchctl bootout "$domain/$label"
fi
if [[ -f "$launch_agent" ]]; then
  rm -f -- "$launch_agent"
fi
if [[ -d "$application_path" ]]; then
  rm -rf -- "$application_path"
fi

echo "Removed the Monux menu bar app and login item."
echo "Kept the Monux configuration and CLI data."
