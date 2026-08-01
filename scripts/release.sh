#!/usr/bin/env bash
# ZyHive 标准发布脚本
# 用法:
#   ./scripts/release.sh 26.8.1v1 --dry-run  # 只验证和构建，不发布
#   ./scripts/release.sh 26.8.1v1            # 创建 GitHub Release
set -euo pipefail

VERSION="${1:-}"
MODE="${2:-}"
REPO="${ZYHIVE_REPO:-Zyling-ai/ZyHive}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST_DIR="${ZYHIVE_DIST_DIR:-/tmp/zyhive-release-${VERSION}}"
DRY_RUN=false

if [[ -z "$VERSION" || ! "$VERSION" =~ ^[0-9]{2}\.[0-9]{1,2}\.[0-9]{1,2}v[0-9]+$ ]]; then
  echo "用法: $0 <YY.M.DvN> [--dry-run]  例如: $0 26.8.1v1 --dry-run" >&2
  exit 2
fi
if [[ -n "$MODE" && "$MODE" != "--dry-run" ]]; then
  echo "未知参数: $MODE" >&2
  exit 2
fi
[[ "$MODE" == "--dry-run" ]] && DRY_RUN=true

cd "$REPO_ROOT"

require_command() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "❌ 缺少命令: $1" >&2
    exit 1
  }
}

for cmd in git go node npm curl python3; do
  require_command "$cmd"
done

node -e '
  const [major, minor] = process.versions.node.split(".").map(Number)
  if (major < 22 || (major === 22 && minor < 13)) {
    console.error(`需要 Node >= 22.13.0，当前为 ${process.versions.node}`)
    process.exit(1)
  }
'

scripts/verify-commit-identity.sh

if [[ "$DRY_RUN" == false ]]; then
  require_command gh
  gh auth status >/dev/null
  if [[ -n "$(git status --porcelain)" ]]; then
    echo "❌ 正式发布要求工作区无未提交改动" >&2
    exit 1
  fi
  if [[ "$(git branch --show-current)" != "main" ]]; then
    echo "❌ 正式发布必须从 main 分支执行" >&2
    exit 1
  fi
  if gh release view "$VERSION" --repo "$REPO" >/dev/null 2>&1; then
    echo "❌ Release $VERSION 已存在" >&2
    exit 1
  fi
  PREVIOUS_VERSION="$(
    gh release list --repo "$REPO" --exclude-drafts --exclude-pre-releases \
      --limit 1 --json tagName --jq '.[0].tagName // ""'
  )"
else
  PREVIOUS_VERSION=""
fi

echo "▶ 版本: $VERSION"
echo "▶ 模式: $([[ "$DRY_RUN" == true ]] && echo "干运行" || echo "正式发布")"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

echo "🧪 [1/8] 后端质量门槛..."
go vet ./...
go test ./... -count=1 -timeout=5m
go build ./...

echo "🖥  [2/8] 前端测试、类型检查与构建..."
(
  cd ui
  npm ci
  npm test
  npm run build
)

echo "🔄 [3/8] 同步并验证嵌入 UI..."
rm -rf cmd/aipanel/ui_dist
cp -R ui/dist cmd/aipanel/ui_dist
if ! git diff --no-index --quiet -- ui/dist cmd/aipanel/ui_dist; then
  echo "❌ 嵌入 UI 与源码构建结果不一致" >&2
  exit 1
fi

echo "🔨 [4/8] 构建官方支持平台..."
rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"
cp scripts/install.sh "$DIST_DIR/install.sh"
platforms=(
  "linux amd64"
  "linux arm64"
  "darwin amd64"
  "darwin arm64"
)
for platform in "${platforms[@]}"; do
  read -r goos goarch <<<"$platform"
  name="zyhive-${goos}-${goarch}"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags="-s -w -X main.Version=${VERSION}" \
    -o "${DIST_DIR}/${name}" ./cmd/aipanel/
  echo "   ✅ $name"
done

echo "🔐 [5/8] 生成 SHA-256 校验和..."
(
  cd "$DIST_DIR"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum zyhive-* > SHA256SUMS
  else
    shasum -a 256 zyhive-* > SHA256SUMS
  fi
)

echo "🧭 [6/8] 隔离环境验证全新安装、基础功能与旧版更新..."
scripts/test/release-e2e/run.sh local "$VERSION" "$DIST_DIR"

if [[ "$DRY_RUN" == true ]]; then
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "✅ 干运行完成，发布前全流程测试通过，未创建 Release"
  echo "   产物目录: $DIST_DIR"
  exit 0
fi

echo "🚀 [7/8] 创建 GitHub Release..."
release_notes=$(cat <<EOF
## ${VERSION}

发布日期：$(date '+%Y-%m-%d')

安装和升级前会使用随 Release 发布的 \`SHA256SUMS\` 校验二进制完整性。
具体变更见仓库 \`CHANGELOG.md\`。
EOF
)

gh release create "$VERSION" \
  "$DIST_DIR"/zyhive-* \
  "$DIST_DIR/install.sh" \
  "$DIST_DIR/SHA256SUMS" \
  --repo "$REPO" \
  --target "$(git rev-parse HEAD)" \
  --title "$VERSION" \
  --notes "$release_notes"

echo "🌐 [8/8] 验证正式 Release 在线安装与真实版本更新..."
if ! scripts/test/release-e2e/run.sh online "$VERSION" "$PREVIOUS_VERSION"; then
  echo "❌ Release 在线全流程测试失败，正在转为草稿以停止公开分发..." >&2
  gh release edit "$VERSION" --repo "$REPO" --draft
  exit 1
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ 发布完成: ${VERSION}"
echo "   GitHub: https://github.com/${REPO}/releases/tag/${VERSION}"
echo "   国内镜像: https://install.zyling.ai/dl/${VERSION}/zyhive-linux-amd64"
