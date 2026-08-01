#!/usr/bin/env bash
# Release 全流程测试：产物校验 → 全新安装 → 服务/API → 旧版更新 → 备份确认。
set -euo pipefail

MODE="${1:-}"
VERSION="${2:-}"
ARG3="${3:-}"
REPO="${ZYHIVE_REPO:-Zyling-ai/ZyHive}"
REPO_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
INSTALL_SOURCE="$REPO_ROOT/scripts/install.sh"
SERVER_SCRIPT="$REPO_ROOT/scripts/test/release-e2e/server.py"
TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/zyhive-release-e2e.XXXXXX")"
PIDS=()

cleanup() {
  local pid
  for pid in "${PIDS[@]:-}"; do
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  done
  rm -rf "$TMP_ROOT"
}
trap cleanup EXIT

fail() {
  echo "❌ $*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "缺少命令: $1"
}

for command_name in bash curl python3; do
  require_command "$command_name"
done

case "$(uname -s)" in
  Linux) OS="linux" ;;
  Darwin) OS="darwin" ;;
  *) fail "不支持的测试系统: $(uname -s)" ;;
esac

case "$(uname -m)" in
  x86_64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) fail "不支持的测试架构: $(uname -m)" ;;
esac

BINARY_NAME="zyhive-${OS}-${ARCH}"

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

version_of() {
  "$1" --version 2>/dev/null | awk '{print $NF}'
}

assert_version() {
  local binary="$1" expected="$2" actual
  actual="$(version_of "$binary")"
  [[ "$actual" == "$expected" ]] || fail "版本不匹配：期望 ${expected}，实际 ${actual:-未知}"
}

random_port() {
  python3 - <<'PY'
import socket

sock = socket.socket()
sock.bind(("127.0.0.1", 0))
print(sock.getsockname()[1])
sock.close()
PY
}

json_value() {
  local file="$1" expression="$2"
  python3 - "$file" "$expression" <<'PY'
import json
import sys

value = json.load(open(sys.argv[1], encoding="utf-8"))
for key in sys.argv[2].split("."):
    value = value[key]
print(value)
PY
}

file_mode() {
  if stat -f '%Lp' "$1" >/dev/null 2>&1; then
    stat -f '%Lp' "$1"
  else
    stat -c '%a' "$1"
  fi
}

http_code() {
  local url="$1"
  shift
  curl -sS -o "$TMP_ROOT/http-body" -w '%{http_code}' "$@" "$url"
}

wait_for_code() {
  local url="$1" expected="$2" attempts="${3:-60}" code=""
  shift 3 || true
  for _ in $(seq 1 "$attempts"); do
    code="$(http_code "$url" "$@" 2>/dev/null || true)"
    [[ "$code" == "$expected" ]] && return 0
    sleep 0.5
  done
  echo "最后响应（HTTP ${code:-000}）：" >&2
  test -f "$TMP_ROOT/http-body" && python3 -c 'import sys; print(sys.stdin.read()[:1000])' < "$TMP_ROOT/http-body" >&2
  return 1
}

stop_process() {
  local pid="$1"
  kill "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
}

