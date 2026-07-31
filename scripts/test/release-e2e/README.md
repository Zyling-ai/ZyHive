# Release 安装更新全流程测试

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
9. 更新不修改原配置，并保留可执行的 `.bak` 旧版；
10. 重复安装同版本保持幂等。

## 两层门禁

- `local`：正式发布前运行。本机启动临时 HTTP Release 仓库，用合成旧版验证当前待发布产物。
- `online`：Release 创建后运行。从 GitHub 下载正式 `install.sh` 和二进制，并使用上一个正式版本验证真实更新。

任何断言失败都会返回非零退出码。正式发布脚本会把失败的 Release 转为草稿，避免继续作为公开最新版。

## 手动运行

```bash
scripts/test/release-e2e/run.sh local 26.8.1v1 /tmp/zyhive-release-26.8.1v1
scripts/test/release-e2e/run.sh online 26.8.1v1 26.7.31v4
```

测试依赖：`bash`、`curl`、`python3`；`local` 模式额外需要 `go`。
