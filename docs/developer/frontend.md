# 前端开发

ZyHive 管理界面位于 `ui/`，技术栈为 Vue 3、TypeScript、Vite、Pinia、Vue Router、Element Plus、Axios 和 Vitest。Node 最低版本为 22.13.0。

## 常用命令

```bash
cd ui
npm ci
npm run dev
npm test
npm run build
npm run preview
```

`npm run build` 先执行 `vue-tsc -b`，再执行 Vite 构建；类型错误会阻止生成产物。

## API 层

- `ui/src/api/base.ts`：API 基址和 URL 拼接。
- `ui/src/api/index.ts`：主要 REST 类型、调用封装和聊天 SSE 客户端。
- Axios 请求拦截器每次从本地存储读取 `aipanel_token`，并发送 `Authorization: Bearer <token>`。
- 收到 `401` 后会删除本地 token，并在非登录页跳转 `/login`。

新增后端端点时：

1. 在 `internal/api/router.go` 注册路由并明确公开/鉴权边界。
2. 在 `ui/src/api/index.ts` 增加准确的请求和响应类型。
3. 不要在视图中重复拼接服务地址或鉴权头。
4. 对不兼容响应做显式降级，不要用无边界的 `any` 隐藏契约变化。

## SSE 客户端约定

管理聊天使用 `fetch`，而非 Axios：

- `POST /api/agents/:id/chat` 创建/继续生成；
- `GET /api/agents/:id/chat/stream?sessionId=...` 重连；
- 每条业务事件是单行 `data: <JSON>`；
- `: keepalive` 是注释，客户端应忽略；
- `done`、`error` 或 `idle` 终止当前订阅；
- 切换会话或卸载组件时应调用 `AbortController.abort()`，防止旧流污染新会话。

连接断开不代表生成取消。后端 Worker 独立于 HTTP 连接继续运行，前端应先查询状态或重连，而不是无条件重复提交同一消息。

## 开发代理与 CORS

同源部署时页面和 `/api` 由同一二进制提供，不需要 CORS 配置。Vite 开发服务器跨源访问后端时，把完整 Origin 加入配置：

```json
{
  "gateway": {
    "port": 8080,
    "bind": "localhost",
    "cors": {
      "allowedOrigins": ["http://localhost:5173"]
    }
  }
}
```

Origin 必须只有 scheme、host 和可选端口；带路径、查询或用户信息的值会被忽略。不要用 `*` 替代明确来源。

## 嵌入式 UI

生产 UI 通过 `go:embed all:ui_dist` 编译进二进制，源目录是 `cmd/aipanel/ui_dist/`。完整同步流程：

```bash
cd ui
npm run build
cd ..
make sync-ui
git diff --no-index --exit-code -- ui/dist cmd/aipanel/ui_dist
```

规则：

- `ui/dist/` 是本地构建目录。
- `cmd/aipanel/ui_dist/` 是必须随源码保持同步的嵌入快照。
- 修改前端后只运行 `go build` 会继续嵌入旧页面。
- CI 会重新构建并比较两个目录；不一致会失败。
- 哈希静态资源使用长期 immutable 缓存，`index.html` 和 SPA fallback 禁止缓存。

## 测试

运行全部前端测试：

```bash
cd ui
npm test
```

聚焦单文件时可直接调用 Vitest：

```bash
cd ui
npx vitest run src/path/to/example.test.ts --environment jsdom
```

提交前至少执行：

```bash
cd ui
npm test
npm run build
cd ..
make sync-ui
git diff --no-index --exit-code -- ui/dist cmd/aipanel/ui_dist
```

涉及登录、SSE、审批、会话切换或上传下载的变更，应同时补后端契约测试，因为这些能力的安全边界不只存在于前端。