basic_smoke() {
  local home="$1" expected_version="$2" label="$3" data_mode="${4:-none}"
  local binary="$home/.local/bin/zyhive"
  local config="$home/.config/zyhive/zyhive.json"
  local token port base log pid code version

  [[ -x "$binary" ]] || fail "${label}：安装后的二进制不存在"
  [[ -f "$config" ]] || fail "${label}：配置文件不存在"
  [[ "$(file_mode "$config")" == "600" ]] || fail "${label}：配置文件权限必须为 0600"
  assert_version "$binary" "$expected_version"

  token="$(json_value "$config" auth.token)"
  port="$(json_value "$config" gateway.port)"
  base="http://127.0.0.1:${port}"
  log="$TMP_ROOT/${label}.log"

  (
    cd "$home"
    exec env HOME="$home" "$binary" --serve --config "$config"
  ) >"$log" 2>&1 &
  pid=$!
  PIDS+=("$pid")

  if ! wait_for_code "$base/healthz" 200 60; then
    echo "服务日志：" >&2
    python3 -c 'import sys; print(sys.stdin.read()[-4000:])' < "$log" >&2
    fail "${label}：服务未通过健康检查"
  fi

  curl -fsS "$base/api/version" > "$TMP_ROOT/version.json"
  version="$(json_value "$TMP_ROOT/version.json" version)"
  [[ "$version" == "$expected_version" ]] || fail "${label}：API 版本错误 $version"
  code="$(http_code "$base/api/update/status")"
  [[ "$code" == "200" ]] || fail "${label}：更新状态接口失败，HTTP $code"

  code="$(http_code "$base/api/agents")"
  [[ "$code" == "401" ]] || fail "${label}：未鉴权访问 /api/agents 应返回 401，实际 $code"
  code="$(http_code "$base/api/health" -H "Authorization: Bearer wrong-token")"
  [[ "$code" == "401" ]] || fail "${label}：错误令牌访问 /api/health 应返回 401，实际 $code"
  code="$(http_code "$base/api/health" -H "Authorization: Bearer $token")"
  [[ "$code" == "200" ]] || fail "${label}：鉴权健康接口失败，HTTP $code"
  code="$(http_code "$base/api/status" -H "Authorization: Bearer $token")"
  [[ "$code" == "200" ]] || fail "${label}：系统状态接口失败，HTTP $code"

  code="$(http_code "$base/api/agents" -H "Authorization: Bearer $token")"
  [[ "$code" == "200" ]] || fail "${label}：鉴权访问 /api/agents 失败，HTTP $code"
  python3 - "$TMP_ROOT/http-body" <<'PY'
import json
import sys

assert isinstance(json.load(open(sys.argv[1], encoding="utf-8")), list)
PY

  code="$(http_code "$base/api/config" -H "Authorization: Bearer $token")"
  [[ "$code" == "200" ]] || fail "${label}：读取 /api/config 失败，HTTP $code"
  [[ "$(json_value "$TMP_ROOT/http-body" auth.token)" == "***" ]] \
    || fail "${label}：配置 API 未遮蔽管理令牌"

  wait_for_code "$base/readyz" 200 30 || fail "${label}：服务未进入 ready 状态"
  code="$(http_code "$base/")"
  [[ "$code" == "200" ]] || fail "${label}：嵌入管理界面不可访问，HTTP $code"

  case "$data_mode" in
    crud|seed)
      code="$(http_code "$base/api/agents" \
        -X POST \
        -H "Authorization: Bearer $token" \
        -H 'Content-Type: application/json' \
        -d '{"id":"release-smoke","name":"Release Smoke"}')"
      [[ "$code" == "201" ]] || fail "${label}：创建测试成员失败，HTTP $code"

      code="$(http_code "$base/api/agents/release-smoke" \
        -X PATCH \
        -H "Authorization: Bearer $token" \
        -H 'Content-Type: application/json' \
        -d '{"name":"Release Smoke Updated"}')"
      [[ "$code" == "200" ]] || fail "${label}：更新测试成员失败，HTTP $code"

      code="$(http_code "$base/api/agents/release-smoke/files/persist.txt" \
        -X PUT \
        -H "Authorization: Bearer $token" \
        -H 'Content-Type: text/plain' \
        --data-binary 'release-e2e-persistent-data')"
      [[ "$code" == "200" ]] || fail "${label}：写入工作区文件失败，HTTP $code"
      ;;
    verify)
      code="$(http_code "$base/api/agents/release-smoke" -H "Authorization: Bearer $token")"
      [[ "$code" == "200" ]] || fail "${label}：更新后测试成员丢失，HTTP $code"
      [[ "$(json_value "$TMP_ROOT/http-body" name)" == "Release Smoke Updated" ]] \
        || fail "${label}：更新后成员字段未保留"

      code="$(http_code "$base/api/agents/release-smoke/files/persist.txt" \
        -H "Authorization: Bearer $token")"
      [[ "$code" == "200" ]] || fail "${label}：更新后工作区文件丢失，HTTP $code"
      [[ "$(json_value "$TMP_ROOT/http-body" content)" == "release-e2e-persistent-data" ]] \
        || fail "${label}：更新后工作区文件内容错误"
      ;;
    none) ;;
    *) fail "未知数据测试模式: $data_mode" ;;
  esac

  if [[ "$data_mode" == "crud" || "$data_mode" == "verify" ]]; then
    code="$(http_code "$base/api/agents/release-smoke" \
      -X DELETE -H "Authorization: Bearer $token")"
    [[ "$code" == "200" ]] || fail "${label}：删除测试成员失败，HTTP $code"
    code="$(http_code "$base/api/agents/release-smoke" -H "Authorization: Bearer $token")"
    [[ "$code" == "404" ]] || fail "${label}：删除后的成员仍可访问，HTTP $code"
  fi

  stop_process "$pid"
  echo "  ✅ ${label}：安装、启动、鉴权、CRUD、持久化与管理界面通过"
}

