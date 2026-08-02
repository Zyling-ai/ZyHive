# 持久化与一致性

![本地数据布局与事实源](../assets/diagrams/data-layout.svg)

![备份恢复的分阶段提交](../assets/diagrams/backup-restore.svg)


## 1. 存储策略

ZyHive 采用本地可读文件：

- JSON：配置、索引、Job、Claim、结构化状态；
- JSONL：Session、Usage、ToolAudit、Conversation/Chat log、账本；
- Markdown：身份、灵魂、记忆、通讯录、关系和项目知识；
- 二进制/普通文件：工作区产物、头像、审计 blob、备份。

优点是易备份、易检查、无数据库依赖；代价是跨文件事务、复杂查询、迁移和崩溃恢复需每个 Store 自行实现。

## 2. `pkg/persist` 原语

### 2.1 路径锁

`LockFile(path)`：

1. 按 clean path 获取进程内 `sync.Mutex`；
2. 创建父目录；
3. 获取相邻 `path.lock` 的 OS 文件锁；
4. 返回 exactly-once unlock closure。

这只对使用同一规范化 path 且遵守该协议的写者有效。普通编辑器、脚本或绕过 `persist` 的包不会被协调。

### 2.2 原子替换

`AtomicWrite(path, data, mode)`：

1. 在目标目录创建临时文件；
2. chmod；
3. 完整写入并 `fsync` 临时文件；
4. close；
5. rename 到目标；
6. 再 chmod；
7. `fsync` 父目录。

同一文件系统内可避免读到半文件，并提高掉电后目录项耐久性。它不自动加锁；需要串行化时使用 `WriteFile` 或外层 `WithFileLock`。

Windows rename/lock 细节由平台文件实现决定；任何发布平台都需单独测试，而不能仅凭 POSIX 语义推断。

## 3. 配置事务

`config.Transaction(path, cfg, mutate)` 的意图：

1. 对内存 Config 的专属 RWMutex 加写锁；
2. 对配置路径加跨进程文件锁；
3. 深复制当前 `cfg` 为 candidate；
4. 在 candidate 上执行 mutate；
5. 校验、保留未变 SecretRef 并原子写盘；
6. 写盘成功后才把 candidate 发布回现有 `cfg` 对象。

性质：

- 写盘失败不污染当前内存配置；
- 同一 cfg 指针的并发更新串行；
- `Snapshot` 在读锁下返回深复制；
- SecretRef（如 `{"$env":"NAME"}`）在运行时解析，但无关更新不会把它展平成明文。

限制：

- 配置对象仍是共享可变结构；绕过 Transaction 直接改字段会破坏保证；
- 不同进程各有内存快照，文件锁不会自动让另一个进程热重载；
- “内存已发布”不代表 HTTP listener、auth middleware、Bot 或 throttle 已重新构造；
- 配置与 Agent/Channel 其他文件没有跨域事务。

## 4. Session 事务

Session Store 同时使用：

- 规范化目录共享 mutex；
- `.store-transaction` 跨进程锁；
- JSONL append；
- `sessions.json` 原子替换。

写消息不是单个原子文件事务：JSONL 成功后索引可能失败。系统把 JSONL 作为事实源，并通过 reconcile 恢复索引。设计者必须维持以下不变量：

- 绝不为了索引失败回删已追加消息；
- 重建索引不重复统计当前行；
- 压缩替换后 metadata 可从新 JSONL 得出；
- Session ID 先验证，不能影响锁文件或其他会话路径。

## 5. Compaction 提交

压缩跨三个状态：

1. 原 Session JSONL generation；
2. `.compaction.json` intent/prepared；
3. 新 JSONL + 新 sessions index。

Generation 比较防止摘要期间并发消息被覆盖。Prepared sidecar 让崩溃恢复可复用已生成摘要。JSONL 与 index 仍不是同一原子提交，因此恢复逻辑必须接受：

- 新 JSONL + 旧 index；
- prepared sidecar + 旧 JSONL；
- chatlog summary 更新失败。

不能重新引入只 Append CompactionEntry 的旧 `pkg/compaction` 语义，否则会绕过这些代际保证。

## 6. Cron Job 与 Claim

Cron 数据目录包含 Job、`runs/` 和 `claims/`。计划触发与手动 `RunNow` 不完全相同：

- 同一进程 `activeRuns[jobID]` 阻止重叠；
- **计划触发**按 `{jobID, occurrenceMs}` 调 `acquireClaim`；
- Claim 初态 `claimed`，执行前变 `running`，执行完成先记 `executed`，最终写 `ok/error/skipped/uncertain`；
- 创建使用原子“若不存在”语义及文件锁，使共享目录的多个 Engine 对同 occurrence 只有一个 claim 获得者；
- 未获得者写 skipped run，不执行 Agent。

### 6.1 崩溃恢复

`recoverClaimsLocked` 扫描 `claimed/running/executed`：

- 改成 `uncertain`；
- 写 RunRecord；
- 更新 Job last status；
- 禁用需要人工判断的任务，`at` 一次性任务不再重触发。

这是“at-most-once 倾向 + 显式不确定”，不是严格 exactly-once：

- 外部副作用可能已发生但 claim 尚未更新；
- 文件系统原子 claim 不能与外部 API 事务提交；
- 系统选择不自动重放，以避免重复发送/付款/写入；
- 人工需根据 ToolAudit、外部系统和 run record 判断。

手动 `RunNow` 没有按 schedule occurrence 的持久 claim，主要受当前进程重叠保护。不要把计划 claim 的保证外推到所有调用。

## 7. 审批与运行状态

审批 Broker、Worker queue、Broadcaster buffer、Runner tool loop 都是内存状态：

- 重启后 pending approval 消失；
- 正在执行的消息不恢复；
- 当前 SSE 增量不恢复；
- 工具是否已完成只能从审计和外部状态推断。

Session JSONL 持久化的是对话结果，不是完整事务日志。未来 Run/Step/Event/Checkpoint 应进入 SQLite 或等价事务存储，但当前仓库尚未实现。

## 8. 日志与派生数据

- Usage：按月 JSONL，写失败通常不阻塞回复，会造成成本统计缺口；
- ToolAudit：按日/大小管理，失败不回滚工具；
- convlog/chatlog：管理员和渠道视图，可能与 Session 在崩溃窗口不一致；
- Memory/Network INDEX：由 Markdown 源文件刷新，属于派生视图；
- Title/token estimate：索引元数据，可与 JSONL 协调。

读取方应明确哪个是 source of truth，不应在多个副本间做“最后写入覆盖”猜测。

## 9. 文件权限与 Secret

敏感新文件通常要求：

- 配置、claim、session/compaction sidecar、审批与秘密状态：0600；
- 私有目录：0700；
- 可执行发布二进制：按安装需求设置。

旧文件不一定自动 chmod；升级后的安全姿态需迁移或运维检查。文件权限只防本机其他普通用户，不防 root、同用户进程或被 Agent 执行的宿主机命令。

## 10. 备份与恢复

完整备份必须形成一致集合，至少包括：

- 活动配置与 SecretRef；
- Agent 定义、workspace、sessions、network、memory、skills；
- projects；
- cron jobs/runs/claims；
- usage、audit 和必要 artifact；
- manifest、版本和每文件摘要。

恢复必须校验路径穿越、符号链接、摘要、版本兼容和归档完整性，再以受限目录解包。仅验证“tar 可生成”不等于恢复可靠；发布 E2E 已覆盖破坏后恢复与重启复核，但新数据域加入时必须同步扩展清单。
