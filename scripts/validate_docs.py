#!/usr/bin/env python3
"""Repository-local documentation quality gate.

The checker intentionally uses only Python's standard library so it can run in
developer environments and GitHub Actions without installing another toolchain.
"""

from __future__ import annotations

import re
import struct
import sys
import xml.etree.ElementTree as ET
from pathlib import Path
from urllib.parse import unquote, urlsplit


ROOT = Path(__file__).resolve().parents[1]
DOCS = ROOT / "docs"
CURRENT_VERSION = "26.8.2v1"

REQUIRED_FILES = [
    ROOT / "README.md",
    DOCS / "README.md",
    DOCS / "reference" / "feature-status.md",
    DOCS / "getting-started" / "installation.md",
    DOCS / "user-guide" / "README.md",
    DOCS / "admin" / "README.md",
    DOCS / "architecture" / "README.md",
    DOCS / "developer" / "README.md",
    DOCS / "governance" / "coverage-matrix.md",
]
REQUIRED_DIAGRAMS = {
    "system-overview.svg",
    "startup-dependencies.svg",
    "chat-sse-flow.svg",
    "runner-loop.svg",
    "tool-governance.svg",
    "session-compaction.svg",
    "memory-system.svg",
    "cron-claims.svg",
    "subagent-flow.svg",
    "channel-flow.svg",
    "data-layout.svg",
    "deployment-topology.svg",
    "trust-boundaries.svg",
    "backup-restore.svg",
    "update-rollback.svg",
    "release-state-machine.svg",
}
STATUS_LABELS = {"Stable", "Labs", "Planned", "Accepted Risk"}

LINK_RE = re.compile(r"!?\[[^\]]*]\(([^)\s]+)(?:\s+[\"'][^\"']*[\"'])?\)")
HEADING_RE = re.compile(r"^(#{1,6})\s+(.+?)\s*$", re.MULTILINE)
VERSION_RE = re.compile(r"\b\d{2}\.\d{1,2}\.\d{1,2}v\d+\b")
SECRET_PATTERNS = [
    re.compile(r"\b(?:sk|ghp|github_pat|glpat)-?[A-Za-z0-9_]{20,}\b"),
    re.compile(r"-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----"),
    re.compile(r"(?i)\b(?:password|api[_ -]?key|access[_ -]?token)\s*[:=]\s*[\"']?(?!<|\$\{|example|replace|your-|留空|占位)[A-Za-z0-9_./+=-]{16,}"),
]


def active_markdown_files() -> list[Path]:
    files = [ROOT / "README.md"]
    files.extend(path for path in DOCS.rglob("*.md") if "archive" not in path.parts)
    return sorted(files)


def resolve_link(source: Path, value: str) -> Path | None:
    parsed = urlsplit(value)
    if parsed.scheme or value.startswith(("mailto:", "#")):
        return None
    target = unquote(parsed.path)
    if not target:
        return None
    if target.startswith("/"):
        # Documentation must be portable; an absolute filesystem path is always stale.
        return Path(target)
    return (source.parent / target).resolve()


def check_required(errors: list[str]) -> None:
    for path in REQUIRED_FILES:
        if not path.is_file():
            errors.append(f"缺少必需文档: {path.relative_to(ROOT)}")
    diagrams = DOCS / "assets" / "diagrams"
    actual = {path.name for path in diagrams.glob("*.svg")}
    for name in sorted(REQUIRED_DIAGRAMS - actual):
        errors.append(f"缺少必需图表: docs/assets/diagrams/{name}")


