#!/usr/bin/env bash
# 快速热部署到 hive.lilianbot.com 生产服务器
#
# 必须从仓库根目录运行:
#   ./scripts/deploy-hive.sh
#
# 流程: vite build → sync ui_dist → CGO=0 静态编译 → scp 上传 → systemctl restart
#
# 关键: 一定要 sync ui_dist, 否则二进制内嵌的是旧 UI. 本脚本会显式 sync.

set -e

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

VERSION="${1:-$(git describe --tags --abbrev=0 2>/dev/null || echo dev)}"
HOST="43.164.0.138"
USER="root"
PASSWORD="${HIVE_ROOT_PASS:-123ABCDabcd}"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "▶ 部署 ZyHive 到 hive.lilianbot.com"
echo "   版本: $VERSION"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

echo "🧱 [1/4] 构建 UI (vite)..."
cd ui
if [[ ! -d node_modules ]]; then npm install --silent; fi
npx vite build --silent
cd ..

echo "🔄 [2/4] 同步 ui/dist → cmd/aipanel/ui_dist (关键!)..."
rm -rf cmd/aipanel/ui_dist
cp -r ui/dist cmd/aipanel/ui_dist
echo "   ui_dist: $(ls cmd/aipanel/ui_dist/assets/ | wc -l | tr -d ' ') 个文件"

echo "🔨 [3/4] CGO_ENABLED=0 静态编译 linux-amd64..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags "-X main.Version=${VERSION}" \
  -o /tmp/zyhive-deploy-$$ \
  ./cmd/aipanel/

# 完整性检查: 看二进制是否含关键 UI 标识字符串
if ! strings /tmp/zyhive-deploy-$$ | grep -q "readonly-banner"; then
  echo "⚠️  warn: 二进制内嵌 UI 可能不是最新 (缺 readonly-banner)"
  echo "   请手动验证 ui/dist 是否已重新构建"
fi

echo "🚀 [4/4] 上传 + 热部署..."
if ! command -v python3 >/dev/null; then
  echo "❌ 需要 python3 + paramiko 做 SFTP 上传"; exit 1
fi

python3 - <<PY
import paramiko
tr = paramiko.Transport(("${HOST}", 22))
tr.connect(username="${USER}", password="${PASSWORD}")
sftp = paramiko.SFTPClient.from_transport(tr)
print(f"  ↑ uploading to ${HOST}:/tmp/zyhive-deploy ...")
sftp.put("/tmp/zyhive-deploy-$$", "/tmp/zyhive-deploy")
sftp.close(); tr.close()
PY

python3 - <<PY
import paramiko, time
c = paramiko.SSHClient()
c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect("${HOST}", username="${USER}", password="${PASSWORD}", allow_agent=False, look_for_keys=False)
cmd = '''set -e
TS=\$(date +%Y%m%d-%H%M%S)
cp /usr/local/bin/zyhive /usr/local/bin/zyhive.bak-deploy-\$TS
chmod +x /tmp/zyhive-deploy
mv /tmp/zyhive-deploy /usr/local/bin/zyhive
systemctl restart zyhive
sleep 3
systemctl is-active zyhive
echo ""
curl -s http://localhost:8080/api/version
echo ""
# 运行二进制是否真含最新 UI
strings /usr/local/bin/zyhive | grep -c "readonly-banner" | xargs -I{} echo "binary has readonly-banner marker x{}"
'''
stdin, stdout, stderr = c.exec_command(cmd)
print(stdout.read().decode())
err = stderr.read().decode()
if err.strip(): print("[stderr]", err)
c.close()
PY

rm -f /tmp/zyhive-deploy-$$

echo ""
echo "✅ 部署完成, 请访问 https://hive.lilianbot.com 验证"
