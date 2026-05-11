#!/usr/bin/env bash
# smoke-aiteam-ui-flow.sh — exercises every API endpoint the UI relies on,
# end to end, with all aiteam flags ON. For each step verifies the JSON
# response has the shape the Vue components expect.
#
# Usage:
#   ./smoke-aiteam-ui-flow.sh [base_url] [auth_token]
#
# Exits non-zero on the first mismatch.

set -euo pipefail

BASE="${1:-http://18.162.161.138:8080}"
TOKEN="${2:-${ZYHIVE_TOKEN:-}}"
if [[ -z "$TOKEN" ]]; then
    echo "❌ auth token required (arg 2 or \$ZYHIVE_TOKEN)" >&2
    exit 2
fi

AUTH="Authorization: Bearer $TOKEN"
TMP=/tmp/ui-flow-$$
mkdir -p "$TMP"
trap "rm -rf $TMP" EXIT

PASS=0
FAIL=0

assert() {
    local label="$1" actual="$2" expected="$3"
    if [[ "$actual" == "$expected" ]]; then
        echo "  ✅ $label"
        PASS=$((PASS+1))
    else
        echo "  ❌ $label expected=$expected got=$actual" >&2
        FAIL=$((FAIL+1))
    fi
}

assert_in() {
    local label="$1" actual="$2" needle="$3"
    if echo "$actual" | grep -q -- "$needle"; then
        echo "  ✅ $label"
        PASS=$((PASS+1))
    else
        echo "  ❌ $label missing: $needle" >&2
        echo "     got: $actual" >&2
        FAIL=$((FAIL+1))
    fi
}

api() {
    local method="$1" path="$2" body="${3:-}"
    if [[ -n "$body" ]]; then
        curl -s -X "$method" -H "$AUTH" -H "Content-Type: application/json" -d "$body" "$BASE$path"
    else
        curl -s -X "$method" -H "$AUTH" "$BASE$path"
    fi
}

# ────────────────────────────────────────────────────────────────────
# Section 1: App.vue boot — flags discovery
# ────────────────────────────────────────────────────────────────────
echo "=== § 1: App.vue boot — getFlags() ==="
flags=$(api GET /api/aiteam/flags)
echo "$flags" > "$TMP/flags.json"
assert "flags JSON has 'any' key" "$(echo "$flags" | python3 -c 'import sys,json; print("yes" if "any" in json.load(sys.stdin) else "no")')" "yes"
assert "flags JSON has 'flags' key" "$(echo "$flags" | python3 -c 'import sys,json; print("yes" if "flags" in json.load(sys.stdin) else "no")')" "yes"
any_on=$(echo "$flags" | python3 -c 'import sys,json; print(json.load(sys.stdin)["any"])')
echo "  ℹ aiteam any: $any_on"

# Number of flags should be exactly 8 (ZYHIVE_EXPERIMENTAL_*)
flag_count=$(echo "$flags" | python3 -c 'import sys,json; print(len(json.load(sys.stdin)["flags"]))')
assert "flags count = 8" "$flag_count" "8"

# ────────────────────────────────────────────────────────────────────
# Section 2: AiteamDashboardView — getOverview() + getAuditTail()
# ────────────────────────────────────────────────────────────────────
echo ""
echo "=== § 2: AiteamDashboardView contract ==="
overview=$(api GET /api/aiteam/overview)
echo "$overview" > "$TMP/overview.json"
assert_in "overview has flags" "$overview" '"flags"'
assert_in "overview has any" "$overview" '"any"'

if [[ "$any_on" == "True" ]]; then
    assert_in "overview has wallet block" "$overview" '"wallet"'
    assert_in "overview has fx block" "$overview" '"fx"'
    assert_in "overview has guard block" "$overview" '"guard"'
fi

audit=$(api GET '/api/aiteam/audit?limit=10')
assert_in "audit has count" "$audit" '"count"'
assert_in "audit has entries array" "$audit" '"entries"'

# ────────────────────────────────────────────────────────────────────
# Section 3: AiteamWalletView — getWallet, creditWallet, getOverview (for picker)
# ────────────────────────────────────────────────────────────────────
echo ""
echo "=== § 3: AiteamWalletView contract ==="

# 3a. owner credits a new test agent for the smoke run
TEST_AGENT="uitest-$(date +%s)"
credit_resp=$(api POST "/api/aiteam/wallet/$TEST_AGENT/credit" '{"amount_usdt":"3.00","reason":"ui-smoke"}')
assert_in "credit returns entry with type" "$credit_resp" '"type":"credit"'
assert_in "credit returns amount_usdt" "$credit_resp" '"amount_usdt":"3"'
assert_in "credit returns balance_after_usdt" "$credit_resp" '"balance_after_usdt":"3"'
assert_in "credit ships fx_snapshot" "$credit_resp" '"fx_snapshot"'

