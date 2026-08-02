# 测试与质量门禁

## 本地必跑

仓库提供与主要 CI 门禁等价的入口：

```bash
make check
```

它依次执行：

```bash
go vet ./...
go test ./... -count=1 -timeout=5m
go build ./...
cd ui && npm ci && npm test && npm run build
make sync-ui
git diff --no-index --exit-code -- ui/dist cmd/aipanel/ui_dist
```

`make check` 会更新 `cmd/aipanel/ui_dist`。执行后必须审查工作区，确认嵌入快照变化与前端源码一致。

## 分层运行

后端全部测试：

```bash
go test ./... -count=1 -timeout=5m
```

单包或单用例：

```bash
go test ./pkg/config -count=1
go test ./internal/api -run TestName -count=1
```

关键并发包 Race：

```bash
go test -race -count=1 -timeout=10m \
  ./internal/api ./pkg/agent ./pkg/artifact ./pkg/config \
  ./pkg/cron ./pkg/network ./pkg/persist ./pkg/project \
  ./pkg/safefs ./pkg/session ./pkg/tools
```

前端：

```bash
cd ui
npm test
npm run build
```

脚本语法和发布契约：

```bash
bash -n scripts/*.sh scripts/test/*.sh scripts/test/release-e2e/*.sh
bash scripts/test/release-supply-chain.sh
python3 scripts/test/release_draft_gate_test.py
```

## CI 门禁

`.github/workflows/ci.yml` 对 PR 和 `main` push 运行：

- 唯一提交身份检查：`scripts/verify-commit-identity.sh`。
- Go vet、全量测试和构建。
- 关键包 Race detector。
- UI 测试、类型检查、构建及嵌入快照一致性。
- Shell 语法、供应链脚本契约和 Draft-first 契约测试。
- Release E2E smoke：原生 fixture 的安装、API 和更新。
- `main` push 后的四平台交叉构建。

`golangci-lint` 当前是建议性检查：问题会出现在日志中，但 job 使用 `--issues-exit-code=0` 且 `continue-on-error`，不是合入阻断门。不要把“CI 绿色”理解为 lint 已清零。

## 测试选择

- 配置字段、SecretRef、事务保存：`pkg/config`。
- 原子写、锁、权限：`pkg/persist` 及对应存储包。
- 路由、鉴权、请求限制、SSE：`internal/api`。
- 路径穿越和符号链接：`pkg/safefs`、API/Manager security tests。
- 会话事实源与索引恢复：`pkg/session`。
- Cron 时区、一次性任务、重叠与恢复：`pkg/cron`。
- Agent CLI 参数、HTTP 映射和退出码：`internal/agentcli`。
- UI 类型和交互逻辑：`ui/src/**/*.test.ts`。

持久化改动至少要覆盖：首次创建、已有数据读取、写失败不污染内存、并发写、旧格式迁移、权限位和损坏索引恢复。

## Release E2E

本机当前平台 smoke：

```bash
make release-e2e RELEASE_VERSION=00.0.0v1
```

完整干运行：

```bash
make release-dry-run RELEASE_VERSION=26.8.2v2
```

正式候选旅程直接调用：

```bash
scripts/test/release-e2e/run.sh local "$VERSION" "$ARTIFACT_DIR" "$PREVIOUS_VERSION"
```

Release E2E 使用隔离的临时 HOME，不调用真实模型，覆盖全新安装、鉴权、核心 API、Web 流、项目/Goal/Cron/任务闭环、备份恢复、真实旧版更新、健康确认和失败回滚。任何断言失败都必须保持候选为 Draft。

## 测试数据纪律

- 使用 `t.TempDir()` 或脚本临时目录，不写开发者真实配置。
- 不依赖真实 API Key；网络集成应使用确定性本地 fixture。
- 断言文件权限时按 Unix 语义运行；Windows 专用锁实现需单独考虑。
- 不用缓存掩盖事实源损坏；应直接破坏索引并验证从权威数据恢复。
- 测试命令默认加 `-count=1`，避免 Go test cache 隐藏回归。
