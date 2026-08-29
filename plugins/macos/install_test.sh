#!/usr/bin/env bash

set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
test_root=$(mktemp -d)
trap 'rm -rf -- "$test_root"' EXIT

fake_monux="$test_root/monux"
printf '%s\n' '#!/usr/bin/env bash' 'exit 0' >"$fake_monux"
chmod 0755 "$fake_monux"

MONUX_APP_DIR="$test_root/Applications" \
MONUX_LAUNCH_AGENTS_DIR="$test_root/LaunchAgents" \
MONUX_EXECUTABLE="$fake_monux" \
MONUX_CONFIG="$test_root/config.yaml" \
MONUX_SKIP_INIT=1 \
MONUX_START_AT_LOGIN=0 \
MONUX_MANAGE_LAUNCH_AGENT=0 \
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

echo "macOS menu bar installer test passed"
