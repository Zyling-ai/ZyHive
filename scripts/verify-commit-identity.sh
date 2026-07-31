#!/usr/bin/env bash
set -euo pipefail

EXPECTED_NAME="Zyling-ai"
EXPECTED_EMAIL="github@zyling.ai"
failed=false

while IFS=$'\t' read -r sha author_name author_email committer_name committer_email; do
  if [[ "$author_name" != "$EXPECTED_NAME" || "$author_email" != "$EXPECTED_EMAIL" ||
        "$committer_name" != "$EXPECTED_NAME" || "$committer_email" != "$EXPECTED_EMAIL" ]]; then
    echo "❌ $sha 提交身份不符合要求" >&2
    echo "   author:    $author_name <$author_email>" >&2
    echo "   committer: $committer_name <$committer_email>" >&2
    failed=true
  fi
done < <(git log --format=$'%H\t%an\t%ae\t%cn\t%ce' HEAD)

if grep -Eiq '^[[:space:]]*co-authored-by:' <<<"$(git log --format='%B' HEAD)"; then
  echo "❌ 提交历史包含 Co-authored-by，共同作者会污染 GitHub Contributors" >&2
  failed=true
fi

if [[ "$failed" == true ]]; then
  echo "提交作者必须始终为 $EXPECTED_NAME <$EXPECTED_EMAIL>" >&2
  exit 1
fi

echo "✅ 所有可达提交均仅使用 $EXPECTED_NAME <$EXPECTED_EMAIL>"
