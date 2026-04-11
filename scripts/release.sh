#!/usr/bin/env bash
# ZyHive 标准发布脚本
# 用法: ./scripts/release.sh 26.4.11v2
# 流程：构建UI → 同步嵌入 → 编译三平台 → 上传GitHub → 更新CF镜像latest
set -e

VERSION="$1"
if [ -z "$VERSION" ]; then
  echo "用法: $0 <版本号>  例如: $0 26.4.11v2"
  exit 1
fi

REPO="Zyling-ai/ZyHive"
GITHUB_TOKEN="${GITHUB_TOKEN:-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}"
DIST_DIR="/tmp/zyhive-release-${VERSION}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

echo "▶ 版本: $VERSION"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Step 1: 构建 Vue UI
echo "📦 [1/4] 构建 Vue UI..."
cd ui && npm install --silent && npx vite build --silent && cd ..
echo "   ✅ UI 构建完成"

# Step 2: ⚠️ 关键：同步 ui/dist → cmd/aipanel/ui_dist
echo "🔄 [2/4] 同步 UI 到嵌入目录..."
rm -rf cmd/aipanel/ui_dist
cp -r ui/dist cmd/aipanel/ui_dist
echo "   ✅ 同步完成 ($(ls cmd/aipanel/ui_dist/assets/ | wc -l | tr -d ' ') 个文件)"

# Step 3: 交叉编译三平台
echo "🔨 [3/4] 交叉编译..."
mkdir -p "$DIST_DIR"
GOOS=linux  GOARCH=amd64 go build -ldflags="-X main.Version=${VERSION}" -o "${DIST_DIR}/zyhive-linux-amd64"  ./cmd/aipanel/ &
GOOS=darwin GOARCH=arm64 go build -ldflags="-X main.Version=${VERSION}" -o "${DIST_DIR}/zyhive-darwin-arm64" ./cmd/aipanel/ &
GOOS=darwin GOARCH=amd64 go build -ldflags="-X main.Version=${VERSION}" -o "${DIST_DIR}/zyhive-darwin-amd64" ./cmd/aipanel/ &
wait
echo "   ✅ linux-amd64 darwin-arm64 darwin-amd64"

# Step 4: 发布 GitHub Release + CF Worker 自动代理
echo "🚀 [4/4] 发布 GitHub Release..."
RELEASE_ID=$(curl -s -X POST "https://api.github.com/repos/${REPO}/releases" \
  -H "Authorization: token ${GITHUB_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"tag_name\":\"${VERSION}\",\"name\":\"${VERSION}\",\"body\":\"## ${VERSION}\\n\\n发布日期：$(date '+%Y-%m-%d')\",\"draft\":false,\"prerelease\":false}" \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")

for NAME in zyhive-linux-amd64 zyhive-darwin-arm64 zyhive-darwin-amd64; do
  curl -s -X POST \
    "https://uploads.github.com/repos/${REPO}/releases/${RELEASE_ID}/assets?name=${NAME}" \
    -H "Authorization: token ${GITHUB_TOKEN}" \
    -H "Content-Type: application/octet-stream" \
    --data-binary "@${DIST_DIR}/${NAME}" > /dev/null
  echo "   ✅ 上传 ${NAME}"
done

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ 发布完成: ${VERSION}"
echo "   GitHub: https://github.com/${REPO}/releases/tag/${VERSION}"
echo "   CF镜像: https://install.zyling.ai/dl/${VERSION}/zyhive-linux-amd64"
echo ""
echo "⚠️  部署生产服务器请运行:"
echo "   sshpass -p '123ABCDabcd' scp ${DIST_DIR}/zyhive-linux-amd64 root@43.164.0.138:/tmp/zyhive-new"
echo "   sshpass -p '123ABCDabcd' ssh root@43.164.0.138 'systemctl stop zyhive && cp /tmp/zyhive-new /usr/local/bin/zyhive && systemctl start zyhive'"
