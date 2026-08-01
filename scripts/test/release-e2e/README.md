# Release 安装更新全流程测试

> 文档版本：V1.2
> 基准日期：2026-08-02
> 状态：当前发布门禁说明
> 适用范围：正式 Release 的全新安装、基础功能、持久化与旧版更新验证

本目录是 ZyHive 正式发布门禁。它不调用真实模型，也不写入当前用户的配置、服务或成员目录。

## 覆盖范围

每次测试都会验证：

1. 发布二进制版本号和 SHA-256；
2. 临时 `HOME` 下的全新安装；
3. 配置生成与管理令牌；
4. 服务启动、`/healthz`、`/readyz` 和版本接口；
5. 未鉴权请求被拒绝；
6. 鉴权后的成员列表和脱敏配置接口；
7. 嵌入管理界面可访问；
8. 旧版到新版的非交互更新；
9. 成员创建、修改、删除和工作区文件读写；
10. 成员与工作区文件跨进程、跨版本更新保持不变；
11. 更新不修改原配置，并保留可执行的 `.bak` 旧版；
12. 损坏校验和必须失败，且不能替换原二进制；
13. 新版本进程通过 `/healthz` 且版本匹配后才确认更新；
14. 新版本失联、健康失败或版本错误时原子恢复 `.bak`；
15. 重复安装同版本保持幂等；
16. 首次设置可保存 Provider、默认模型和随机管理令牌，重装不覆盖配置；
17. Web 渠道密码、公开对话、确定性模型响应和重启保持；
18. 成员、工作区、项目、Goal、Cron 和后台任务形成真实闭环；
19. 完整备份经过 manifest 与摘要检查，破坏数据后可恢复并重启复核；
20. 路径穿越、符号链接、损坏摘要和不完整备份由 Go 回归测试拒绝。

## Draft 候选门禁

- `local`：在隔离 HTTP Release 仓库中验证待发布产物；可传入上一稳定版及对应平台二进制，执行真实旧版更新，也可在开发期使用合成旧版。
- `workflow_dispatch`：`release.sh` 创建不可见 Draft 后，供应链任务生成并复核 SBOM、签名和 provenance，Linux/macOS 的 amd64/arm64 四种原生 Runner 分别运行 `local` 全旅程。
- `promote`：只依赖上述全部门禁；成功时执行唯一一次 Draft → Published 状态转换，失败时保持 Draft。

任何断言失败都会返回非零退出码。候选验证期间旧稳定版始终保持 latest，未验证资产不会先公开再撤回。

## 手动运行

```bash
VERSION=<待测版本>
PREVIOUS_VERSION=<上一正式版本>
ARTIFACT_DIR=<四平台产物与SHA256SUMS所在目录>

scripts/test/release-e2e/run.sh local "$VERSION" "$ARTIFACT_DIR" "$PREVIOUS_VERSION"
scripts/test/release-e2e/run.sh online "$VERSION" "$PREVIOUS_VERSION"
```

传入 `PREVIOUS_VERSION` 时，资产目录需包含 `previous-zyhive-<os>-<arch>`；正式工作流会从上一稳定 Release 自动下载。

测试依赖：`bash`、`curl`、`python3`；`local` 模式额外需要 `go`。
