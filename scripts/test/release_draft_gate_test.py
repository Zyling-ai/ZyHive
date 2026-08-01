#!/usr/bin/env python3
"""Static regression checks for the draft-first release state machine."""

from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = (ROOT / ".github/workflows/release-e2e.yml").read_text(encoding="utf-8")
SCRIPT = (ROOT / "scripts/release.sh").read_text(encoding="utf-8")


def require(condition: bool, message: str) -> None:
    if not condition:
        raise AssertionError(message)


require("workflow_dispatch:" in WORKFLOW, "release gate must be explicitly dispatched")
require("types: [published]" not in WORKFLOW, "validation must not start after publication")
require("ref: ${{ inputs.candidate_sha }}" in WORKFLOW, "draft candidates must be checked out by immutable commit")
require("releases/${RELEASE_ID}" in WORKFLOW, "draft verification must use its database ID before a tag exists")
require("--jq '.draft'" in WORKFLOW, "draft state must be read without shell-escaped inline Python")
require("uses: actions/upload-artifact@v4" in WORKFLOW, "verified draft assets must be staged for read-only runners")
require("uses: actions/download-artifact@v4" in WORKFLOW, "native runners must consume the verified artifact")
require("candidate-install-update:" in WORKFLOW, "candidate install/update gate is missing")
require("needs: [supply-chain, candidate-install-update]" in WORKFLOW, "promotion must depend on every candidate gate")
require("run: gh release edit \"$VERSION\" --repo \"$REPOSITORY\" --draft=false --latest" in WORKFLOW,
        "promotion must be the only publication transition")
require(WORKFLOW.count("--draft=false") == 1, "workflow must contain exactly one publication transition")
for runner in ("ubuntu-latest", "ubuntu-24.04-arm", "macos-15-intel", "macos-14"):
    require(runner in WORKFLOW, f"missing native candidate runner: {runner}")

draft_at = SCRIPT.index("--draft")
dispatch_at = SCRIPT.index("gh workflow run release-e2e.yml")
require(draft_at < dispatch_at, "draft must exist before candidate validation starts")
require("gh release create \"$VERSION\"" in SCRIPT, "release script must create the candidate")
require("-f \"candidate_sha=$candidate_sha\"" in SCRIPT, "dispatch must bind the candidate commit")
require("-f \"release_id=$release_id\"" in SCRIPT, "dispatch must bind the private draft identity")
require("gh release edit \"$VERSION\" --repo \"$REPO\" --draft" not in SCRIPT,
        "local script must never publish or withdraw around failed validation")
require("gh run watch \"$run_id\"" in SCRIPT, "release script must wait for the promotion workflow")

print("draft-first release gate checks passed")