run_installer() {
  local installer="$1" home="$2" port="$3" install_base="$4" version_override="${5:-}"
  local log="$TMP_ROOT/install-$(basename "$home").log"
  local env_args=(
    "HOME=$home"
    "ZYHIVE_INSTALL_BASE=$install_base"
    "ZYHIVE_DISABLE_FALLBACK=1"
  )
  [[ -n "$version_override" ]] && env_args+=("ZYHIVE_VERSION=$version_override")

  mkdir -p "$home"
  if ! env "${env_args[@]}" bash "$installer" \
      --no-root --no-service --yes --skip-setup --bind localhost --port "$port" \
      >"$log" 2>&1; then
    python3 -c 'import sys; print(sys.stdin.read()[-5000:])' < "$log" >&2
    fail "安装脚本执行失败"
  fi
}

run_installer_expect_failure() {
  local installer="$1" home="$2" port="$3" install_base="$4"
  local log="$TMP_ROOT/install-expected-failure.log"
  if env \
      "HOME=$home" \
      "ZYHIVE_INSTALL_BASE=$install_base" \
      "ZYHIVE_DISABLE_FALLBACK=1" \
      bash "$installer" \
      --no-root --no-service --yes --skip-setup --bind localhost --port "$port" \
      >"$log" 2>&1; then
    fail "损坏发布源未能阻断安装"
  fi
  if ! python3 -c 'import sys; raise SystemExit(0 if "SHA-256 校验失败" in sys.stdin.read() else 1)' < "$log"; then
    python3 -c 'import sys; print(sys.stdin.read()[-3000:])' < "$log" >&2
    fail "损坏发布源的失败原因不符合预期"
  fi
}

write_minimal_config() {
  local home="$1" port="$2"
  mkdir -p "$home/.config/zyhive" "$home/.local/share/zyhive/agents"
  python3 - "$home/.config/zyhive/zyhive.json" "$home" "$port" <<'PY'
import json
import pathlib
import sys

path, home, port = sys.argv[1], sys.argv[2], int(sys.argv[3])
config = {
    "configVersion": 3,
    "gateway": {"port": port, "bind": "localhost"},
    "agents": {"dir": f"{home}/.local/share/zyhive/agents"},
    "providers": [],
    "models": [],
    "channels": [],
    "tools": [],
    "skills": [],
    "auth": {"mode": "token", "token": "release-smoke-token"},
}
with open(path, "w", encoding="utf-8") as handle:
    json.dump(config, handle)
pathlib.Path(path).chmod(0o600)
PY
}

prepare_fixture_release() {
  local fixture_root="$1" version="$2" binary="$3"
  local release_dir="$fixture_root/dl/$version"
  mkdir -p "$release_dir"
  cp "$binary" "$release_dir/$BINARY_NAME"
  printf '%s  %s\n' "$(sha256_file "$binary")" "$BINARY_NAME" > "$release_dir/SHA256SUMS"
}

set_fixture_latest() {
  local fixture_root="$1" version="$2"
  printf '{"version":"%s"}\n' "$version" > "$fixture_root/latest.new"
  mv "$fixture_root/latest.new" "$fixture_root/latest"
}

start_fixture_server() {
  local fixture_root="$1"
  local port_file="$TMP_ROOT/fixture-port"
  python3 "$SERVER_SCRIPT" "$fixture_root" "$port_file" >"$TMP_ROOT/fixture-server.log" 2>&1 &
  local pid=$!
  PIDS+=("$pid")
  for _ in $(seq 1 50); do
    [[ -s "$port_file" ]] && break
    sleep 0.1
  done
  [[ -s "$port_file" ]] || fail "本地 Release 服务启动失败"
  FIXTURE_BASE="http://127.0.0.1:$(<"$port_file")"
}

