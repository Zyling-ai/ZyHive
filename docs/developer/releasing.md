# 发布流程

正式发布不是直接上传二进制。当前流程先创建不可见 Draft，完成可复现构建、四平台原生旅程和供应链验证后，才由唯一 promote job 公开。

## 版本与支持平台

版本格式：

```text
YY.M.DvN
```

例如 `26.8.2v2`。`scripts/release.sh` 使用正则强制该格式。

官方资产：

- `zyhive-linux-amd64`
- `zyhive-linux-arm64`
- `zyhive-darwin-amd64`
- `zyhive-darwin-arm64`
- `install.sh`
- `SHA256SUMS`

正式构建使用 `CGO_ENABLED=0`、`-buildvcs=false`、`-trimpath`，并通过 `-X main.Version=<version>` 注入版本。

## 发布前检查

需要：

- Go 恰好为 1.25.10；
- Node >= 22.13.0；
- `git go node npm curl python3`；
- 正式发布还需 `gh` 且已认证；
- 当前分支为 `main`；
- 工作区干净；
- 提交身份通过 `scripts/verify-commit-identity.sh`；
- 目标 Release 不存在。

先执行干运行：

```bash
make release-dry-run RELEASE_VERSION=26.8.2v2
```

它会运行后端门禁、前端测试/构建、嵌入同步、四平台构建、校验和及隔离 Release E2E，但不会创建 GitHub Release。

## 正式发布

```bash
make release RELEASE_VERSION=26.8.2v2
```

`scripts/release.sh` 的关键阶段：

1. `go vet`、全量测试、Go build。
2. `npm ci`、前端测试、类型检查和构建。
3. 同步并比较嵌入 UI。
4. 构建四个平台。
5. 生成并验证 SHA-256。
6. 在隔离环境验证安装、API、更新和恢复。
7. 创建不可见 GitHub Draft Release。
8. 触发 `release-e2e.yml`，等待候选门禁。
9. 仅在工作流已公开候选后报告成功。

脚本失败时 Draft 保持不可见；不要手工把失败候选标成 latest。

## 候选工作流门禁

`release-e2e.yml` 先验证：

- Release ID、tag、target commit、checkout SHA 完全一致；
- 候选仍为 Draft，提交位于 `main` 历史；
- 唯一提交身份；
- 已上传资产的校验和；
- 从相同 SHA 重建四个二进制并逐字节 `cmp`；
- `install.sh` 与仓库版本一致。

供应链阶段随后：

- 使用固定版本 Syft 生成 SPDX JSON SBOM；
- 生成 GitHub build provenance；
- 使用 Sigstore OIDC keyless 签名资产、校验和和 SBOM；
- 重新下载并验证签名、证书身份和 attestation；
- 把验证后的候选传给四种原生 Runner。

四个 Runner 分别执行全新安装、真实旧版更新（如提供上一版本）、核心 API/数据旅程、完整备份恢复和健康回滚。全部通过后，`promote` 再次确认 Draft/commit，执行唯一的 Draft → Published/latest 转换。

## 本地 Release E2E

当前主机产物：

```bash
make release-e2e RELEASE_VERSION=00.0.0v1
```

指定资产：

```bash
scripts/test/release-e2e/run.sh local "$VERSION" "$ARTIFACT_DIR" "$PREVIOUS_VERSION"
```

验证已下载供应链资产：

```bash
./scripts/release.sh --verify-supply-chain "$VERSION" "$ASSET_DIR"
```

如需非默认证书身份，可设置 `ZYHIVE_CERTIFICATE_IDENTITY`；仓库和临时产物目录可分别通过 `ZYHIVE_REPO`、`ZYHIVE_DIST_DIR` 指定。

## 失败处理

- 构建/测试失败：修复代码后使用新候选；不要绕过门禁。
- Draft 工作流失败：保留 Draft 供调查，旧稳定版继续是 latest。
- 已有同名 Release：使用新的版本号，不覆盖公开历史。
- 资产不匹配：确认 UI 快照、Go 工具链、`-buildvcs=false` 和候选 SHA。
- 原生 Runner 失败：以该平台旅程日志为准，不用交叉编译成功替代运行验证。

发布后应确认 Release 为公开 latest、安装端点返回新版本、四平台资产及 `SHA256SUMS`、SBOM、Sigstore bundle 和 provenance 均可验证。