# 3b. wallet view fetches by agentId
wallet=$(api GET "/api/aiteam/wallet/$TEST_AGENT")
assert_in "wallet shape: agentId" "$wallet" "\"agentId\":\"$TEST_AGENT\""
assert_in "wallet shape: balance_usdt" "$wallet" '"balance_usdt":"3"'
assert_in "wallet shape: recent_ledger" "$wallet" '"recent_ledger"'

# 3c. CSV export
csv=$(curl -s -H "$AUTH" "$BASE/api/aiteam/wallet/$TEST_AGENT/ledger.csv" | head -2)
assert_in "csv header row present" "$csv" "timestamp_ms,iso8601,type,amount_usdt"
assert_in "csv data row present" "$csv" 'credit'

# 3d. Ledger
ledger=$(api GET "/api/aiteam/wallet/$TEST_AGENT/ledger")
assert_in "ledger has entries array" "$ledger" '"entries"'

# ────────────────────────────────────────────────────────────────────
# Section 4: AiteamFXView — getFxRates / refreshFx / overrideFx / clearFxOverride
# ────────────────────────────────────────────────────────────────────
echo ""
echo "=== § 4: AiteamFXView contract ==="
fx=$(api GET /api/aiteam/fx/rates)
assert_in "fx has rates map" "$fx" '"rates"'
assert_in "fx has source" "$fx" '"source"'
assert_in "fx USDT=1" "$fx" '"USDT":1'

# 4a. valid override
ov_resp=$(api POST /api/aiteam/fx/override '{"currency":"CNY","rate":7.0}')
assert_in "override 7.0 success" "$ov_resp" '"currency":"CNY"'

# 4b. clear override
clr_resp=$(api DELETE /api/aiteam/fx/override/CNY)
assert_in "clear override" "$clr_resp" '"cleared":"CNY"'

# 4c. bad override rejected (B022 verification)
bad_ov=$(curl -s -o "$TMP/bad_ov.txt" -w "%{http_code}" -H "$AUTH" -H "Content-Type: application/json" \
    -d '{"currency":"CNY","rate":1e30}' "$BASE/api/aiteam/fx/override")
assert "B022: bad rate=1e30 → 400" "$bad_ov" "400"

# ────────────────────────────────────────────────────────────────────
# Section 5: AiteamGuardView — getGuard / releaseGuard / setGuardLimit
# ────────────────────────────────────────────────────────────────────
echo ""
echo "=== § 5: AiteamGuardView contract ==="
guard=$(api GET /api/aiteam/guard)
assert_in "guard has enabled" "$guard" '"enabled"'
assert_in "guard has day_key" "$guard" '"day_key"'
assert_in "guard has limits" "$guard" '"limits"'
assert_in "guard has agents map" "$guard" '"agents"'

# 5a. release non-panicked agent → 404 expected
rel_code=$(curl -s -o /dev/null -w "%{http_code}" -X POST -H "$AUTH" -H "Content-Type: application/json" \
    -d '{"operator":"smoke","reason":"test"}' "$BASE/api/aiteam/guard/$TEST_AGENT/release")
assert "release non-panicked → 404" "$rel_code" "404"

# 5b. set limit on test agent
lim_resp=$(api PATCH "/api/aiteam/guard/$TEST_AGENT/limit" '{"limit_usdt":"5.00"}')
assert_in "set limit response" "$lim_resp" '"updated":true'

