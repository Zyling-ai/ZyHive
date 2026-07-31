# ZyHive Install Worker

> 文档版本：历史兼容说明 V1
> 最近复核：2026-08-01
> 状态：旧版独立 Worker，停止作为正式部署源
> 适用范围：追溯旧安装代理实现；不得直接用于当前生产部署

当前 `install.zyling.ai` 已由 `zyling-website` 仓库中的统一 Cloudflare Worker 承载，版本只跟随 GitHub 最新正式 Release。此目录保留旧版独立 Worker 代码用于追溯，不应单独部署，否则可能覆盖现行官网和安装分发路由。

旧 Worker 曾提供：

- **安装脚本代理** — `GET /zyhive.sh` 从 GitHub `main` 的 `scripts/install.sh` 读取脚本；这与现行“只跟随正式 Release”的规则冲突
- **版本查询** — `GET /latest` 返回最新 Release 版本号（5分钟缓存）
- **二进制下载代理** — `GET /dl/{version}/{filename}` 代理 GitHub Release 下载，解决国内无法访问 GitHub 的问题，CF 边缘节点缓存加速

## 当前正式入口

```bash
curl -fsSL https://install.zyling.ai/install | bash
```

`/zyhive.sh` 仅作为统一官网 Worker 的兼容脚本端点。当前实现、路由和测试以独立的 `zyling-website` 仓库为准，不以本目录 `worker.js` 为准。

## 历史部署方式（禁止直接用于当前生产环境）

```bash
npm install -g wrangler
wrangler login
wrangler deploy
```

目录中的旧 GitHub Actions 仅属于历史实现说明，不代表当前 `main` 推送会部署该 Worker。

需要在 GitHub 仓库 Secrets 中配置：
- `CLOUDFLARE_API_TOKEN` — CF API Token（需要 Worker 编辑权限）
