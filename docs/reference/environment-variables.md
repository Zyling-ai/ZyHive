# 环境变量参考

本页按来源和用途列关键变量，不手工复制所有测试/部署脚本的临时开关。新增变量时以源码搜索和对应脚本 `--help` 为准。

## 运行时入口

- `AIPANEL_CONFIG`：服务和 CLI 的配置文件路径。显式 `--config` 优先。
- `LOG_FORMAT`：`text` 或 `json`，默认 `text`。
- `LOG_LEVEL`：`debug`、`info`、`warn`、`error`，默认 `info`。
- `PUBLIC_IP`：部分启动输出/外部地址探测的显式覆盖。
- `EDITOR`：交互式运维面板编辑原始 JSON 时使用，默认 `vi`。
- `ZYHIVE_BROWSER_BIN`：Chromium/Chrome 可执行文件。优先于系统探测和自动下载。

权威来源：`cmd/aipanel/main.go`、`cmd/aipanel/cli.go`、`pkg/logging`、`pkg/browser/manager.go`。

## Agent CLI

- `ZYHIVE_HOST`：服务基址，例如 `http://localhost:8080`。
- `ZYHIVE_TOKEN`：管理 Bearer token。
- `AIPANEL_CONFIG`：当 host/token 未显式给出时，用于加载本地配置。

优先级是 CLI flag > 环境变量 > 配置文件 > host 默认值。不要在共享 shell history 或 CI 日志中明文回显 token。

## Provider 凭据

当前 API 自动探测的关键变量：

- `ANTHROPIC_API_KEY`
- `OPENAI_API_KEY`

模型解析顺序是配置 Provider/模型凭据优先，再尝试对应环境变量。其他 Provider 通常通过 `providers[]` 配置；若要完全避免配置明文，可在任意受支持 credential 字段使用 SecretRef：

```json
{
  "apiKey": "{\"$env\":\"MY_PROVIDER_API_KEY\"}"
}
```

SecretRef 可引用任意环境变量名，不限于上面两个。被引用变量必须存在且非空，否则配置加载失败。

## HTTP 与公开聊天限制

- `ZYHIVE_MAX_REQUEST_BODY_MB`：普通请求正文上限，正整数 MiB；默认 4。文件上传端点有自身限制。
- `ZYHIVE_PUBLIC_REQUESTS_PER_MINUTE`：每来源所有公开接口请求数，默认 60。
- `ZYHIVE_PUBLIC_RATE_PER_MINUTE`：每来源公开模型执行数，默认 12。
- `ZYHIVE_PUBLIC_MAX_SESSIONS`：每来源在 session TTL 内的会话数，默认 8。
- `ZYHIVE_PUBLIC_MAX_ACTIVE_SESSIONS`：全局活跃会话数，默认 100。
- `ZYHIVE_PUBLIC_MAX_CONCURRENT`：公开模型并发，默认 4。
- `ZYHIVE_PUBLIC_MAX_SSE`：公开 SSE 连接数，默认 32。
- `ZYHIVE_PUBLIC_MAX_MESSAGE_BYTES`：单条公开消息字节数，默认 16384。
- `ZYHIVE_PUBLIC_RUN_TIMEOUT`：单次公开生成时限，默认 `2m`。
- `ZYHIVE_PUBLIC_SESSION_TTL`：每来源会话计数保留时长，默认 `24h`。
- `ZYHIVE_PUBLIC_ACTIVE_SESSION_TTL`：全局活跃会话保留时长，默认 `30m`。
- `ZYHIVE_TRUST_PROXY_HEADERS`：仅值 `1` 时信任 `CF-Connecting-IP`、`X-Real-IP`。只应在服务明确位于可信反向代理后启用。

整数/时长值解析失败或不大于零时回退默认值，而不是禁用限制。

## aiteam 实验开关

以下默认关闭：

- `ZYHIVE_EXPERIMENTAL_WALLET`
- `ZYHIVE_EXPERIMENTAL_PAYROLL`
- `ZYHIVE_EXPERIMENTAL_BUDGETGUARD`
- `ZYHIVE_EXPERIMENTAL_JUDGE`
- `ZYHIVE_EXPERIMENTAL_REVENUE`
- `ZYHIVE_EXPERIMENTAL_SANDBOX`
- `ZYHIVE_EXPERIMENTAL_PROMPTDEF`
- `ZYHIVE_EXPERIMENTAL_AITEAM_DASHBOARD`

启用值不区分大小写地接受 `1`、`true`、`yes`、`on`；其他值均关闭。关闭时相关路由返回 404、工具不注册。

相关关键参数：

- `ZYHIVE_AITEAM_REVENUE_SECRET`：Revenue webhook secret；缺失时即使开关开启也禁用收入接入。
- `ZYHIVE_AITEAM_PAYROLL_TIME`、`ZYHIVE_AITEAM_PAYROLL_TZ`：工资调度时间和时区。
- `ZYHIVE_AITEAM_PANIC_TG_CHAT`：紧急通知 Telegram chat。

实验模块不是稳定兼容承诺，部署前阅读 `docs/aiteam-*.md` 和实际实现。

## 安装器

`scripts/install.sh` 的关键覆盖：

- `ZYHIVE_INSTALL_BASE`
- `ZYHIVE_GITHUB_API_URL`
- `ZYHIVE_GITHUB_DOWNLOAD_BASE`
- `ZYHIVE_VERSION`
- `ZYHIVE_DISABLE_FALLBACK`
- `ZYHIVE_VERIFY_SIGNATURE`
- `ZYHIVE_CERTIFICATE_IDENTITY`

签名严格模式应使用安装器参数或 `ZYHIVE_VERIFY_SIGNATURE=1`；该变量只接受 `0`/`1`。

## 发布

- `ZYHIVE_REPO`：GitHub 仓库，默认 `Zyling-ai/ZyHive`。
- `ZYHIVE_DIST_DIR`：本地发布产物目录。
- `ZYHIVE_CERTIFICATE_IDENTITY`：验证 Sigstore bundle 时覆盖证书身份。

Release E2E 还使用 `ZYHIVE_RELEASE_E2E`、`ZYHIVE_UPDATE_HEALTH_TIMEOUT` 及若干等待次数变量。这些是测试夹具接口，不应作为生产调优项。

## 测试开关

- `ZYHIVE_BROWSER_E2E=1`：运行需要真实 Chromium 的集成测试。
- 测试源码中的 `ZYHIVE_TEST_*` 仅用于隔离用例。

## 发现新增变量

从仓库根目录执行：

```bash
rg 'os\.(Getenv|LookupEnv)\(' --glob '*.go'
rg 'ZYHIVE_[A-Z0-9_]+' scripts .github --glob '*.sh' --glob '*.yml'
```

环境变量并不都属于公共稳定 API。判断依据依次为：运行时源码、安装/发布脚本、自动化测试、本文档。
