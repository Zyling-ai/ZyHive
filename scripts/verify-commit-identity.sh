#!/usr/bin/env bash
set -euo pipefail

EXPECTED_NAME="Zyling-ai"
EXPECTED_EMAIL="github@zyling.ai"
failed=false
revision="HEAD"

# pull_request 工作流默认检出 GitHub 生成的临时合并提交，其 Author/Committer
# 必然不是仓库贡献者。PR 中只验证 base..head 的真实提交；main push 仍验证
# HEAD 全部可达历史，防止历史身份或共同作者重新进入默认分支。
if [[ "${GITHUB_EVENT_NAME:-}" == "pull_request" &&
      -n "${GITHUB_EVENT_PATH:-}" &&
      -f "${GITHUB_EVENT_PATH}" ]]; then
  read -r base_sha head_sha < <(
    python3 - "${GITHUB_EVENT_PATH}" <<'PY'
import json
import sys

event = json.load(open(sys.argv[1], encoding="utf-8"))
pull_request = event.get("pull_request", {})
print(
    pull_request.get("base", {}).get("sha", ""),
    pull_request.get("head", {}).get("sha", ""),
)
PY
  )
  if [[ -z "$base_sha" || -z "$head_sha" ]]; then
    echo "❌ 无法从 PR 事件读取 base/head 提交" >&2
    exit 1
  fi
  revision="${base_sha}..${head_sha}"
fi

while IFS=$'\t' read -r sha author_name author_email committer_name committer_email; do
  if [[ "$author_name" != "$EXPECTED_NAME" || "$author_email" != "$EXPECTED_EMAIL" ||
        "$committer_name" != "$EXPECTED_NAME" || "$committer_email" != "$EXPECTED_EMAIL" ]]; then
    echo "❌ $sha 提交身份不符合要求" >&2
    echo "   author:    $author_name <$author_email>" >&2
    echo "   committer: $committer_name <$committer_email>" >&2
    failed=true
  fi
done < <(git log --format=$'%H\t%an\t%ae\t%cn\t%ce' "$revision")

if grep -Eiq '^[[:space:]]*co-authored-by:' <<<"$(git log --format='%B' "$revision")"; then
  echo "❌ 提交历史包含 Co-authored-by，共同作者会污染 GitHub Contributors" >&2
  failed=true
fi

if [[ "$failed" == true ]]; then
  echo "提交作者必须始终为 $EXPECTED_NAME <$EXPECTED_EMAIL>" >&2
  exit 1
fi

echo "✅ 所有可达提交均仅使用 $EXPECTED_NAME <$EXPECTED_EMAIL>"
