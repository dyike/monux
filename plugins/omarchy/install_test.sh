#!/usr/bin/env bash

set -euo pipefail

repository_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
test_root=$(mktemp -d)
trap 'rm -rf -- "$test_root"' EXIT

config_root="$test_root/config"
fake_bin="$test_root/bin"
log_file="$test_root/systemctl.log"
mkdir -p "$config_root/omarchy" "$fake_bin"
printf '%s\n' '{"bar":{"layout":{"right":[]}}}' >"$config_root/omarchy/shell.json"

cat >"$fake_bin/monux" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
mkdir -p "$XDG_CONFIG_HOME/monux"
printf '%s\n' 'monitor:' '  id: "23"' 'inputs:' '  linux: displayport-1' >"$XDG_CONFIG_HOME/monux/config.yaml"
SCRIPT

cat >"$fake_bin/omarchy" <<'SCRIPT'
#!/usr/bin/env bash
exit 0
SCRIPT

cat >"$fake_bin/omarchy-shell" <<'SCRIPT'
#!/usr/bin/env bash
exit 1
SCRIPT

cat >"$fake_bin/systemctl" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$TEST_SYSTEMCTL_LOG"
SCRIPT

cat >"$fake_bin/curl" <<'SCRIPT'
#!/usr/bin/env bash
exit 0
SCRIPT
chmod 0755 "$fake_bin/monux" "$fake_bin/omarchy" "$fake_bin/omarchy-shell" "$fake_bin/systemctl" "$fake_bin/curl"

env \
  HOME="$test_root/home" \
  XDG_CONFIG_HOME="$config_root" \
  PATH="$fake_bin:$PATH" \
  MONUX_EXECUTABLE="$fake_bin/monux" \
  MONUX_HTTP_TOKEN="test-secret" \
  TEST_SYSTEMCTL_LOG="$log_file" \
  "$repository_root/plugins/omarchy/install.sh" >/dev/null

unit="$config_root/systemd/user/monux.service"
test -f "$unit"
grep -Fq "ExecStart=\"$fake_bin/monux\" --config \"$config_root/monux/config.yaml\" serve --listen \"0.0.0.0:8765\"" "$unit"
grep -Fq 'Environment="MONUX_HTTP_TOKEN=test-secret"' "$unit"
test "$(stat -c '%a' "$unit")" = "600"
grep -Fq "daemon-reload" "$log_file"
grep -Fq "enable monux.service" "$log_file"
grep -Fq "stop monux.service" "$log_file"
grep -Fq "restart monux.service" "$log_file"
grep -Fq "is-active --quiet monux.service" "$log_file"
test -f "$config_root/omarchy/plugins/dyike.monux/Monux.qml"
jq -e '.bar.layout.right | any(.id == "dyike.monux")' "$config_root/omarchy/shell.json" >/dev/null

echo "Omarchy installer test passed"