write_pending_update() {
  local binary="$1" backup="$2" health_url="$3" old_version="$4"
  local expected_version="$5" token="$6" pid="$7"
  python3 - "$binary.update-pending.json" "$binary" "$backup" "$health_url" \
    "$old_version" "$expected_version" "$token" "$pid" <<'PY'
import datetime
import json
import pathlib
import sys

path, binary, backup, health_url, old_version, expected_version, token, pid = sys.argv[1:]
payload = {
    "token": token,
    "oldVersion": old_version,
    "expectedVersion": expected_version,
    "binaryPath": str(pathlib.Path(binary).resolve()),
    "backupPath": str(pathlib.Path(backup).resolve()),
    "healthUrl": health_url,
    "pid": int(pid),
    "createdAt": datetime.datetime.now(datetime.timezone.utc).isoformat(),
}
target = pathlib.Path(path)
target.write_text(json.dumps(payload), encoding="utf-8")
target.chmod(0o600)
PY
}

verify_update_watchdog() {
  local current_binary="$1"
  local root="$TMP_ROOT/watchdog"
  local home="$root/home"
  local binary="$root/zyhive"
  local backup="$root/zyhive.bak"
  local config="$home/.config/zyhive/zyhive.json"
  local token="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  local port base log pid result_stage

  echo "▶ [Release E2E/local] 验证更新后健康确认与自动回滚"
  mkdir -p "$root" "$home"
  cp "$current_binary" "$binary"
  cp "$current_binary" "$backup"
  chmod +x "$binary" "$backup"

  port="$(random_port)"
  base="http://127.0.0.1:${port}"
  write_minimal_config "$home" "$port"
  log="$TMP_ROOT/watchdog-healthy.log"
  (
    cd "$home"
    exec env HOME="$home" "$binary" --serve --config "$config"
  ) >"$log" 2>&1 &
  pid=$!
  PIDS+=("$pid")
  wait_for_code "$base/healthz" 200 60 || fail "更新守护成功路径：服务未通过健康检查"

  write_pending_update "$binary" "$backup" "$base/healthz" "$VERSION" "$VERSION" "$token" "$pid"
  env ZYHIVE_RELEASE_E2E=1 ZYHIVE_UPDATE_HEALTH_TIMEOUT=1s \
    "$backup" __update-watchdog "$token"
  [[ ! -e "$binary.update-pending.json" ]] || fail "健康确认后仍残留 pending 记录"
  [[ ! -e "$binary.update-result.json" ]] || fail "健康确认被错误标记为回滚"
  assert_version "$binary" "$VERSION"
  stop_process "$pid"

  printf '#!/bin/sh\nexit 1\n' > "$binary"
  chmod +x "$binary"
  port="$(random_port)"
  write_pending_update "$binary" "$backup" "http://127.0.0.1:${port}/healthz" \
    "$VERSION" "$VERSION" "$token" 0
  env ZYHIVE_RELEASE_E2E=1 ZYHIVE_UPDATE_HEALTH_TIMEOUT=200ms \
    "$backup" __update-watchdog "$token"
  assert_version "$binary" "$VERSION"
  [[ ! -e "$binary.update-pending.json" ]] || fail "回滚后仍残留 pending 记录"
  [[ -f "$binary.update-result.json" ]] || fail "回滚后缺少结果记录"
  result_stage="$(json_value "$binary.update-result.json" stage)"
  [[ "$result_stage" == "rolledback" ]] || fail "回滚结果状态错误：$result_stage"
  echo "  ✅ 更新守护：健康版本已确认，失联版本已原子恢复"
}

