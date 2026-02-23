# ZyHive Install Worker

Cloudflare Worker，部署在 `install.zyling.ai`，为 ZyHive 提供：

- **安装脚本代理** — `GET /zyhive.sh` 实时返回最新安装脚本
- **版本查询** — `GET /latest` 返回最新 Release 版本号（5分钟缓存）
- **二进制下载代理** — `GET /dl/{version}/{filename}` 代理 GitHub Release 下载，解决国内无法访问 GitHub 的问题，CF 边缘节点缓存加速

## 使用

```bash
curl -sSL https://install.zyling.ai/zyhive.sh | bash
```

## 部署

```bash
npm install -g wrangler
wrangler login
wrangler deploy
```

或通过 GitHub Actions 自动部署（推送 main 分支即触发）。

需要在 GitHub 仓库 Secrets 中配置：
- `CLOUDFLARE_API_TOKEN` — CF API Token（需要 Worker 编辑权限）
