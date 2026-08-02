# 开发环境与本地启动

## 前置条件

- Go **1.25.10**。`go.mod`、CI 和正式发布脚本都固定到该版本。
- Node.js **22.13.0 或更高**，以及 npm。前端 `package.json` 声明了该下限。
- macOS 或 Linux；正式产物覆盖 Linux/macOS 的 amd64、arm64。
- `git`、`bash`、`curl`、`python3`；正式发布还需要已认证的 GitHub CLI `gh`。

确认版本：

```bash
go version
node --version
npm --version
```

## 安装依赖

Go 依赖由模块系统按需下载。前端应使用锁文件安装：

```bash
cd /path/to/ZyHive
cd ui
npm ci
cd ..
```

日常需要更新前端依赖时才使用 `npm install`，并审查 `ui/package-lock.json` 的变化。

## 构建

完整构建：

```bash
make build
./bin/aipanel --version
```

目标执行顺序为：

1. `cd ui && npm run build`
2. 把 `ui/dist` 完整复制到 `cmd/aipanel/ui_dist`
3. 使用当前 Git tag 或提交摘要注入 `main.Version`
4. 编译 `bin/aipanel`

仅后端变更且确认嵌入 UI 已同步时可运行：

```bash
make go-only
```

## 最小本地配置

以下配置不包含模型，因此服务可启动但聊天会返回 `no model configured`。它适合 API、数据层和 UI 壳联调：

```bash
umask 077
cat > /tmp/zyhive-dev.json <<'JSON'
{
  "configVersion": 3,
  "gateway": {
    "port": 8080,
    "bind": "localhost"
  },
  "agents": {
    "dir": "/tmp/zyhive-dev-data/agents"
  },
  "providers": [],
  "models": [],
  "channels": [],
  "tools": [],
  "skills": [],
  "auth": {
    "mode": "token",
    "token": "dev-only-change-me"
  }
}
JSON
chmod 600 /tmp/zyhive-dev.json
```

启动：

```bash
./bin/aipanel --serve --config /tmp/zyhive-dev.json
```

另一个终端验证：

```bash
curl -fsS http://localhost:8080/healthz
curl -fsS http://localhost:8080/readyz
curl -fsS \
  -H 'Authorization: Bearer dev-only-change-me' \
  http://localhost:8080/api/agents
```

`make run` 固定执行 `AIPANEL_CONFIG=aipanel.json ./bin/aipanel`，因此需要仓库根目录存在相应文件。要使用其他配置路径，直接运行二进制：

```bash
AIPANEL_CONFIG=/tmp/zyhive-dev.json ./bin/aipanel
```

## 配置真实模型

推荐在 `providers[]` 保存凭据，在 `models[]` 通过 `providerId` 引用。开发机可用 SecretRef 避免把明文写入 JSON：

```json
{
  "providers": [
    {
      "id": "openai-dev",
      "name": "OpenAI Dev",
      "provider": "openai",
      "apiKey": "{\"$env\":\"OPENAI_API_KEY\"}",
      "status": "untested"
    }
  ],
  "models": [
    {
      "id": "gpt-dev",
      "name": "GPT Dev",
      "provider": "openai",
      "model": "gpt-4o",
      "providerId": "openai-dev",
      "isDefault": true,
      "status": "untested"
    }
  ]
}
```

这里的 `apiKey` 在 Go 结构中仍是字符串，因此 SecretRef 的 wire 形式是“包含 JSON 对象文本的 JSON 字符串”，不是直接嵌套对象。详见[配置结构](../reference/configuration-schema.md)。

## 开发服务与前端热更新

后端：

```bash
make go-only
./bin/aipanel --serve --config /tmp/zyhive-dev.json
```

前端：

```bash
cd ui
npm run dev
```

前端 API 基址由 `ui/src/api/base.ts` 的运行时逻辑决定。跨源联调时需要在 `gateway.cors.allowedOrigins` 显式允许 Vite 地址；同源生产访问无需加入列表。

## 清理

```bash
make clean
rm -rf /tmp/zyhive-dev-data /tmp/zyhive-dev.json
```

`make clean` 会删除前端构建结果、嵌入快照和本地产物；重新构建前必须再次执行 `make build`。