run_local() {
  local dist_dir="$ARG3"
  local current_binary="$dist_dir/$BINARY_NAME"
  local old_version="00.0.0v0"
  local fixture_root="$TMP_ROOT/fixture"
  local old_binary="$TMP_ROOT/$BINARY_NAME.old"
  local base fresh_home upgrade_home fresh_port upgrade_port config_before config_after
  local current_sums valid_sums

  require_command go
  [[ -x "$current_binary" ]] || fail "缺少当前平台发布产物: $current_binary"
  assert_version "$current_binary" "$VERSION"

  echo "▶ [Release E2E/local] 准备隔离 Release 仓库"
  (
    cd "$REPO_ROOT"
    CGO_ENABLED=0 GOOS="$OS" GOARCH="$ARCH" \
      go build -trimpath -ldflags="-s -w -X main.Version=${old_version}" \
      -o "$old_binary" ./cmd/aipanel/
  )
  prepare_fixture_release "$fixture_root" "$old_version" "$old_binary"
  prepare_fixture_release "$fixture_root" "$VERSION" "$current_binary"
  set_fixture_latest "$fixture_root" "$VERSION"
  start_fixture_server "$fixture_root"
  base="$FIXTURE_BASE"

  verify_update_watchdog "$current_binary"

  echo "▶ [Release E2E/local] 验证全新安装"
  fresh_home="$TMP_ROOT/home-fresh"
  fresh_port="$(random_port)"
  run_installer "$INSTALL_SOURCE" "$fresh_home" "$fresh_port" "$base"
  basic_smoke "$fresh_home" "$VERSION" "fresh-install" crud

  echo "▶ [Release E2E/local] 验证旧版更新与备份"
  upgrade_home="$TMP_ROOT/home-upgrade"
  upgrade_port="$(random_port)"
  set_fixture_latest "$fixture_root" "$old_version"
  run_installer "$INSTALL_SOURCE" "$upgrade_home" "$upgrade_port" "$base"
  assert_version "$upgrade_home/.local/bin/zyhive" "$old_version"
  basic_smoke "$upgrade_home" "$old_version" "old-version-seed" seed
  config_before="$(sha256_file "$upgrade_home/.config/zyhive/zyhive.json")"

  echo "▶ [Release E2E/local] 验证损坏校验和不会替换旧版"
  current_sums="$fixture_root/dl/$VERSION/SHA256SUMS"
  valid_sums="$TMP_ROOT/valid-SHA256SUMS"
  cp "$current_sums" "$valid_sums"
  printf '%064d  %s\n' 0 "$BINARY_NAME" > "$current_sums"
  set_fixture_latest "$fixture_root" "$VERSION"
  run_installer_expect_failure "$INSTALL_SOURCE" "$upgrade_home" "$upgrade_port" "$base"
  assert_version "$upgrade_home/.local/bin/zyhive" "$old_version"
  [[ ! -e "$upgrade_home/.local/bin/zyhive.new" ]] || fail "校验失败后残留 .new 文件"
  cp "$valid_sums" "$current_sums"

  echo "▶ [Release E2E/local] 执行有效更新并验证持久数据"
  run_installer "$INSTALL_SOURCE" "$upgrade_home" "$upgrade_port" "$base"
  assert_version "$upgrade_home/.local/bin/zyhive" "$VERSION"
  assert_version "$upgrade_home/.local/bin/zyhive.bak" "$old_version"
  config_after="$(sha256_file "$upgrade_home/.config/zyhive/zyhive.json")"
  [[ "$config_before" == "$config_after" ]] || fail "更新流程修改了已有配置"

  run_installer "$INSTALL_SOURCE" "$upgrade_home" "$upgrade_port" "$base"
  basic_smoke "$upgrade_home" "$VERSION" "upgrade-install" verify
  echo "✅ Release 本地全流程测试通过"
}

download_release_asset() {
  local version="$1" asset="$2" destination="$3"
  curl -fsSL --retry 3 \
    "https://github.com/${REPO}/releases/download/${version}/${asset}" \
    -o "$destination"
}

wait_for_latest_release() {
  local expected="$1" payload tag draft attempt
  local attempts="${ZYHIVE_RELEASE_WAIT_ATTEMPTS:-150}"
  [[ "$attempts" =~ ^[0-9]+$ ]] || attempts=150
  for ((attempt = 1; attempt <= attempts; attempt++)); do
    if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
      payload="$(gh api "repos/${REPO}/releases/tags/${expected}" 2>/dev/null || true)"
    else
      payload="$(
        curl -fsSL \
          -H "Accept: application/vnd.github+json" \
          -H "Cache-Control: no-cache" \
          "https://api.github.com/repos/${REPO}/releases/tags/${expected}?nocache=$(date +%s)-${attempt}" \
          2>/dev/null || true
      )"
    fi
    if [[ -n "$payload" ]]; then
      read -r tag draft < <(
        python3 -c 'import json,sys; data=json.load(sys.stdin); print(data.get("tag_name", ""), str(data.get("draft", True)).lower())' <<<"$payload"
      )
      [[ "$tag" == "$expected" && "$draft" == "false" ]] && return 0
    fi
    sleep 2
  done
  fail "GitHub 正式 Release 在五分钟内仍不可读取：$expected"
}

