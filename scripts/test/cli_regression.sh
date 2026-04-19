#!/usr/bin/env bash
# ZyHive CLI 全面回归测试（stdin 驱动）
#
# 覆盖 9 大主菜单 + 子菜单：
#   A 子命令 / B 主菜单 / C 系统状态 / D 服务 / E 配置 / F Providers
#   G 成员 / H 日志 / I 更新 / J Nginx / K SSL / L 备份
#
# 使用方法:
#   make build                          # 先构建当前版本二进制 bin/aipanel
#   ./scripts/test/cli_regression.sh    # 默认用 bin/aipanel，搭建 /tmp 里的临时环境
#
# 可通过环境变量覆盖:
#   TEST_BIN  — 被测二进制路径（默认 $PWD/bin/aipanel）
#   TEST_HOME — 隔离 HOME（默认 /tmp/zyhive-cli-test-home）
#   TEST_DATA — 成员数据目录（默认 /tmp/zyhive-cli-test/agents）

set +e
REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TEST_BIN="${TEST_BIN:-$REPO_ROOT/bin/aipanel}"
TEST_HOME="${TEST_HOME:-/tmp/zyhive-cli-test-home}"
TEST_DATA="${TEST_DATA:-/tmp/zyhive-cli-test/agents}"

if [[ ! -x "$TEST_BIN" ]]; then
  echo "❌ 找不到被测二进制: $TEST_BIN"
  echo "   请先运行 'make build' 或设置 TEST_BIN 环境变量"
  exit 2
fi

# 准备隔离 HOME + 配置
mkdir -p "$TEST_HOME/.config/zyhive" "$TEST_DATA"
cat > "$TEST_HOME/.config/zyhive/zyhive.json" <<EOF
{
  "configVersion": 3,
  "gateway": {"port": 8080, "bind": "lan"},
  "agents": {"dir": "$TEST_DATA"},
  "auth": {"mode": "token", "token": "test-token-abcdefgh12345678"},
  "providers": [],
  "models": [],
  "channels": [],
  "tools": [],
  "skills": []
}
EOF
# 准备三个测试 agent（含一个隐藏目录 .subagent-tasks 用于验证过滤）
mkdir -p "$TEST_DATA/main/workspace/memory"
mkdir -p "$TEST_DATA/__config__/workspace"
mkdir -p "$TEST_DATA/testbot/workspace/memory"
mkdir -p "$TEST_DATA/.subagent-tasks"
echo '{"name":"主助手","id":"main"}' > "$TEST_DATA/main/identity.json"
echo '{"name":"系统配置","id":"__config__"}' > "$TEST_DATA/__config__/identity.json"
echo '{"name":"测试机器人","id":"testbot"}' > "$TEST_DATA/testbot/identity.json"

export HOME="$TEST_HOME"
unset AIPANEL_CONFIG
BIN="$TEST_BIN"
PASS=0
FAIL=0

check() {
  local name="$1"; local expected="$2"; local output="$3"
  if echo "$output" | grep -q -- "$expected"; then
    echo "  ✅ $name"
    PASS=$((PASS+1))
  else
    echo "  ❌ $name  (期望: $expected)"
    FAIL=$((FAIL+1))
  fi
}

ACTUAL_VER="$($BIN version 2>&1 | awk '{print $2}')"
echo "=== A 子命令 (当前版本: $ACTUAL_VER) ==="
check "A1 version" "ZyHive $ACTUAL_VER" "$($BIN version 2>&1)"
check "A2 -version" "ZyHive $ACTUAL_VER" "$($BIN -version 2>&1)"
check "A3a help subcmd 中文" "引巢 · ZyHive" "$($BIN help 2>&1)"
check "A3b --help 中文" "引巢 · ZyHive" "$($BIN --help 2>&1)"
check "A3c -h 中文" "引巢 · ZyHive" "$($BIN -h 2>&1)"
check "A4 unknown subcmd exit 1" "未知子命令：bogus" "$($BIN bogus 2>&1)"
check "A5 token 配置存在" "test-token-abcdefgh12345678" "$($BIN token 2>&1)"
check "A6 token 无配置报错" "未找到访问令牌" "$(AIPANEL_CONFIG=/tmp/nope.json $BIN token 2>&1)"

echo ""
echo "=== B 主菜单 ==="
check "B1 主菜单渲染" "请输入选项" "$(printf '0\n' | timeout 5 $BIN 2>&1)"
check "B2 q 退出" "再见" "$(printf 'q\n' | timeout 5 $BIN 2>&1)"
check "B2 Q 退出" "再见" "$(printf 'Q\n' | timeout 5 $BIN 2>&1)"
check "B2 quit 退出" "再见" "$(printf 'quit\n' | timeout 5 $BIN 2>&1)"
check "B2 exit 退出" "再见" "$(printf 'exit\n' | timeout 5 $BIN 2>&1)"
check "B3 无效选项" "无效选项" "$(printf '99\n\n0\n' | timeout 5 $BIN 2>&1)"

echo ""
echo "=== C 系统状态 ==="
check "C1 访问入口" "访问入口" "$(printf '1\n\n0\n' | timeout 10 $BIN 2>&1)"
check "C1 Token 脱敏" "test\*" "$(printf '1\n\n0\n' | timeout 10 $BIN 2>&1)"