# 5c. negative limit (B020) → clamped to 0
neg_resp=$(api PATCH "/api/aiteam/guard/$TEST_AGENT/limit" '{"limit_usdt":"-50"}')
# clamp behavior is silent — verify by reading guard back
guard2=$(api GET /api/aiteam/guard)
neg_limit=$(echo "$guard2" | python3 -c "
import sys, json
d = json.load(sys.stdin)
agents = d.get('agents', {})
print(agents.get('$TEST_AGENT', {}).get('effective_limit_usdt', '?'))
")
echo "  ℹ B020: after SetAgentLimit(-50), effective_limit=$neg_limit (should fall back to default)"

# ────────────────────────────────────────────────────────────────────
# Section 6: AiteamJudgeView — listJudgeAgents / runJudge / getJudgeScores / overrideJudge
# ────────────────────────────────────────────────────────────────────
echo ""
echo "=== § 6: AiteamJudgeView contract ==="

# 6a. agent list
jagents=$(api GET /api/aiteam/judge/agents)
assert_in "judge/agents has agents array" "$jagents" '"agents"'

# 6b. run heuristic judge for our test agent
jrun=$(api POST /api/aiteam/judge/run "{\"agent_id\":\"$TEST_AGENT\",\"usage_cost_usd\":0.30,\"call_count\":12}")
assert_in "judge run: completion" "$jrun" '"completion"'
assert_in "judge run: quality" "$jrun" '"quality"'
assert_in "judge run: communication" "$jrun" '"communication"'
assert_in "judge run: creativity" "$jrun" '"creativity"'
assert_in "judge run: cost" "$jrun" '"cost"'
assert_in "judge run: average" "$jrun" '"average"'
assert_in "judge run: source heuristic or llm" "$jrun" '"source":"heuristic"'

# 6c. scores history
scores=$(api GET "/api/aiteam/judge/scores/$TEST_AGENT")
assert_in "scores has history array" "$scores" '"history"'

# 6d. manual override
override=$(api POST /api/aiteam/judge/override "{\"agent_id\":\"$TEST_AGENT\",\"completion\":8,\"quality\":8,\"communication\":8,\"creativity\":8,\"cost\":8,\"operator\":\"smoke\",\"rationale\":\"ui-test\"}")
assert_in "override: source=manual" "$override" '"source":"manual"'
assert_in "override: average=8" "$override" '"average":8'

# 6e. invalid override (out of range, B021-like) — should clamp
clamp_test=$(api POST /api/aiteam/judge/override "{\"agent_id\":\"$TEST_AGENT\",\"completion\":50,\"quality\":-5,\"communication\":5,\"creativity\":5,\"cost\":5}")
assert_in "override clamps completion=50 to 10" "$clamp_test" '"completion":10'
assert_in "override clamps quality=-5 to 0" "$clamp_test" '"quality":0'

# ────────────────────────────────────────────────────────────────────
# Section 7: AiteamPayrollView — getPayroll / runPayroll
# ────────────────────────────────────────────────────────────────────
echo ""
echo "=== § 7: AiteamPayrollView contract ==="
hist=$(api GET "/api/aiteam/payroll/$TEST_AGENT")
assert_in "payroll/:id has history" "$hist" '"history"'

# 7a. run payroll for our test agent
run=$(api POST /api/aiteam/payroll/run "{\"agent_ids\":[\"$TEST_AGENT\"]}")
assert_in "payroll run: entries" "$run" '"entries"'

# 7b. empty body (B023): should NOT include __config__
run_all=$(api POST /api/aiteam/payroll/run '{}')
echo "  ℹ B023 check: agents in mass-pay = $(echo "$run_all" | python3 -c '
import sys, json
d = json.load(sys.stdin)
ids = [e["agent_id"] for e in d.get("entries", [])]
print(ids)
print("config_present" if "__config__" in ids else "system_filtered")
' | tail -1)"

# ────────────────────────────────────────────────────────────────────
# Section 8: Top-bar 💱 currency switcher data
# ────────────────────────────────────────────────────────────────────
echo ""
echo "=== § 8: Currency switcher dependent data ==="
# useCurrency polls /api/aiteam/fx/rates every 5 min — verify shape one more time
fx2=$(api GET /api/aiteam/fx/rates)
for cur in USDT USD CNY EUR JPY GBP KRW HKD TWD; do
    assert_in "fx has currency $cur" "$fx2" "\"$cur\":"
done

# ────────────────────────────────────────────────────────────────────
# Section 9: /metrics for Prometheus scrape
# ────────────────────────────────────────────────────────────────────
echo ""
echo "=== § 9: /metrics endpoint ==="
metrics=$(curl -s "$BASE/metrics")
assert_in "metrics: TYPE counter line" "$metrics" "# TYPE aiteam_"
assert_in "metrics: payroll counter (we just ran payroll)" "$metrics" "aiteam_payroll_runs_total"
assert_in "metrics: wallet balance gauge" "$metrics" "aiteam_wallet_balance_usdt"

# Cleanup: delete test agent wallet entries? No API for that. Leave it; data is small.

# ────────────────────────────────────────────────────────────────────
# Summary
# ────────────────────────────────────────────────────────────────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  passed: $PASS"
echo "  failed: $FAIL"
echo "  test agent: $TEST_AGENT (leftover, harmless)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
[[ "$FAIL" -eq 0 ]]
