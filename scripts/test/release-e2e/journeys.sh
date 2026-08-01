#!/usr/bin/env bash
# Release E2E 用户旅程：首次设置、Web 渠道、任务、Goal/Cron 与完整恢复。

journey_request() {
  local expected="$1" method="$2" url="$3" token="$4" body="${5:-}"
  local code
  local args=(-X "$method" -H "Authorization: Bearer $token")
  if [[ -n "$body" ]]; then
    args+=(-H 'Content-Type: application/json' -d "$body")
  fi
  code="$(http_code "$url" "${args[@]}")"
  [[ "$code" == "$expected" ]] || fail "用户旅程 ${method} ${url}：期望 HTTP ${expected}，实际 ${code}"
}

start_journey_service() {
  local home="$1" label="$2"
  local binary="$home/.local/bin/zyhive"
  local config="$home/.config/zyhive/zyhive.json"
  JOURNEY_TOKEN="$(json_value "$config" auth.token)"
  JOURNEY_PORT="$(json_value "$config" gateway.port)"
  JOURNEY_BASE="http://127.0.0.1:${JOURNEY_PORT}"
  JOURNEY_LOG="$TMP_ROOT/${label}.log"
  (
    cd "$home"
    exec env HOME="$home" "$binary" --serve --config "$config"
  ) >"$JOURNEY_LOG" 2>&1 &
  JOURNEY_PID=$!
  PIDS+=("$JOURNEY_PID")
  if ! wait_for_code "$JOURNEY_BASE/readyz" 200 60; then
    python3 -c 'import sys; print(sys.stdin.read()[-4000:])' < "$JOURNEY_LOG" >&2
    fail "用户旅程服务未就绪"
  fi
}

assert_journey_config() {
  local config="$1" model_base="$2"
  python3 - "$config" "$model_base" <<'PY'
import json
import os
import stat
import sys

path, model_base = sys.argv[1:]
data = json.load(open(path, encoding="utf-8"))
assert stat.S_IMODE(os.stat(path).st_mode) == 0o600
assert data["gateway"]["bind"] == "localhost"
assert len(data["providers"]) == 1
assert data["providers"][0]["provider"] == "ollama"
assert data["providers"][0]["baseUrl"] == model_base
assert data["providers"][0]["apiKey"] == "release-e2e-placeholder"
assert len(data["models"]) == 1
assert data["models"][0]["id"] == "default"
assert data["models"][0]["model"] == "e2e-model"
assert data["models"][0]["isDefault"] is True
PY
}

assert_masked_setup() {
  local base="$1" token="$2"
  journey_request 200 GET "$base/api/config" "$token"
  python3 - "$TMP_ROOT/http-body" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
assert data["auth"]["token"] == "***"
assert data["providers"][0]["apiKey"].endswith("***")
assert data["providers"][0]["apiKey"] != "release-e2e-placeholder"
PY
}

seed_user_journeys() {
  local base="$1" token="$2"
  journey_request 201 POST "$base/api/agents" "$token" \
    '{"id":"journey-worker","name":"Journey Worker","modelId":"default","toolPolicy":{"profile":"minimal"}}'
  journey_request 200 PUT "$base/api/agents/journey-worker/files/persist.txt" "$token" \
    '{"content":"journey-persistent-data"}'
  journey_request 200 PUT "$base/api/agents/journey-worker/channels" "$token" \
    '[{"id":"web-e2e","name":"E2E Web","type":"web","enabled":true,"status":"untested","config":{"password":"e2e-password","title":"E2E Assistant","welcomeMsg":"Welcome"}}]'

  code="$(http_code "$base/pub/chat/journey-worker/web-e2e/history?sessionToken=visitor-1" \
    -H 'X-Chat-Password: wrong')"
  [[ "$code" == "401" ]] || fail "Web 渠道错误密码未被拒绝"
  code="$(http_code "$base/pub/chat/journey-worker/web-e2e/history?sessionToken=visitor-1" \
    -H 'X-Chat-Password: e2e-password')"
  [[ "$code" == "200" ]] || fail "Web 渠道正确密码不可访问"

  curl -fsS "$base/pub/chat/journey-worker/web-e2e/stream" \
    -H 'Content-Type: application/json' \
    -H 'X-Chat-Password: e2e-password' \
    -d '{"message":"Return exactly TASK_E2E_OK","sessionToken":"visitor-1"}' \
    > "$TMP_ROOT/public-stream"
  grep -q 'TASK_E2E_OK' "$TMP_ROOT/public-stream" || fail "Web 渠道未返回确定性模型结果"

  journey_request 201 POST "$base/api/goals" "$token" \
    '{"id":"journey-goal","title":"Release Journey","type":"team","agentIds":["journey-worker"],"status":"active","progress":10,"milestones":[{"id":"m1","title":"verify","done":false}]}'
  journey_request 200 PATCH "$base/api/goals/journey-goal/progress" "$token" '{"progress":60}'
  journey_request 200 PATCH "$base/api/goals/journey-goal/milestones/m1" "$token" '{"done":true}'

  journey_request 201 POST "$base/api/cron" "$token" \
    '{"id":"journey-cron","name":"Journey Cron","enabled":false,"agentId":"journey-worker","schedule":{"kind":"cron","expr":"0 9 * * *","tz":"UTC"},"payload":{"kind":"agentTurn","message":"journey"},"delivery":{"mode":"none"}}'

  journey_request 201 POST "$base/api/projects" "$token" \
    '{"id":"journey-project","name":"Journey Project","description":"backup coverage"}'
  journey_request 200 PUT "$base/api/projects/journey-project/files/result.txt" "$token" \
    '{"content":"project-persistent-data"}'

  journey_request 201 POST "$base/api/tasks" "$token" \
    '{"agentId":"journey-worker","task":"Return exactly TASK_E2E_OK","label":"release-e2e","taskType":"system","deliverable":"TASK_E2E_OK"}'
  JOURNEY_TASK_ID="$(json_value "$TMP_ROOT/http-body" id)"
  local status=""
  for _ in $(seq 1 80); do
    journey_request 200 GET "$base/api/tasks/$JOURNEY_TASK_ID" "$token"
    status="$(json_value "$TMP_ROOT/http-body" status)"
    [[ "$status" == "done" || "$status" == "error" ]] && break
    sleep 0.25
  done
  [[ "$status" == "done" ]] || fail "核心任务未成功完成，状态：${status:-未知}"
  grep -q 'TASK_E2E_OK' "$TMP_ROOT/http-body" || fail "核心任务结果不符合预期"
}