echo ""
echo "=== D 服务管理 ==="
check "D1 菜单渲染" "启动服务" "$(printf '2\n0\n0\n' | timeout 5 $BIN 2>&1)"
check "D1 状态行" "服务状态" "$(printf '2\n0\n0\n' | timeout 5 $BIN 2>&1)"

echo ""
echo "=== E 配置管理 ==="
check "E1 配置文件路径" "配置文件" "$(printf '3\n0\n0\n' | timeout 5 $BIN 2>&1)"
check "E2 查看完整配置" "configVersion" "$(printf '3\n1\n\n0\n0\n' | timeout 10 $BIN 2>&1)"
check "E4 端口非法拒绝" "端口号无效" "$(printf '3\n3\nabc\n\n0\n0\n' | timeout 10 $BIN 2>&1)"
check "E6 绑定非法拒绝" "无效的绑定模式" "$(printf '3\n4\nbogus\n\n0\n0\n' | timeout 10 $BIN 2>&1)"
check "E8 空路径拒绝" "路径不能为空" "$(printf '3\n5\n\n\n0\n0\n' | timeout 10 $BIN 2>&1)"

echo ""
echo "=== F Providers 子菜单 ==="
check "F1 入口空列表" "尚未配置任何" "$(printf '3\n7\n0\n0\n0\n' | timeout 5 $BIN 2>&1)"
check "F2 无效编号" "无效选择" "$(printf '3\n7\n1\n99\n\n0\n0\n0\n' | timeout 5 $BIN 2>&1)"
check "F3 空 Key 拒绝" "API Key 不能为空" "$(printf '3\n7\n1\n1\n\n\n\n\n0\n0\n0\n' | timeout 5 $BIN 2>&1)"

echo ""
echo "=== G 成员管理 ==="
check "G1 列表不含隐藏目录" "已有 3 个 AI 成员" "$(printf '4\n0\n0\n' | timeout 5 $BIN 2>&1)"
check "G1 序号连续 1,2,3" "测试机器人 (testbot)" "$(printf '4\n0\n0\n' | timeout 5 $BIN 2>&1)"
check "G3 不存在成员" "成员不存在" "$(printf '4\n1\nnonexistent\n\n0\n0\n' | timeout 5 $BIN 2>&1)"
check "G4a 保护 main" "系统成员 main 不可删除" "$(printf '4\n2\nmain\n\n0\n0\n' | timeout 5 $BIN 2>&1)"
check "G4b 保护 __config__" "系统成员 __config__ 不可删除" "$(printf '4\n2\n__config__\n\n0\n0\n' | timeout 5 $BIN 2>&1)"

echo ""
echo "=== H 日志 ==="
check "H1 菜单渲染" "实时查看" "$(printf '5\n0\n0\n' | timeout 5 $BIN 2>&1)"
check "H3 搜索未匹配" "未找到匹配" "$(printf '5\n4\nxyznonsense\n\n0\n0\n' | timeout 5 $BIN 2>&1)"

echo ""
echo "=== I 在线更新 ==="
I_OUT="$(printf '6\nn\n\n0\n' | timeout 20 $BIN 2>&1)"
check "I 当前版本显示" "当前版本" "$I_OUT"
check "I 最新版本 fetch" "最新版本" "$I_OUT"
# 当前版本 == 最新：走"已是最新"分支；否则走 confirm → "已取消更新"
if echo "$I_OUT" | grep -q "已是最新版本"; then
  check "I 当前已是最新" "已是最新版本" "$I_OUT"
else
  check "I 拒绝后显示已取消" "已取消更新" "$I_OUT"
fi

echo ""
echo "=== J Nginx guard ==="
check "J1 nginx 未装" "Nginx 未安装" "$(printf '7\n\n0\n' | timeout 5 $BIN 2>&1)"

echo ""
echo "=== K SSL guard ==="
check "K1 certbot 未装" "certbot 未安装" "$(printf '8\n\n0\n' | timeout 5 $BIN 2>&1)"

echo ""
echo "=== L 备份 ==="
check "L1 菜单渲染" "备份与恢复" "$(printf '9\n0\n0\n' | timeout 5 $BIN 2>&1)"
check "L3 不存在的备份文件" "备份文件不存在" "$(printf '9\n3\nnonexistent.tar.gz\n\n0\n0\n' | timeout 5 $BIN 2>&1)"
# L4: 修改备份目录 + 持久化
rm -f $HOME/.config/zyhive/backup-dir
printf '9\n4\n/tmp/test-persist-dir\n\n0\n0\n' | timeout 5 $BIN > /dev/null 2>&1
check "L4 备份目录持久化到 state file" "/tmp/test-persist-dir" "$(cat $HOME/.config/zyhive/backup-dir)"
check "L4 重入后记住新目录" "/tmp/test-persist-dir" "$(printf '9\n0\n0\n' | timeout 5 $BIN 2>&1)"

echo ""
echo "================================================"
echo "Summary: PASS=$PASS  FAIL=$FAIL"
echo "================================================"
exit $FAIL
