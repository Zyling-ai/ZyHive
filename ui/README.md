# ZyHive 管理界面

> 文档版本：V1.0
> 基准日期：2026-08-01
> 状态：当前前端开发入口
> 适用范围：`ui/` 下 Vue 3、TypeScript 与 Vite 工程

本目录是 ZyHive 管理界面的源代码，不是独立产品。正式发布时，构建产物必须同步到 `cmd/aipanel/ui_dist/` 并嵌入 Go 单二进制。

## 环境

- Node.js：`>=20.19.0`
- 依赖锁定：`package-lock.json`
- 框架：Vue 3、TypeScript、Vite、Element Plus
- 测试：Vitest + jsdom

## 常用命令

```bash
npm ci
npm test
npm run build
```

`npm run build` 同时执行 TypeScript/Vue 类型检查和生产构建。完整发布与嵌入校验应从仓库根目录运行 `scripts/release.sh <版本> --dry-run`，不能只凭前端本地构建宣布可发布。

## 状态边界

- 前端测试数量仍有限，测试通过不等于全部页面和浏览器旅程均已覆盖；
- `dist/` 是本地构建输出，不是权威发布资产；
- 正式版本、安装方式和产品边界统一读取仓库根目录 `README.md`、`CHANGELOG.md` 和 GitHub Releases。
