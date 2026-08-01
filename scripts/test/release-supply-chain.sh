#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
HELPER="$ROOT/scripts/release-supply-chain.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() {
  echo "❌ $*" >&2
  exit 1
}

expect_failure() {
  if "$@" >/dev/null 2>&1; then
    fail "命令本应失败: $*"
  fi
}

mkdir -p "$TMP/bin" "$TMP/release"
assets=(
  zyhive-linux-amd64
  zyhive-linux-arm64
  zyhive-darwin-amd64
  zyhive-darwin-arm64
  install.sh
)
for asset in "${assets[@]}"; do
  printf 'fixture:%s\n' "$asset" > "$TMP/release/$asset"
done
(
  cd "$TMP/release"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "${assets[@]}" > SHA256SUMS
  else
    shasum -a 256 "${assets[@]}" > SHA256SUMS
  fi
)

cat > "$TMP/bin/syft" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
output=""
while [[ $# -gt 0 ]]; do
  if [[ "$1" == "-o" ]]; then
    output="${2#spdx-json=}"
    shift 2
  else
    shift
  fi
done
[[ -n "$output" ]]
printf '{"spdxVersion":"SPDX-2.3","documentNamespace":"https://example.invalid/test"}\n' > "$output"
EOF

cat > "$TMP/bin/cosign" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
command_name="$1"
shift
bundle=""
identity=""
subject=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --bundle) bundle="$2"; shift 2 ;;
    --certificate-identity) identity="$2"; shift 2 ;;
    --certificate-oidc-issuer) shift 2 ;;
    --yes) shift ;;
    *) subject="$1"; shift ;;
  esac
done
hash_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}
case "$command_name" in
  sign-blob)
    printf '%s %s\n' "$(hash_file "$subject")" "${MOCK_SIGN_IDENTITY:?}" > "$bundle"
    ;;
  verify-blob)
    read -r expected_hash expected_identity < "$bundle"
    [[ "$(hash_file "$subject")" == "$expected_hash" ]] || exit 1
    [[ "$identity" == "$expected_identity" ]] || exit 1
    ;;
  *)
    exit 2
    ;;
esac
EOF
chmod +x "$TMP/bin/syft" "$TMP/bin/cosign"
export PATH="$TMP/bin:$PATH"
export MOCK_SIGN_IDENTITY="https://github.com/Zyling-ai/ZyHive/.github/workflows/release-e2e.yml@refs/tags/00.0.0v1"

bash "$HELPER" verify-checksums "$TMP/release" >/dev/null

cp "$TMP/release/zyhive-linux-amd64" "$TMP/original"
printf 'tampered\n' >> "$TMP/release/zyhive-linux-amd64"
expect_failure bash "$HELPER" verify-checksums "$TMP/release"
mv "$TMP/original" "$TMP/release/zyhive-linux-amd64"

cp "$TMP/release/SHA256SUMS" "$TMP/sums"
printf '%s  unexpected\n' "$(printf x | shasum -a 256 | awk '{print $1}')" >> "$TMP/release/SHA256SUMS"
expect_failure bash "$HELPER" verify-checksums "$TMP/release"
mv "$TMP/sums" "$TMP/release/SHA256SUMS"

bash "$HELPER" generate-sboms "$TMP/release" >/dev/null
[[ "$(find "$TMP/release" -name '*.spdx.json' -type f | wc -l | tr -d ' ')" == "5" ]] \
  || fail "SBOM 数量不正确"

bash "$HELPER" sign "$TMP/release" >/dev/null
[[ "$(find "$TMP/release" -name '*.sigstore.json' -type f | wc -l | tr -d ' ')" == "11" ]] \
  || fail "Sigstore bundle 数量不正确"

bash "$HELPER" verify "$TMP/release" "$MOCK_SIGN_IDENTITY" >/dev/null
expect_failure bash "$HELPER" verify "$TMP/release" "https://example.invalid/wrong-identity"

printf '\n' >> "$TMP/release/install.sh.spdx.json"
expect_failure bash "$HELPER" verify "$TMP/release" "$MOCK_SIGN_IDENTITY"

grep -q -- '--verify-signature' "$ROOT/scripts/install.sh" \
  || fail "安装器缺少严格签名验证入口"
grep -q '未验证发布者签名' "$ROOT/scripts/install.sh" \
  || fail "安装器未声明默认摘要信任边界"
grep -q 'command -v cosign' "$ROOT/scripts/install.sh" \
  || fail "安装器严格模式未对 cosign fail-closed"

WORKFLOW="$ROOT/.github/workflows/release-e2e.yml"
grep -q 'id-token: write' "$WORKFLOW" || fail "发布工作流缺少 OIDC 权限"
grep -q 'attestations: write' "$WORKFLOW" || fail "发布工作流缺少证明写权限"
grep -q 'anchore/sbom-action/download-syft@v0.20.6' "$WORKFLOW" \
  || fail "Syft 安装动作版本未固定"
grep -q 'syft-version: v1.29.0' "$WORKFLOW" || fail "Syft 版本未固定"
grep -q 'cosign-release: v2.5.0' "$WORKFLOW" || fail "cosign 版本未固定"
grep -q 'actions/attest-build-provenance@v2' "$WORKFLOW" \
  || fail "发布工作流缺少构建来源证明"
grep -q 'gh release edit.*--draft' "$WORKFLOW" \
  || fail "发布失败时未关闭公开分发"
if grep -Eq 'COSIGN_(PASSWORD|PRIVATE_KEY)|SIGNING_PRIVATE_KEY' "$WORKFLOW"; then
  fail "发布工作流禁止仓库签名私钥"
fi

echo "✅ 软件供应链文件集、摘要、SBOM、签名身份和失败路径测试通过"
