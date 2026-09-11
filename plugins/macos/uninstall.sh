#!/usr/bin/env bash

set -euo pipefail

application_root="${MONUX_APP_DIR:-$HOME/Applications}"
application_path="$application_root/Monux.app"
launch_agents_root="${MONUX_LAUNCH_AGENTS_DIR:-$HOME/Library/LaunchAgents}"
launch_agent="$launch_agents_root/com.dyike.monux.menubar.plist"
server_launch_agent="$launch_agents_root/com.dyike.monux.server.plist"
domain="gui/$(id -u)"
label="com.dyike.monux.menubar"
server_label="com.dyike.monux.server"

if launchctl print "$domain/$label" >/dev/null 2>&1; then
  launchctl bootout "$domain/$label"
fi
if launchctl print "$domain/$server_label" >/dev/null 2>&1; then
  launchctl bootout "$domain/$server_label"
fi
if [[ -f "$launch_agent" ]]; then
  rm -f -- "$launch_agent"
fi
if [[ -f "$server_launch_agent" ]]; then
  rm -f -- "$server_launch_agent"
fi
if [[ -d "$application_path" ]]; then
  rm -rf -- "$application_path"
fi

echo "Removed the Monux menu bar app, peer server, and login items."
echo "Kept the Monux configuration and CLI data."
