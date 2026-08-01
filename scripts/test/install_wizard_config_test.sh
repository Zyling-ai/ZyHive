#!/bin/bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
INSTALL_SCRIPT="$ROOT/scripts/install.sh"

# Load only the pure JSON helper functions; sourcing install.sh would run installation.
eval "$(awk '/^_json_escape\(\)/,/^# 向导主函数/' "$INSTALL_SCRIPT" | sed '$d')"

json=$(_make_channels_json '123:ABC' '111,222')
python3 - "$json" <<'PY'
import json
import sys

channels = json.loads(sys.argv[1])
channel = channels[0]
assert "allowedFrom" not in channel, channel
assert channel["config"]["allowedFrom"] == "111,222", channel
assert channel["config"]["botToken"] == "123:ABC", channel
PY

echo "install wizard Telegram config test: PASS"