verify_user_journeys() {
  local base="$1" token="$2" task_id="$3"
  journey_request 200 GET "$base/api/agents/journey-worker" "$token"
  journey_request 200 GET "$base/api/agents/journey-worker/files/persist.txt" "$token"
  grep -q 'journey-persistent-data' "$TMP_ROOT/http-body" || fail "恢复后成员工作区内容错误"
  journey_request 200 GET "$base/api/goals/journey-goal" "$token"
  [[ "$(json_value "$TMP_ROOT/http-body" progress)" == "60" ]] || fail "恢复后 Goal 进度错误"
  journey_request 200 GET "$base/api/projects/journey-project" "$token"
  journey_request 200 GET "$base/api/projects/journey-project/files/result.txt" "$token"
  grep -q 'project-persistent-data' "$TMP_ROOT/http-body" || fail "恢复后项目文件内容错误"
  journey_request 200 GET "$base/api/tasks/$task_id" "$token"
  [[ "$(json_value "$TMP_ROOT/http-body" status)" == "done" ]] || fail "恢复后任务状态错误"
  journey_request 200 GET "$base/api/cron?agentId=journey-worker" "$token"
  grep -q 'journey-cron' "$TMP_ROOT/http-body" || fail "恢复后 Cron 丢失"
  code="$(http_code "$base/pub/chat/journey-worker/web-e2e/info")"
  [[ "$code" == "200" ]] || fail "恢复后 Web 渠道不可访问"
}

verify_backup_restore_journey() {
  local home="$1" model_base="$2"
  local config="$home/.config/zyhive/zyhive.json"
  local binary="$home/.local/bin/zyhive"
  local archive="$TMP_ROOT/journey-backup.tar.gz"

  (
    cd "$home"
    env HOME="$home" "$binary" backup create \
      --config "$config" --workdir "$home" --output "$archive"
    env HOME="$home" "$binary" backup inspect --input "$archive" >/dev/null
  )

  rm -rf "$home/.local/share/zyhive/agents" "$home/projects" "$home/cron"
  python3 - "$config" <<'PY'
import json
import sys

path = sys.argv[1]
data = json.load(open(path, encoding="utf-8"))
data["auth"]["token"] = "destroyed-token"
data["providers"] = []
data["models"] = []
with open(path, "w", encoding="utf-8") as handle:
    json.dump(data, handle)
PY
  chmod 600 "$config"
  (
    cd "$home"
    env HOME="$home" "$binary" backup restore \
      --config "$config" --workdir "$home" --input "$archive" --yes --no-service
  )
  assert_journey_config "$config" "$model_base"

  start_journey_service "$home" "journey-restored"
  verify_user_journeys "$JOURNEY_BASE" "$JOURNEY_TOKEN" "$JOURNEY_TASK_ID"
  stop_process "$JOURNEY_PID"
  echo "  ✅ 完整备份、数据破坏、恢复和重启复核通过"
}

verify_release_journeys() {
  local installer="$1" home="$2" port="$3" install_base="$4" model_base="$5"
  local config="$home/.config/zyhive/zyhive.json"
  local config_hash

  echo "▶ [Release E2E/local] 验证首次设置、渠道和核心任务"
  mkdir -p "$home"
  env HOME="$home" ZYHIVE_INSTALL_BASE="$install_base" ZYHIVE_DISABLE_FALLBACK=1 \
    bash "$installer" --no-root --no-service --yes --bind localhost --port "$port" \
      --provider ollama --api-key release-e2e-placeholder \
      --base-url "$model_base" --model e2e-model \
      >"$TMP_ROOT/journey-install.log" 2>&1 \
    || fail "带模型首次设置失败"

  assert_journey_config "$config" "$model_base"
  config_hash="$(sha256_file "$config")"
  env HOME="$home" ZYHIVE_INSTALL_BASE="$install_base" ZYHIVE_DISABLE_FALLBACK=1 \
    bash "$installer" --no-root --no-service --yes --bind localhost --port "$port" \
      >"$TMP_ROOT/journey-reinstall.log" 2>&1 \
    || fail "首次设置后的重复安装失败"
  [[ "$(sha256_file "$config")" == "$config_hash" ]] || fail "重复安装覆盖了首次设置"

  start_journey_service "$home" "journey-seed"
  assert_masked_setup "$JOURNEY_BASE" "$JOURNEY_TOKEN"
  seed_user_journeys "$JOURNEY_BASE" "$JOURNEY_TOKEN"
  stop_process "$JOURNEY_PID"

  start_journey_service "$home" "journey-restart"
  verify_user_journeys "$JOURNEY_BASE" "$JOURNEY_TOKEN" "$JOURNEY_TASK_ID"
  stop_process "$JOURNEY_PID"
  echo "  ✅ 首次设置、Web 渠道、任务、Goal/Cron 和重启保持全部通过"
}
