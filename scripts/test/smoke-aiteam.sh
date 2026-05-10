#!/usr/bin/env bash
# scripts/test/smoke-aiteam.sh — Smoke test of ZyHive + aiteam endpoints.
#
# Verifies:
#   1. /api/version responds and matches expected version (if provided)
#   2. /healthz and /readyz are reachable
#   3. /api/aiteam/flags is reachable with auth token (always-on endpoint)
#   4. With all ZYHIVE_EXPERIMENTAL_* flags OFF: every gated /api/aiteam/*
#      route returns HTTP 404 with body {"error":"not enabled", ...}
#   5. Main-line behaviour unchanged: /api/agents responds 200
#
# Usage:
#   ./scripts/test/smoke-aiteam.sh [base_url] [auth_token] [expected_version]
#
# Required:
#   base_url    — e.g. http://18.162.161.138:8080
#   auth_token  — bearer token from /etc/zyhive/zyhive.json auth.token
# Optional:
#   expected_version — if set, /api/version must include it (substring)
#
# Exits non-zero on first failure. Output is grep-friendly.

set -euo pipefail

BASE="${1:-${ZYHIVE_BASE_URL:-http://localhost:8080}}"
TOKEN="${2:-${ZYHIVE_TOKEN:-}}"
EXPECTED_VERSION="${3:-${ZYHIVE_EXPECTED_VERSION:-}}"

if [[ -z "$TOKEN" ]]; then
    echo "❌ auth token required (arg 2 or \$ZYHIVE_TOKEN)" >&2
    exit 2
fi

pass=0
fail=0

assert_code() {
    local label="$1"; local want="$2"; local got="$3"
    if [[ "$got" == "$want" ]]; then
        echo "  ✅ $label  (HTTP $got)"
        pass=$((pass+1))
    else
        echo "  ❌ $label  expected HTTP $want, got $got" >&2
        fail=$((fail+1))
    fi
}

assert_grep() {
    local label="$1"; local pat="$2"; local body="$3"
    if echo "$body" | grep -q -- "$pat"; then
        echo "  ✅ $label  (body matches $pat)"
        pass=$((pass+1))
    else
        echo "  ❌ $label  body does NOT contain $pat" >&2
        echo "     body: $body" >&2
        fail=$((fail+1))
    fi
}

curl_get() {
    local path="$1"
    local hdrs=()
    if [[ "$path" == /api/* ]]; then hdrs+=(-H "Authorization: Bearer $TOKEN"); fi
    curl -sS -o /tmp/smoke-body.$$ -w "%{http_code}" "${hdrs[@]}" "$BASE$path" || echo "000"
}

echo "▶ smoke target: $BASE"

# 1. /api/version
echo "--- 1. /api/version ---"
code=$(curl_get /api/version)
body=$(cat /tmp/smoke-body.$$)
assert_code "version reachable" 200 "$code"
if [[ -n "$EXPECTED_VERSION" ]]; then
    assert_grep "version contains '$EXPECTED_VERSION'" "$EXPECTED_VERSION" "$body"
fi

# 2. /healthz + /readyz
echo "--- 2. /healthz + /readyz ---"
code=$(curl_get /healthz)
assert_code "/healthz reachable" 200 "$code"
code=$(curl_get /readyz)
# readyz returns 200 on healthy state and 503 on degraded — both are valid
# responses for a smoke test (we just verify it's a recognised code).
if [[ "$code" == "200" || "$code" == "503" ]]; then
    echo "  ✅ /readyz reachable  (HTTP $code)"
    pass=$((pass+1))
else
    echo "  ❌ /readyz unexpected HTTP $code" >&2
    fail=$((fail+1))
fi

# 3. /api/aiteam/flags is always available
echo "--- 3. /api/aiteam/flags (discovery, always-on) ---"
code=$(curl_get /api/aiteam/flags)
body=$(cat /tmp/smoke-body.$$)
assert_code "flags endpoint reachable" 200 "$code"
assert_grep "flags JSON has 'any' key" '"any"' "$body"
assert_grep "flags JSON lists ZYHIVE_EXPERIMENTAL_WALLET" "ZYHIVE_EXPERIMENTAL_WALLET" "$body"

# 4. Every gated /api/aiteam/* endpoint must 404 by default
echo "--- 4. gated endpoints default-OFF (404) ---"
gated=(
    "/api/aiteam/wallet/alice"
    "/api/aiteam/fx/rates"
    "/api/aiteam/guard"
    "/api/aiteam/payroll/alice"
    "/api/aiteam/overview"
    "/api/aiteam/audit"
)
for ep in "${gated[@]}"; do
    code=$(curl_get "$ep")
    body=$(cat /tmp/smoke-body.$$)
    assert_code "$ep gated 404" 404 "$code"
    assert_grep "$ep body says 'not enabled'" '"not enabled"' "$body"
done

# 5. Main-line endpoint still works (proves aiteam is zero-impact)
echo "--- 5. main-line unaffected ---"
code=$(curl_get /api/agents)
assert_code "/api/agents reachable" 200 "$code"

rm -f /tmp/smoke-body.$$

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  passed: $pass"
echo "  failed: $fail"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
[[ "$fail" -eq 0 ]]
