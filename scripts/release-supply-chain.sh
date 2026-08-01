#!/usr/bin/env bash
# Release 资产、SBOM 与 Sigstore bundle 的可复用生成/验证入口。
set -euo pipefail

ASSETS=(
  zyhive-linux-amd64
  zyhive-linux-arm64
  zyhive-darwin-amd64
  zyhive-darwin-arm64
  install.sh
)

usage() {
  cat >&2 <<'EOF'
用法:
  release-supply-chain.sh verify-checksums <目录>
  release-supply-chain.sh generate-sboms <目录>
  release-supply-chain.sh sign <目录>
  release-supply-chain.sh verify <目录> <证书身份>
EOF
  exit 2
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "❌ 缺少命令: $1" >&2
    exit 1
  }
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

verify_checksums() {
  local dir="$1" sums="$1/SHA256SUMS" asset expected actual
  [[ -f "$sums" ]] || { echo "❌ 缺少 SHA256SUMS" >&2; return 1; }

  for asset in "${ASSETS[@]}"; do
    [[ -f "$dir/$asset" ]] || { echo "❌ 缺少发布资产: $asset" >&2; return 1; }
    expected="$(
      awk -v name="$asset" '
        $2 == name || $2 == "*" name { count++; value=$1 }
        END {
          if (count == 1 && value ~ /^[0-9a-fA-F]{64}$/) print tolower(value)
        }
      ' "$sums"
    )"
    [[ -n "$expected" ]] || {
      echo "❌ SHA256SUMS 中 $asset 缺失、重复或格式错误" >&2
      return 1
    }
    actual="$(sha256_file "$dir/$asset")"
    [[ "$actual" == "$expected" ]] || {
      echo "❌ $asset 的 SHA-256 不匹配" >&2
      return 1
    }
  done

  local entries
  entries="$(awk 'NF { count++ } END { print count+0 }' "$sums")"
  [[ "$entries" == "${#ASSETS[@]}" ]] || {
    echo "❌ SHA256SUMS 必须且只能包含 ${#ASSETS[@]} 个正式资产" >&2
    return 1
  }
  echo "✅ 发布资产 SHA-256 全部匹配"
}

generate_sboms() {
  local dir="$1" asset output
  require_command syft
  verify_checksums "$dir"
  for asset in "${ASSETS[@]}"; do
    output="$dir/${asset}.spdx.json"
    syft "$dir/$asset" -o "spdx-json=$output"
    python3 - "$output" "$asset" <<'PY'
import json
import sys

path, asset = sys.argv[1:]
with open(path, encoding="utf-8") as handle:
    document = json.load(handle)
if document.get("spdxVersion") != "SPDX-2.3":
    raise SystemExit(f"{asset}: SBOM 不是 SPDX 2.3")
if not document.get("documentNamespace"):
    raise SystemExit(f"{asset}: SBOM 缺少 documentNamespace")
PY
  done
  echo "✅ 已生成 ${#ASSETS[@]} 份 SPDX 2.3 SBOM"
}

sign_assets() {
  local dir="$1" file
  require_command cosign
  verify_checksums "$dir"
  for file in \
    "${ASSETS[@]}" \
    SHA256SUMS \
    "${ASSETS[@]/%/.spdx.json}"; do
    [[ -f "$dir/$file" ]] || { echo "❌ 缺少待签名文件: $file" >&2; return 1; }
    COSIGN_EXPERIMENTAL=1 cosign sign-blob --yes \
      --bundle "$dir/${file}.sigstore.json" \
      "$dir/$file"
  done
  echo "✅ 已为正式资产、校验和与 SBOM 生成 keyless Sigstore bundle"
}

verify_release() {
  local dir="$1" identity="$2" file
  require_command cosign
  verify_checksums "$dir"
  for file in \
    "${ASSETS[@]}" \
    SHA256SUMS \
    "${ASSETS[@]/%/.spdx.json}"; do
    [[ -f "$dir/$file" ]] || { echo "❌ 缺少已签名文件: $file" >&2; return 1; }
    [[ -f "$dir/${file}.sigstore.json" ]] || {
      echo "❌ 缺少 Sigstore bundle: ${file}.sigstore.json" >&2
      return 1
    }
    if ! COSIGN_EXPERIMENTAL=1 cosign verify-blob \
      --bundle "$dir/${file}.sigstore.json" \
      --certificate-identity "$identity" \
      --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
      "$dir/$file" >/dev/null; then
      echo "❌ $file 的 Sigstore bundle 或证书身份验证失败" >&2
      return 1
    fi
  done
  echo "✅ SHA-256 与全部 keyless 签名验证通过"
}

[[ $# -ge 2 ]] || usage
command_name="$1"
directory="$2"
[[ -d "$directory" ]] || { echo "❌ 目录不存在: $directory" >&2; exit 1; }

case "$command_name" in
  verify-checksums)
    [[ $# -eq 2 ]] || usage
    verify_checksums "$directory"
    ;;
  generate-sboms)
    [[ $# -eq 2 ]] || usage
    require_command python3
    generate_sboms "$directory"
    ;;
  sign)
    [[ $# -eq 2 ]] || usage
    sign_assets "$directory"
    ;;
  verify)
    [[ $# -eq 3 ]] || usage
    verify_release "$directory" "$3"
    ;;
  *)
    usage
    ;;
esac
