# ZyHive Agent CLI

`zyhive` 现在同时提供两类能力：

- 人类运维面板：`zyhive`、`zyhive start`、`zyhive stop`、`zyhive status` 等。
- Agent 系统操作面：`zyhive <资源> <动作>`，面向内部 AI 成员、外部 agent、CI 和脚本。

Agent CLI 是 REST API 的瘦客户端，不直连数据文件。它复用服务端鉴权、审计、Cron 引擎、Session Worker、Bot Pool 等运行时能力。

## 全局约定

```bash
zyhive <资源> <动作> [参数] [--json] [--host URL] [--token TOKEN]
```

连接与鉴权优先级：

1. CLI flags：`--host`、`--token`、`--config`
2. 环境变量：`ZYHIVE_HOST`、`ZYHIVE_TOKEN`、`AIPANEL_CONFIG`
3. 本机配置文件：`/etc/zyhive/zyhive.json`、`~/.config/zyhive/zyhive.json` 等

机器调用建议：

```bash
zyhive agent list --json
zyhive chat send main "总结今天的目标进展" --json
zyhive api GET /api/status --json
```

写入、删除、派遣等有副作用的操作需要 `--yes`：

```bash
zyhive cron add --agent main --name morning --expr "0 9 * * *" --message "整理昨日进展" --yes
zyhive relation edge-add --from main --to planner --type 平级协作 --yes
```

退出码稳定用于脚本分支：

- `0` 成功
- `1` 通用错误
- `2` 参数/用法错误
- `3` 鉴权失败
- `4` 资源不存在
- `5` 无法连接服务

## 资源命令

核心资源：

- `agent`：成员 list/get/create/update/delete/start/stop/message
- `chat`：发送对话、会话列表/读取/重命名/删除
- `cron`：定时任务 list/add/update/remove/run/runs/enable
- `memory`：记忆树 tree/read/write/daily/config/consolidate
- `task`：后台任务 list/spawn/get/kill/eligible
- `goal`：目标 CRUD、进度、里程碑、检查计划与记录

团队与知识：

- `network`：联系人与群档案 contacts/contact/contact-update/contact-merge/chats/chat/refresh
- `relation`：团队关系 get/set/graph/edge-add/edge-delete/clear
- `project`：共享项目 CRUD、权限、文件读写
- `file`：成员 workspace 文件读写删
- `session`：全局跨成员会话管理

配置与可观测：

- `model`、`provider`、`channel`、`tool`、`acp`：list/create/update/delete/test
- `skill`：list/install/delete
- `usage`：summary/timeline/records
- `system`：status/stats/health/ready
- `conversations`：全局或成员对话审计
- `approval`：pending/approve/deny

逃生舱：

```bash
zyhive api GET /api/agents --json
zyhive api POST /api/goals '{"id":"g1","title":"..."}' --json
echo '{"title":"新标题"}' | zyhive api PATCH /api/sessions/main/sid -
```

## 给内部 AI 成员的建议

内部成员可通过 `exec` 工具调用 `zyhive`。遇到系统级操作时优先：

1. 先运行 `zyhive --help` 或 `zyhive <资源> --help` 确认命令。
2. 读操作加 `--json`，便于解析。
3. 写操作明确加 `--yes`，并在执行前向用户说明会修改什么。
4. 未封装的 API 用 `zyhive api <METHOD> <path>`。
