# P0-03 · GitHub Actions CI workflow + lint

- 主题：H 工程化
- 优先级：P0
- 规模：S（新增 `.github/workflows/`、`.golangci.yml`、`ui/.eslintrc.cjs`）
- 状态：proposed

## 1. 背景与问题

`.github/` 目前只有 `ISSUE_TEMPLATE/`，没有任何自动化：

- 提交一份 PR 没有任何门禁，依赖维护者本地跑 `make build` + `go test ./...`
- 仓库内已经有不少测试（`pkg/llm/retry_test.go` `pkg/tools/*_test.go` `pkg/agent/definition_test.go` `internal/api/relations_test.go`），但没有 CI 跑
- 前端 `ui/` 没有 lint 配置（只有 `vite` + `tsconfig`）
- Release 构建 `make release` 完全靠 `scripts/release.sh` 手动

## 2. 目标 & 非目标

**目标**：

1. PR 触发：`go test ./...`、`go vet ./...`、`go build ./...`、`golangci-lint`、`vite build`
2. 主分支推送触发：以上 + 多平台二进制构建（先产物上传 artifact，不发 release）
3. 添加 `.golangci.yml` 基础规则集（`govet errcheck staticcheck gosimple ineffassign unused`）
4. 添加 `ui/.eslintrc.cjs` + `ui/.prettierrc`（最小 vue 3 + ts 规则）
5. README 顶部加 CI 状态徽章

**非目标**：

- Release 自动化（独立 P1-20 提案处理）
- 覆盖率门槛（独立 H-02 / P1-XX 提案）
- E2E 冒烟测试（独立 H-03 / P1-XX 提案）

## 3. 设计要点

### 3.1 `.github/workflows/ci.yml`

```yaml
name: CI
on:
  pull_request:
  push:
    branches: [main]

jobs:
  go:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.22' }
      - run: go vet ./...
      - run: go test ./... -count=1
      - run: go build ./...
      - uses: golangci/golangci-lint-action@v6
        with: { version: latest }

  ui:
    runs-on: ubuntu-latest
    defaults: { run: { working-directory: ui } }
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with: { node-version: '20', cache: 'npm', cache-dependency-path: ui/package-lock.json }
      - run: npm ci
      - run: npm run build
      - run: npm run lint --if-present
```

### 3.2 `.golangci.yml`

```yaml
run:
  timeout: 5m
  go: '1.22'
linters:
  enable: [govet, errcheck, staticcheck, gosimple, ineffassign, unused, gofmt]
issues:
  exclude-dirs: [ui_dist, cmd/aipanel/ui_dist]
  exclude-rules:
    - path: _test\.go
      linters: [errcheck]
```

### 3.3 ui lint

`ui/package.json` 加 `"lint": "eslint src --ext .ts,.vue"` script + 安装 `eslint eslint-plugin-vue @typescript-eslint/parser @typescript-eslint/eslint-plugin`，配置最小集合：

- `vue/multi-word-component-names: off`（视图命名约定如 `ChatHomeView` 等已定）
- `@typescript-eslint/no-explicit-any: warn`

### 3.4 二进制构建（push 到 main）

```yaml
  release-build:
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    needs: [go, ui]
    strategy:
      matrix:
        include:
          - { goos: linux,  goarch: amd64 }
          - { goos: linux,  goarch: arm64 }
          - { goos: darwin, goarch: arm64 }
    steps: ...  # 使用 make release / scripts/release.sh 现有机制
```

仅上传 artifact，不发 release（留给 P1-20 release-automation）。

## 4. 影响面

| 路径 | 改动 |
|------|------|
| `.github/workflows/ci.yml` | 新增 |
| `.golangci.yml` | 新增 |
| `ui/.eslintrc.cjs` | 新增 |
| `ui/.prettierrc` | 新增 |
| `ui/package.json` | 加 `lint` script + devDeps |
| `README.md` | 顶部加 `![CI](https://github.com/.../workflows/CI/badge.svg)` |
| `Makefile` | 可选加 `make lint` 目标 |

## 5. 迁移与兼容

- 不改运行时行为
- 第一次开 lint 大概率会有若干旧告警；按"warn 不阻断、error 阻断"分级，第一次合入只开 warn 级别

## 6. 测试计划

- 在 PR 上跑通即视为通过
- 故意 push 一个明显错误（如未使用的 import）验证 CI 红

## 7. 文档与 CHANGELOG

- README 顶部加徽章
- 新增 `docs/contributing.md`（可选）描述本地命令：`go test ./... && (cd ui && npm run lint && npm run build) && make build`
- CHANGELOG 单条

## 8. 风险与回滚

- 风险：第一次开 lint 在历史代码爆出大量告警阻塞开发。缓解：第一次合入用 `golangci-lint --new-from-rev=origin/main`（仅检查变更行）；存量逐步清理。
- 回滚：禁用对应 step 即可。
