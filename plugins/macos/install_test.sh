#!/usr/bin/env bash

set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
test_root=$(mktemp -d)
trap 'rm -rf -- "$test_root"' EXIT

fake_monux="$test_root/monux"
printf '%s\n' '#!/usr/bin/env bash' 'exit 0' >"$fake_monux"
chmod 0755 "$fake_monux"

fake_bin="$test_root/bin"
launchctl_log="$test_root/launchctl.log"
mkdir -p "$fake_bin"
printf '%s\n' \
  '#!/usr/bin/env bash' \
  'printf "%s\\n" "$*" >>"$FAKE_LAUNCHCTL_LOG"' \
  '[[ "${1:-}" != print ]]' >"$fake_bin/launchctl"
chmod 0755 "$fake_bin/launchctl"

PATH="$fake_bin:$PATH" \
FAKE_LAUNCHCTL_LOG="$launchctl_log" \
MONUX_APP_DIR="$test_root/Applications" \
MONUX_LAUNCH_AGENTS_DIR="$test_root/LaunchAgents" \
MONUX_EXECUTABLE="$fake_monux" \
MONUX_CONFIG="$test_root/config.yaml" \
MONUX_LOG_DIR="$test_root/Logs" \
MONUX_SKIP_INIT=1 \
MONUX_START_AT_LOGIN=1 \
MONUX_MANAGE_LAUNCH_AGENT=1 \
MONUX_LAUNCH=0 \
  "$script_dir/install.sh" >/dev/null

application="$test_root/Applications/Monux.app"
test -x "$application/Contents/MacOS/MonuxMenuBar"
test -x "$application/Contents/Helpers/monux"
test -f "$application/Contents/Info.plist"
test "$(cat "$application/Contents/Resources/config-path")" = "$test_root/config.yaml"
test "$(plutil -extract LSUIElement raw "$application/Contents/Info.plist")" = "true"
codesign --verify --deep --strict "$application"
"$application/Contents/MacOS/MonuxMenuBar" --self-test

menu_agent="$test_root/LaunchAgents/com.dyike.monux.menubar.plist"
server_agent="$test_root/LaunchAgents/com.dyike.monux.server.plist"
test "$(plutil -extract Label raw "$menu_agent")" = "com.dyike.monux.menubar"
test "$(plutil -extract Label raw "$server_agent")" = "com.dyike.monux.server"
test "$(plutil -extract ProgramArguments.0 raw "$server_agent")" = "$application/Contents/Helpers/monux"
test "$(plutil -extract ProgramArguments.2 raw "$server_agent")" = "$test_root/config.yaml"
test "$(plutil -extract ProgramArguments.3 raw "$server_agent")" = "serve"
test "$(plutil -extract ProgramArguments.5 raw "$server_agent")" = "0.0.0.0:8765"
test "$(plutil -extract RunAtLoad raw "$server_agent")" = "true"
test "$(plutil -extract KeepAlive raw "$server_agent")" = "true"
grep -Fq "bootstrap gui/$(id -u) $server_agent" "$launchctl_log"
grep -Fq "kickstart -k gui/$(id -u)/com.dyike.monux.server" "$launchctl_log"

echo "macOS menu bar installer test passed"