wait_for_install_mirror() {
  local expected="$1" payload tag
  for _ in $(seq 1 210); do
    payload="$(curl -fsSL "https://install.zyling.ai/latest" 2>/dev/null || true)"
    if [[ -n "$payload" ]]; then
      tag="$(python3 -c 'import json,sys; print(json.load(sys.stdin).get("version", ""))' <<<"$payload")"
      [[ "$tag" == "$expected" ]] && return 0
    fi
    sleep 2
  done
  fail "install.zyling.ai/latest 在七分钟内未指向 $expected"
}

run_online() {
  local previous_version="$ARG3"
  local installer="$TMP_ROOT/install.sh"
  local fresh_home="$TMP_ROOT/home-online-fresh"
  local upgrade_home="$TMP_ROOT/home-online-upgrade"
  local fresh_port upgrade_port old_binary sums expected actual
  local config_before config_after bootstrap

  echo "▶ [Release E2E/online] 等待 GitHub latest Release"
  wait_for_latest_release "$VERSION"
  echo "▶ [Release E2E/online] 等待安装镜像切换并获取正式安装器"
  wait_for_install_mirror "$VERSION"
  bootstrap="$(curl -fsSL https://install.zyling.ai/install)"
  [[ "$bootstrap" == *"install.zyling.ai/zyhive.sh"* ]] || fail "通用安装入口未返回 ZyHive 引导脚本"
  curl -fsSL https://install.zyling.ai/zyhive.sh -o "$installer"
  chmod +x "$installer"

  echo "▶ [Release E2E/online] 验证正式 Release 全新安装"
  fresh_port="$(random_port)"
  run_installer "$installer" "$fresh_home" "$fresh_port" "https://install.zyling.ai"
  basic_smoke "$fresh_home" "$VERSION" "online-fresh" crud

  if [[ -z "$previous_version" || "$previous_version" == "$VERSION" ]]; then
    echo "  ℹ️ 未提供可用旧版本，跳过真实旧版更新"
    echo "✅ Release 线上安装测试通过"
    return
  fi

  echo "▶ [Release E2E/online] 验证 $previous_version → $VERSION 真实更新"
  mkdir -p "$upgrade_home/.local/bin"
  old_binary="$upgrade_home/.local/bin/zyhive"
  sums="$TMP_ROOT/previous-SHA256SUMS"
  download_release_asset "$previous_version" "$BINARY_NAME" "$old_binary"
  download_release_asset "$previous_version" SHA256SUMS "$sums"
  chmod +x "$old_binary"
  expected="$(awk -v name="$BINARY_NAME" '$2 == name || $2 == "*" name {print $1; exit}' "$sums")"
  actual="$(sha256_file "$old_binary")"
  [[ -n "$expected" && "$actual" == "$expected" ]] || fail "旧版发布产物校验失败"
  assert_version "$old_binary" "$previous_version"

  upgrade_port="$(random_port)"
  write_minimal_config "$upgrade_home" "$upgrade_port"
  basic_smoke "$upgrade_home" "$previous_version" "online-old-seed" seed
  config_before="$(sha256_file "$upgrade_home/.config/zyhive/zyhive.json")"
  run_installer "$installer" "$upgrade_home" "$upgrade_port" "https://install.zyling.ai"
  assert_version "$old_binary" "$VERSION"
  assert_version "$upgrade_home/.local/bin/zyhive.bak" "$previous_version"
  config_after="$(sha256_file "$upgrade_home/.config/zyhive/zyhive.json")"
  [[ "$config_before" == "$config_after" ]] || fail "线上更新修改了已有配置"
  basic_smoke "$upgrade_home" "$VERSION" "online-upgrade" verify
  echo "✅ Release 线上安装与真实更新测试通过"
}

if [[ -z "$MODE" || -z "$VERSION" ]]; then
  fail "用法: $0 local <version> <dist-dir> | online <version> [previous-version]"
fi

case "$MODE" in
  local) run_local ;;
  online) run_online ;;
  *) fail "未知模式: $MODE" ;;
esac
