#!/bin/bash
# 一键部署 ZyHive Install Worker 到 Cloudflare
# 用法：bash deploy.sh

set -e
echo "🚀 部署 ZyHive Install Worker → install.zyling.ai"

# 检查 Node.js
if ! command -v node &>/dev/null; then
  echo "❌ 需要 Node.js，请先安装：https://nodejs.org"
  exit 1
fi

# 安装/确认 wrangler
if ! command -v wrangler &>/dev/null && ! npx --yes wrangler --version &>/dev/null 2>&1; then
  echo "安装 wrangler..."
  npm install -g wrangler
fi
WRANGLER="npx wrangler"

# 登录（如果未登录）
echo ""
echo "→ 检查 Cloudflare 登录状态..."
if ! $WRANGLER whoami &>/dev/null 2>&1; then
  echo "  需要登录 Cloudflare，正在打开浏览器..."
  $WRANGLER login
fi

echo "→ 部署 Worker..."
$WRANGLER deploy

echo ""
echo "✅ Worker 已部署！"
echo ""
echo "接下来在 Cloudflare DNS 确认有一条记录："
echo "  CNAME  install  →  install.zyling.ai.cdn.cloudflare.net  (Proxied)"
echo ""
echo "测试："
echo "  curl https://install.zyling.ai/latest"
echo "  curl -sSL https://install.zyling.ai/zyhive.sh | bash"
