#!/usr/bin/env python3
"""Generate the version-controlled SVG diagrams used by the ZyHive docs."""

from __future__ import annotations

import html
import re
import textwrap
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
OUTPUT = ROOT / "docs" / "assets" / "diagrams"

DIAGRAMS = {
    "system-overview": (
        "ZyHive 系统总体架构",
        "单机、自托管：浏览器和渠道共享同一运行时与本地事实源",
        [
            ("入口", ["Vue 管理界面", "业务 CLI", "Telegram / 飞书", "公开 Web Chat"]),
            ("服务层", ["Gin REST / SSE", "Agent Manager + Pool", "Session Worker", "Cron / Subagent"]),
            ("执行层", ["Runner Agentic Loop", "LLM Adapters", "Tool Registry", "Policy + Approval"]),
            ("数据与外部", ["Config / Agents / Projects", "Session JSONL / Audit", "LLM Providers", "GitHub Release"]),
        ],
    ),
    "startup-dependencies": (
        "启动与依赖装配",
        "main.go 按顺序建立事实源、运行时和入口；关闭时反向释放资源",
        [
            ("1 配置", ["Load + migrate", "SecretRef resolve", "安全默认配置"]),
            ("2 管理器", ["Agent Manager", "Project Manager", "系统成员"]),
            ("3 运行时", ["Agent Pool", "Cron Engine", "WorkerPool", "Approval Broker"]),
            ("4 接入", ["Channel BotPool", "Gin Routes", "HTTP Server", "Reapers"]),
        ],
    ),
    "chat-sse-flow": (
        "管理端聊天与断线恢复",
        "HTTP 断开只移除订阅，后台 Worker 继续运行并通过 Broadcaster 重放事件",
        [
            ("浏览器", ["POST /chat", "订阅 SSE", "刷新后重连"]),
            ("API", ["创建或恢复 Session", "获取 Worker", "返回事件流"]),
            ("Worker", ["同会话串行", "Broadcaster 缓冲", "后台 Context"]),
            ("Runner", ["LLM + Tools", "追加 JSONL", "usage / done"]),
        ],
    ),
    "runner-loop": (
        "Runner Agentic Loop",
        "每轮在预算、上下文和治理边界内调用模型与工具，最多 30 次迭代",
        [
            ("准备", ["Budget 预检", "必要时 Compaction", "追加用户消息"]),
            ("上下文", ["系统提示", "记忆 / 通讯录", "技能 / 项目"]),
            ("模型循环", ["LLM stream", "解析 tool calls", "并行执行工具"]),
            ("提交", ["tool results", "Session 持久化", "Usage / Audit / done"]),
        ],
    ),
    "tool-governance": (
        "工具治理决策链",
        "全局策略与成员策略取交集；deny 优先，ask 必须经过人工审批",
        [
            ("注册", ["内置工具", "动态工具", "ACP / Skills"]),
            ("策略", ["Global Policy", "Agent Policy", "Profile / allow / deny"]),
            ("审批", ["ask 命中", "一次性 SSE ticket", "允许 / 拒绝 / 超时"]),
            ("执行", ["参数与路径校验", "执行工具", "结果与审计"]),
        ],
    ),
    "session-compaction": (
        "Session 与 Compaction 事务",
        "JSONL 是事实源；索引可重建；sidecar 和 generation 防止并发压缩覆盖",
        [
            ("读取快照", ["共享目录锁", "跨进程 lock", "记录 generation"]),
            ("生成摘要", ["上一代摘要", "待压缩消息", "LLM summary"]),
            ("准备提交", ["prepared sidecar", "source digest", "再次校验 generation"]),
            ("原子提交", ["写新代", "更新索引", "中断后恢复"]),
        ],
    ),
    "memory-system": (
        "知识、记忆与通讯录",
        "轻量索引进入系统提示，完整内容由工具按需读取；长期整理由 Cron 触发",
        [
            ("身份上下文", ["IDENTITY / SOUL", "Owner Profile", "AGENTS"]),
            ("记忆树", ["core", "projects", "daily", "topics + INDEX"]),
            ("关系网络", ["Contacts", "Chats", "RELATIONS", "渐进披露"]),
            ("检索与整理", ["Embedding / BM25", "Consolidator", "Conversation Index"]),
        ],
    ),
    "cron-claims": (
        "Cron Claim 与崩溃恢复",
        "对同一计划 occurrence 防自动重复；外部副作用后的严格 exactly-once 不作承诺",
        [
            ("计划触发", ["cron / every / at", "计算 occurrence", "检查重叠"]),
            ("原子 Claim", ["稳定 runKey", "跨进程锁", "claimed → running"]),
            ("隔离执行", ["独立 Session", "Runner", "输出 / delivery"]),
            ("终态", ["ok / error", "runs JSONL", "中断 → uncertain"]),
        ],
    ),
    "subagent-flow": (
        "后台任务与 Subagent",
        "父会话派遣独立任务，进度与结果通过事件流和通知回到主会话",
        [
            ("父 Agent", ["关系权限", "任务 brief", "共享上下文"]),
            ("任务管理器", ["持久任务记录", "独立 Session", "取消控制"]),
            ("执行成员", ["Runner", "进度 report", "产物"]),
            ("回传", ["DispatchPanel", "任务通知", "主会话续接"]),
        ],
    ),
    "skillopt-loop": (
        "SkillOpt 有界演进闭环",
        "预测与真实结果形成证据；提案经过审核和 Shadow 比较后才能替换技能版本",
        [
            ("证据", ["predict 预测", "oracle 真实结果", "hit / miss 台账"]),
            ("归因", ["样本阈值", "Critic 分析 miss", "标签与教训"]),
            ("提案", ["Evolver 有界改写", "pending proposal", "人工接受 / 拒绝"]),
            ("验证", ["Shadow 对照", "晋升或回滚", "版本历史"]),
        ],
    ),
    "channel-flow": (
        "消息渠道处理链",
        "Telegram、飞书与公开 Web 共用 Agent 运行时，但认证、工具和会话边界不同",
        [
            ("接入", ["Telegram Polling", "Feishu WebSocket", "Web Public"]),
            ("身份边界", ["allowlist / pending", "联系人 / 群建档", "公开密码 / 限流"]),
            ("运行", ["渠道 Session", "Pool + Runner", "公开聊天禁用工具"]),
            ("输出", ["流式回复", "chatlog / convlog", "媒体与通知"]),
        ],
    ),
    "data-layout": (
        "本地数据布局与事实源",
        "敏感目录按 0700/0600 管理；JSONL 多为事实源，索引与摘要可重建",
        [
            ("全局", ["zyhive.json", "projects/", "cron/", "backup/"]),
            ("成员", ["config.json", "workspace/", "sessions/", "convlogs/"]),
            ("工作区", ["memory/", "network/", "skills/", "conversations/"]),
            ("运行记录", [".usage/", ".subagent-tasks/", "tool-audit/", "approvals/"]),
        ],
    ),
    "deployment-topology": (
        "单机部署拓扑",
        "正式支持 Linux/macOS 的 amd64/arm64；systemd 或 launchd 托管单进程",
        [
            ("客户端", ["浏览器", "CLI", "Telegram / 飞书"]),
            ("边缘", ["可选 Nginx / TLS", "防火墙", "Bearer Token"]),
            ("ZyHive 主机", ["HTTP API + UI", "Runner / Cron", "Chromium 安全代理"]),
            ("依赖", ["本地数据盘", "LLM Provider", "GitHub / 安装镜像"]),
        ],
    ),
    "trust-boundaries": (
        "信任边界与防护",
        "单一管理员 Token 模型；公开入口、工具执行和外部网络分别治理",
        [
            ("公网边界", ["CORS / body limit", "公开限流", "一次性票据"]),
            ("管理边界", ["Bearer Token", "配置锁", "审批 Broker"]),
            ("执行边界", ["Tool Policy", "safefs", "默认 Shell 可用"]),
            ("出站边界", ["netguard", "安全代理", "Provider allow rules"]),
        ],
    ),
    "backup-restore": (
        "备份与恢复",
        "手工单机恢复能力：完整校验后分阶段替换；归档目前未加密",
        [
            ("创建", ["解析目标", "拒绝 symlink", "manifest + SHA-256"]),
            ("发布", ["tar.gz", "原子写", "0600"]),
            ("恢复准备", ["Inspect 限额", "全量校验", "staging"]),
            ("提交", ["停止服务", "逐根 rename", "失败 rollback", "重启验证"]),
        ],
    ),
    "update-rollback": (
        "在线更新与 Watchdog 回滚",
        "管理 API 更新具备版本健康确认；安装器和遗留 CLI 的保证级别不同",
        [
            ("下载", ["目标平台资产", "SHA256SUMS", "校验内嵌版本"]),
            ("准备", [".bak", "pending record", "随机 token"]),
            ("替换", ["同目录 stage", "原子 rename", "启动新进程"]),
            ("确认", ["health + version", "成功清理", "失败恢复 .bak"]),
        ],
    ),
    "release-state-machine": (
        "Draft-first 发布状态机",
        "候选在全部验证成功前不可见；唯一 promote 将验证过的 Draft 公开为 latest",
        [
            ("本地门禁", ["Go / race / UI", "可复现资产", "身份检查"]),
            ("私有 Draft", ["固定 tag / SHA", "候选资产", "不影响 latest"]),
            ("候选验证", ["重建逐字节匹配", "SBOM / 签名 / provenance", "四目标旅程"]),
            ("唯一公开", ["再次核对 Draft", "promote", "公开 latest"]),
        ],
    ),
}


