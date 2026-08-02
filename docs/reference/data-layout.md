# 数据布局、权限、事实源与缓存

ZyHive 当前是单机文件存储。目录位置由配置和进程工作目录共同决定；不要假定所有数据都位于配置文件旁。

## 根目录

- 主配置：`--config`/`AIPANEL_CONFIG` 指定的 JSON。
- 成员根：`agents.dir`，启动时转为绝对路径。
- 项目根：进程当前工作目录下 `projects/`。
- Cron/Goals 根：进程当前工作目录下 `cron/`。
- 全局 Usage：`{agents.dir}/.usage/`。

生产服务应固定 WorkingDirectory，否则相对的 `projects/`、`cron/` 以及相对 `agents.dir` 可能指向不同位置。

## 主配置

```text
zyhive.json
zyhive.json.lock
```

- 事实源：配置 JSON。
- 内存：启动后 `*config.Config` 是运行快照。
- 写入：`config.Transaction` 在候选快照上修改，成功原子替换后再发布内存。
- 权限：新保存固定 `0600`，父目录由持久化层创建为 `0700`。
- SecretRef：磁盘保留引用，内存为解析后的明文；未修改凭据再次保存时恢复原引用。

历史文件不会仅因升级自动 chmod；加载已有配置前应由运维确认权限。

## 成员目录

```text
{agents.dir}/
  {agentId}/
    config.json
    workspace/
      IDENTITY.md
      SOUL.md
      AGENTS.md
      WISHLIST.md
      memory/
        INDEX.md
        core/
        projects/
        daily/
        topics/
      network/
        INDEX.md
        INDEX.json
        RELATIONS.md
        contacts/*.md
        chats/*.md
        avatars/*
      skills/*
      .chatlogs/*
      .tool-audit/*
    sessions/
      sessions.json
      *.jsonl
      subagent/
    channels-pending/
    ...
  .subagent-tasks/
  .usage/YYYY-MM.jsonl
  approvals/
  aiteam/
```

启动加载成员时，Manager 会把成员根及已管理成员树收紧为目录 `0700`、普通文件 `0600`，保留 owner execute 位的文件变为 `0700`，并跳过符号链接。

### 成员配置

- 事实源：`{agentId}/config.json`。
- 内存：Manager 的 Agent 对象。
- 权限：`0600`。
- 修改入口：成员 API/Manager；不要热编辑磁盘后期待内存自动刷新。

### 工作区文档

- `IDENTITY.md`、`SOUL.md`、memory/network 文档是用户可编辑事实。
- 部分工作区 API/工具以 `0755/0644` 创建目录/普通文档；但下次成员树安全加载会收紧为 `0700/0600`。
- 文件 API 和工具必须通过受限路径解析，拒绝越界和危险符号链接。

### 会话

- `*.jsonl`：消息、compaction 等 append-only 事实源。
- `sessions.json`：会话列表、标题、计数、token 估算等派生索引。
- 目录：`0700`；会话和索引写入为 `0600`。
- JSONL 先追加，随后 best-effort 更新索引；对账时 JSONL 是事实源。
- compaction 不删除旧行，而是在读取 LLM 历史时以最后压缩摘要和之后消息构造有效上下文。
- Broadcaster 的事件缓冲和 Worker 状态只在内存中，用于断线重连，不是持久历史。

### 通讯录

- `contacts/*.md`、`chats/*.md`：frontmatter + Markdown body，档案事实源。
- `INDEX.json`：机器可读摘要快照。
- `INDEX.md`：注入 prompt 的轻量派生索引。
- 保存/删除档案后重建两个索引；也可调用 `/api/agents/:id/network/refresh`。
- 头像文件是外部平台图片的本地缓存；`avatarPath` 只指向缓存，不是远端身份事实。
- 联系人 alias 合并后，新消息路由到 primary 档案；不要根据文件名数量推断真实人数。

### Memory

- `memory/core|projects|daily|topics` 中 Markdown 是事实源。
- `memory/INDEX.md` 是 prompt 使用的轻量索引。
- embedding/搜索索引属于可重建缓存；无 embedding 时可退化到 BM25。
- Consolidator 会把 daily 信息提炼到长期层，属于显式数据变更，不只是缓存刷新。

### 日志与审计

- 会话 JSONL：面向对话恢复。
- `.chatlogs/`：渠道消息日志和其索引。
- conversation log：管理员可见的跨渠道审计视图。
- `.tool-audit/`：工具调用 JSONL，超大结果可拆到 blobs。
- `approvals/`：审批审计。
- 系统日志优先来自 `/tmp/aipanel.log`，否则 Linux journal 或 macOS unified log；这不是业务数据事实源。

## Usage 与预算

```text
{agents.dir}/.usage/YYYY-MM.jsonl
```

- 每次 LLM 调用一行，按 UTC 月分片。
- Usage JSONL 是计费汇总事实源，当前创建实现为目录 `0755`、文件 `0644`。
- 查询时扫描 JSONL 并聚合；API 返回的 summary/timeline 是计算结果，不应回写。
- `pkg/budget` 的普通日预算累计主要是进程内状态；Usage 先持久化，再通知预算 charger。
- 实验 aiteam budget guard 有独立持久状态，不能与普通 budget 等同。

若部署环境要求同机多用户隔离，应额外通过父目录权限/服务账号限制 `.usage`，因为其当前 mode 比成员会话宽。

## Cron 与 Goals

```text
cron/
  jobs.json
  runs/
  claims/
  goals.json
  goals-checks/
```

- `jobs.json`：Cron 定义事实源，`0600` 原子写。
- `runs/`：运行记录。
- `claims/`：运行所有权/恢复数据。
- `goals.json`：Goal 事实源，当前实现使用普通 JSON 文件。
- Cron 内存调度表是 `jobs.json` 的运行投影；加载时规范化任务，非法任务会禁用并写回原因。
- readiness 的 scheduler heartbeat 是内存健康信号，不是任务事实。

## Projects

```text
projects/
  {projectId}/
    meta.json
    README.md
    <用户文件>
```

- `meta.json`：项目元数据和 editors 事实源，`0600` 原子写。
- 项目文件：用户内容事实源。
- 项目根/已加载树会收紧为 `0700/0600`，保留 owner execute。
- `editors=[]` 表示所有成员可写；`["__none__"]` 表示全部只读。
- Manager 内存映射是启动快照；修改应走 API。

## 备份

版本化备份覆盖配置和工作目录中的受管数据，包含 manifest 和摘要。恢复前会检查路径、摘要和完整性，再分阶段替换各数据根；失败时会尽力回滚已替换内容，但这不是跨目录、抗断电的原子事务。推荐使用：

```bash
zyhive backup create --config /path/config.json --workdir /service/workdir --output backup.tar.gz
zyhive backup inspect --input backup.tar.gz
zyhive backup restore --config /path/config.json --workdir /service/workdir \
  --input backup.tar.gz --yes
```

若 `agents.dir` 位于 workdir 外，确认备份命令解析出的路径覆盖它；以 `backup inspect` 的 manifest 为准。

## 权限基线

- 凭据、成员配置、会话、Cron、审计、备份目录：目标 `0700/0600`。
- 可执行文件：owner execute，通常 `0700` 或安装产物 `0755`。
- 普通工作区文档可能初始 `0644`，但成员树加载会收紧。
- 不依赖 umask 代替显式 mode。
- 不手工修改 `.lock`、派生索引或 Worker 缓冲。