def check_markdown(errors: list[str]) -> None:
    for path in active_markdown_files():
        if not path.exists():
            continue
        text = path.read_text(encoding="utf-8")
        rel = path.relative_to(ROOT)

        seen: dict[tuple[int, str], int] = {}
        fenced = False
        for line_number, line in enumerate(text.splitlines(), 1):
            if line.lstrip().startswith("```"):
                fenced = not fenced
                continue
            if fenced:
                continue
            match = re.match(r"^(#{1,6})\s+(.+?)\s*$", line)
            if not match:
                continue
            key = (len(match.group(1)), re.sub(r"\s+\{#.*}$", "", match.group(2)).strip().lower())
            if key in seen:
                errors.append(f"{rel}:{line_number}: 重复标题（首次在 {seen[key]} 行）: {match.group(2)}")
            seen[key] = line_number

        for match in LINK_RE.finditer(text):
            raw = match.group(1).strip("<>")
            target = resolve_link(path, raw)
            if target is None:
                continue
            if target.is_absolute() and not str(target).startswith(str(ROOT)):
                errors.append(f"{rel}: 不可移植的绝对链接: {raw}")
            elif not target.exists():
                errors.append(f"{rel}: 无效本地链接: {raw}")

        strict_version = (
            path in {ROOT / "README.md", DOCS / "README.md"}
            or bool({"getting-started", "user-guide", "admin", "architecture", "reference"} & set(rel.parts))
        )
        if strict_version:
            versions = set(VERSION_RE.findall(text))
            stale = sorted(version for version in versions if version not in {CURRENT_VERSION, "00.0.0v1"})
            if stale:
                errors.append(f"{rel}: 活跃文档包含非当前稳定版号: {', '.join(stale)}")

    status_path = DOCS / "reference" / "feature-status.md"
    if status_path.exists():
        text = status_path.read_text(encoding="utf-8")
        missing = STATUS_LABELS - {label for label in STATUS_LABELS if label in text}
        if missing:
            errors.append(f"docs/reference/feature-status.md 缺少状态标签: {', '.join(sorted(missing))}")

    # Historical material is exempt from current-version and heading rules, but
    # never exempt from credential scanning.
    secret_files = [ROOT / "README.md", *DOCS.rglob("*.md")]
    for path in secret_files:
        text = path.read_text(encoding="utf-8")
        for pattern in SECRET_PATTERNS:
            if pattern.search(text):
                errors.append(f"{path.relative_to(ROOT)}: 命中疑似秘密模式: {pattern.pattern}")


def check_assets(errors: list[str]) -> None:
    for path in (DOCS / "assets" / "diagrams").glob("*.svg"):
        source = path.with_suffix(".mmd")
        if not source.is_file():
            errors.append(f"{path.relative_to(ROOT)}: 缺少同名 Mermaid 源")
        try:
            root = ET.parse(path).getroot()
            if not root.tag.endswith("svg"):
                raise ValueError("root is not svg")
            title = root.find("{http://www.w3.org/2000/svg}title")
            desc = root.find("{http://www.w3.org/2000/svg}desc")
            if title is None or desc is None:
                errors.append(f"{path.relative_to(ROOT)}: SVG 缺少 title/desc 无障碍说明")
        except (ET.ParseError, ValueError) as exc:
            errors.append(f"{path.relative_to(ROOT)}: 无效 SVG: {exc}")

    for path in (DOCS / "assets" / "screenshots").glob("*.png"):
        data = path.read_bytes()
        if len(data) < 24 or data[:8] != b"\x89PNG\r\n\x1a\n":
            errors.append(f"{path.relative_to(ROOT)}: 无效 PNG")
            continue
        width, height = struct.unpack(">II", data[16:24])
        if width < 800 or height < 600:
            errors.append(f"{path.relative_to(ROOT)}: 截图尺寸过小 ({width}x{height})")


def main() -> int:
    errors: list[str] = []
    check_required(errors)
    check_markdown(errors)
    check_assets(errors)
    if errors:
        print("documentation validation failed:", file=sys.stderr)
        for error in errors:
            print(f"- {error}", file=sys.stderr)
        return 1
    count = len(active_markdown_files())
    print(f"documentation validation passed: {count} markdown files, {len(REQUIRED_DIAGRAMS)} required diagrams")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
