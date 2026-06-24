#!/usr/bin/env bash
set -euo pipefail

# Lightweight smoke test for the agent-facing CLI.
#
# Usage:
#   ZYHIVE_BIN=./bin/aipanel ZYHIVE_HOST=http://localhost:8080 ZYHIVE_TOKEN=... \
#     bash scripts/agentcli-smoke.sh
#
# The script is intentionally read-only except for the server's normal request
# logging. It validates help routing, the generic API escape hatch, and a core
# business command.

BIN="${ZYHIVE_BIN:-zyhive}"

"$BIN" agent --help >/dev/null
"$BIN" api GET /api/version --json >/dev/null
"$BIN" system ready --json >/dev/null
"$BIN" agent list --json >/dev/null

echo "agentcli smoke: ok"