def lines(text: str, width: int = 22) -> list[str]:
    return textwrap.wrap(text, width=width, break_long_words=False) or [text]


def text_block(x: int, y: int, values: list[str], css: str, line_height: int = 24) -> str:
    spans = []
    for index, value in enumerate(values):
        dy = 0 if index == 0 else line_height
        spans.append(f'<tspan x="{x}" dy="{dy}">{html.escape(value)}</tspan>')
    return f'<text x="{x}" y="{y}" class="{css}">' + "".join(spans) + "</text>"


def render(key: str, title: str, subtitle: str, columns: list[tuple[str, list[str]]]) -> str:
    width, height = 1280, 720
    margin, gap = 48, 24
    card_width = (width - margin * 2 - gap * (len(columns) - 1)) // len(columns)
    card_y, card_height = 190, 430
    chunks = [
        f'''<svg xmlns="http://www.w3.org/2000/svg" width="{width}" height="{height}" viewBox="0 0 {width} {height}" role="img" aria-labelledby="{key}-title {key}-desc">
  <title id="{key}-title">{html.escape(title)}</title>
  <desc id="{key}-desc">{html.escape(subtitle)}</desc>
  <defs>
    <marker id="arrow" markerWidth="10" markerHeight="10" refX="8" refY="3" orient="auto">
      <path d="M0,0 L0,6 L9,3 z" fill="#64748b"/>
    </marker>
  </defs>
  <style>
    .title {{ font: 700 34px -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; fill: #0f172a; }}
    .subtitle {{ font: 400 18px -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; fill: #475569; }}
    .card {{ fill: #f8fafc; stroke: #cbd5e1; stroke-width: 2; rx: 16; }}
    .cardTitle {{ font: 700 22px -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; fill: #0f172a; }}
    .node {{ fill: #ffffff; stroke: #94a3b8; stroke-width: 1.5; rx: 10; }}
    .nodeText {{ font: 500 17px -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; fill: #1e293b; text-anchor: middle; }}
    .step {{ font: 600 14px -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; fill: #2563eb; }}
    .arrow {{ stroke: #64748b; stroke-width: 2.5; fill: none; marker-end: url(#arrow); }}
  </style>
  <rect width="1280" height="720" fill="#ffffff"/>
  {text_block(48, 64, [title], "title")}
  {text_block(48, 106, lines(subtitle, 62), "subtitle", 26)}
'''
    ]
    for index, (heading, nodes) in enumerate(columns):
        x = margin + index * (card_width + gap)
        chunks.append(f'  <rect x="{x}" y="{card_y}" width="{card_width}" height="{card_height}" class="card"/>\n')
        chunks.append(text_block(x + 24, card_y + 42, [heading], "cardTitle") + "\n")
        node_y = card_y + 78
        available = card_height - 106
        node_h = min(68, (available - 14 * (len(nodes) - 1)) // len(nodes))
        for node_index, node in enumerate(nodes):
            y = node_y + node_index * (node_h + 14)
            chunks.append(f'  <rect x="{x + 20}" y="{y}" width="{card_width - 40}" height="{node_h}" class="node"/>\n')
            wrapped = lines(node, 18)
            start_y = y + node_h // 2 - ((len(wrapped) - 1) * 10) + 6
            chunks.append(text_block(x + card_width // 2, start_y, wrapped, "nodeText", 21) + "\n")
        chunks.append(text_block(x + card_width - 45, card_y + card_height - 18, [f"{index + 1:02d}"], "step") + "\n")
        if index < len(columns) - 1:
            x1 = x + card_width + 4
            x2 = x + card_width + gap - 6
            y = card_y + card_height // 2
            chunks.append(f'  <path d="M{x1},{y} L{x2},{y}" class="arrow"/>\n')
    chunks.append("</svg>\n")
    return "".join(chunks)


def mermaid_id(value: str) -> str:
    return re.sub(r"[^A-Za-z0-9_]", "_", value)


def render_mermaid(title: str, columns: list[tuple[str, list[str]]]) -> str:
    output = [f"---\ntitle: {title}\n---\nflowchart LR\n"]
    previous_tail: str | None = None
    for column_index, (heading, nodes) in enumerate(columns):
        output.append(f'  subgraph C{column_index}["{heading}"]\n')
        node_ids = []
        for node_index, node in enumerate(nodes):
            node_id = mermaid_id(f"C{column_index}_{node_index}")
            node_ids.append(node_id)
            output.append(f'    {node_id}["{node}"]\n')
        for left, right in zip(node_ids, node_ids[1:]):
            output.append(f"    {left} --> {right}\n")
        output.append("  end\n")
        if previous_tail is not None:
            output.append(f"  {previous_tail} --> {node_ids[0]}\n")
        previous_tail = node_ids[-1]
    return "".join(output)


def main() -> None:
    OUTPUT.mkdir(parents=True, exist_ok=True)
    for key, (title, subtitle, columns) in DIAGRAMS.items():
        (OUTPUT / f"{key}.svg").write_text(render(key, title, subtitle, columns), encoding="utf-8")
        (OUTPUT / f"{key}.mmd").write_text(render_mermaid(title, columns), encoding="utf-8")
    print(f"generated {len(DIAGRAMS)} diagrams in {OUTPUT}")


if __name__ == "__main__":
    main()
