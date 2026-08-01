#!/usr/bin/env bash
# ZyHive 标准发布脚本
# 用法:
#   ./scripts/release.sh 26.8.1v1 --dry-run  # 只验证和构建，不发布
#   ./scripts/release.sh 26.8.1v1            # 创建 GitHub Release
#   ./scripts/release.sh --verify-supply-chain 26.8.1v1 ./release-assets
set -euo pipefail

VERSION="${1:-}"
MODE="${2:-}"
REPO="${ZYHIVE_REPO:-Zyling-ai/ZyHive}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST_DIR="${ZYHIVE_DIST_DIR:-/tmp/zyhive-release-${VERSION}}"
DRY_RUN=false

if [[ "$VERSION" == "--verify-supply-chain" ]]; then
  VERIFY_VERSION="${2:-}"
  VERIFY_DIR="${3:-}"
  if [[ -z "$VERIFY_VERSION" || -z "$VERIFY_DIR" || "$#" -ne 3 ]]; then
    echo "用法: $0 --verify-supply-chain <YY.M.DvN> <资产目录>" >&2
    exit 2
  fi
  if [[ -n "${ZYHIVE_CERTIFICATE_IDENTITY:-}" ]]; then
    exec bash "$REPO_ROOT/scripts/release-supply-chain.sh" verify "$VERIFY_DIR" "$ZYHIVE_CERTIFICATE_IDENTITY"
  fi
  MAIN_IDENTITY="https://github.com/${REPO}/.github/workflows/release-e2e.yml@refs/heads/main"
  if bash "$REPO_ROOT/scripts/release-supply-chain.sh" verify "$VERIFY_DIR" "$MAIN_IDENTITY"; then
    exit 0
  fi
  LEGACY_IDENTITY="https://github.com/${REPO}/.github/workflows/release-e2e.yml@refs/tags/${VERIFY_VERSION}"
  exec bash "$REPO_ROOT/scripts/release-supply-chain.sh" verify "$VERIFY_DIR" "$LEGACY_IDENTITY"
fi

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

if [[ "$(go env GOVERSION)" != "go1.25.10" ]]; then
  echo "❌ 可复现正式构建需要 Go 1.25.10，当前为 $(go env GOVERSION)" >&2
  exit 1
fi

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

echo "🧪 [1/9] 后端质量门槛..."
go vet ./...
go test ./... -count=1 -timeout=5m
go build ./...

echo "🖥  [2/9] 前端测试、类型检查与构建..."
(
  cd ui
  npm ci
  npm test
  npm run build
)

echo "🔄 [3/9] 同步并验证嵌入 UI..."
rm -rf cmd/aipanel/ui_dist
cp -R ui/dist cmd/aipanel/ui_dist
if ! git diff --no-index --quiet -- ui/dist cmd/aipanel/ui_dist; then
  echo "❌ 嵌入 UI 与源码构建结果不一致" >&2
  exit 1
fi

echo "🔨 [4/9] 构建官方支持平台..."
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
    go build -buildvcs=false -trimpath -ldflags="-s -w -X main.Version=${VERSION}" \
    -o "${DIST_DIR}/${name}" ./cmd/aipanel/
  echo "   ✅ $name"
done

echo "🔐 [5/9] 生成 SHA-256 校验和..."
(
  cd "$DIST_DIR"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum zyhive-* install.sh > SHA256SUMS
  else
    shasum -a 256 zyhive-* install.sh > SHA256SUMS
  fi
)
bash "$REPO_ROOT/scripts/release-supply-chain.sh" verify-checksums "$DIST_DIR"

echo "🧭 [6/9] 隔离环境验证全新安装、基础功能与旧版更新..."
scripts/test/release-e2e/run.sh local "$VERSION" "$DIST_DIR"

if [[ "$DRY_RUN" == true ]]; then
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "✅ 干运行完成，发布前全流程测试通过，未创建 Release"
  echo "   产物目录: $DIST_DIR"
  exit 0
fi

echo "📝 [7/9] 创建不可见的 GitHub Draft Release..."
release_notes=$(cat <<EOF
## ${VERSION}

发布日期：$(date '+%Y-%m-%d')

安装和升级默认使用随 Release 发布的 \`SHA256SUMS\` 校验二进制完整性；发布后供应链工作流会追加 SPDX SBOM、Sigstore keyless 签名和 GitHub 构建来源证明。
需要验证发布者身份时，请使用安装器的 \`--verify-signature\` 严格模式或 \`release.sh --verify-supply-chain\`。
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
  --notes "$release_notes" \
  --draft
candidate_sha="$(git rev-parse HEAD)"
release_id="$(
  gh release view "$VERSION" --repo "$REPO" --json databaseId --jq '.databaseId'
)"

echo "☁️  [8/9] 触发四平台候选验证、供应链签名与唯一发布门..."
previous_run_id="$(
  gh run list \
    --repo "$REPO" \
    --workflow release-e2e.yml \
    --event workflow_dispatch \
    --limit 20 \
    --json databaseId,displayTitle \
    --jq ".[] | select(.displayTitle == \"Release candidate ${VERSION}\") | .databaseId" |
    sort -nr |
    awk 'NR == 1 { print; exit }'
)"
gh workflow run release-e2e.yml \
  --repo "$REPO" \
  --ref main \
  -f "version=$VERSION" \
  -f "candidate_sha=$candidate_sha" \
  -f "release_id=$release_id" \
  -f "previous_version=$PREVIOUS_VERSION"

run_id=""
for _ in $(seq 1 30); do
  run_id="$(
    gh run list \
      --repo "$REPO" \
      --workflow release-e2e.yml \
      --event workflow_dispatch \
      --limit 20 \
      --json databaseId,displayTitle \
      --jq ".[] | select(.displayTitle == \"Release candidate ${VERSION}\") | .databaseId" |
      sort -nr |
      awk 'NR == 1 { print; exit }'
  )"
  if [[ -n "$run_id" && "$run_id" != "$previous_run_id" ]]; then
    break
  fi
  sleep 2
done
if [[ -z "$run_id" || "$run_id" == "$previous_run_id" ]]; then
  echo "❌ 未找到候选验证工作流；Draft Release 保持不可见" >&2
  exit 1
fi
if ! gh run watch "$run_id" --repo "$REPO" --exit-status; then
  echo "❌ 候选验证失败；Draft Release 保持不可见，不会进入 latest" >&2
  exit 1
fi

echo "🔎 [9/9] 确认 Release 只在全部门禁通过后公开..."
release_draft="$(
  gh api "repos/${REPO}/releases/tags/${VERSION}" --jq '.draft'
)"
if [[ "$release_draft" != "false" ]]; then
  echo "❌ 工作流已结束但 Release 仍是草稿，拒绝报告发布成功" >&2
  exit 1
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ 发布完成: ${VERSION}"
echo "   GitHub: https://github.com/${REPO}/releases/tag/${VERSION}"
echo "   国内镜像: https://install.zyling.ai/dl/${VERSION}/zyhive-linux-amd64"
