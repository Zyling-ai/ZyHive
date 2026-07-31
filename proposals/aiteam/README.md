# `proposals/aiteam/` — aiteam 实验项目对 ZyHive 的需求与提议

> **状态边界（2026-08-01）：本目录是历史提案与实验记录。**
>
> 下方优先级和“待填 spec”等状态反映提案形成时点，不代表当前代码状态；部分实验模块已经实现，但仍默认关闭，不属于稳定版默认商业承诺。是否实现以代码和测试为准，是否发布以正式 Release 为准。
>
> 本目录是 **aiteam 实验**（自治经济实体方向）对 ZyHive 主项目的提议合集。
>
> 与同仓的 `proposals/zyhive-improvements/`（ZyHive 通用改进路线）**并行**，目标受众和优先级不同。

---

## 1. 两条路线为什么要分开

| 维度 | ZyHive 主项目（`proposals/zyhive-improvements/`） | aiteam 实验（本目录） |
|------|-------------------------------------------|---------------------|
| 定位 | 通用 AI 团队 OS | 自治经济实体（agent 自己接活赚钱） |
| 用户 | 任何想搭 AI 团队的人 | 只有 aiteam Genesis 实验需要 |
| 路线 | 通讯录 → 群档案 → 会议 → 渠道 → ... | 钱包 → 护栏 → 工资 → 评判 → 议会 |
| 优先级 | README P1 列表 | aiteam 自己的 P0/P1 |
| 影响范围 | ZyHive 全用户可见 | 默认 off，experimental flag 守卫 |

不冲突。aiteam 提议都将以 **`experimental` flag** 守卫合入，**默认关闭**，零影响 ZyHive 主线行为。

---

## 2. 提议清单（PR-XXX）

### 🔴 P0 · Genesis 跑真业务前必备

| ID | 标题 | 状态 | 描述 |
|----|------|-----|------|
| **PR-001** | Wallet 钱包抽象 | ✅ 已实现（Labs） | per-agent 账户、计价单位、计费维度、API |
| **PR-002** | Payroll 工资发放 | ✅ 已实现（Labs） | 评估 → 给 agent 发钱 |
| **PR-003** | Budget Guard 预算护栏 + panic-stop | ✅ 已实现（Labs） | per-agent 软警告 + 硬上限 panic-stop |
| **PR-004** | Judge Agent 评判智能体 | ✅ 已实现（Labs） | 独立 agent 评分/否决其他 agent 输出 |

### 🟠 P1 · 收入 / 可观测性

| ID | 标题 | 状态 |
|----|------|------|
| PR-005 | Revenue Engine 收入引擎 | ✅ 已实现（Labs） |
| PR-006 | aiteam-specific Observability | ✅ 已实现（Labs，含前端） |

### 🛡️ 安全护栏（B5/B6 系列）

| ID | 标题 | 状态 |
|----|------|------|
| PR-007 | B5 工具沙箱（exec 隔离） | ✅ 已实现（Labs；不等于强容器隔离） |
| PR-008 | B6 提示词注入防御 | ✅ 已实现（Labs） |

---

## 3. Bug 报告（B001-B015）

QA 在 ZyHive 主项目发现的漏洞，编号 B001~B015。详见 `bugs/` 子目录。

| ID | 标题 | 严重度 | ZyHive 状态 |
|----|------|--------|-----------|
| **B001** | API + AI 工具路径穿越（含兄弟前缀混淆 / symlink / abs path） | 🔴 CRITICAL | ✅ 已修复 26.5.10v2 |
| **B002** | Bearer / download / media token 时延侧信道 | 🟠 HIGH | ✅ 已修复 26.5.10v3 |
| **B003** | 无界请求体 OOM DoS（gin ShouldBindJSON 无 size cap） | 🟠 HIGH | ✅ 已修复 26.5.10v4 |
| **B004** | Slowloris（http.Server 缺 ReadHeaderTimeout / IdleTimeout） | 🟠 HIGH | ✅ 已修复 26.5.10v5 |
| **B005** | Go stdlib CVE 继承 (Go 1.22.2 toolchain 老) | 🟠 HIGH | ✅ 已修复 `26.5.10v7` |
| **B006** | 飞书 protobuf 整数转换溢出 (G115) | 🟡 MEDIUM | 🟡 开放，需按 v4 重新验证 |
| B007 | 自更新 syscall.Exec 信任 os.Args (G702) | 🟢 LOW | 📝 false positive |
| B008 | CLI $EDITOR 命令执行 (G702) | 🟢 LOW | 📝 false positive |
| B009 | LLM retry 用 math/rand (G404) | 🟢 LOW | 📝 非安全敏感 |
| B010 | Provider 启动自检 SSRF taint (G704) | 🟢 LOW | ✅ `26.8.1v2` 统一出站防护覆盖 |
| B011 | CLI 状态显示文案被误标 hardcoded credentials (G101) | 🟢 LOW | 📝 false positive |
| B012 | 工具 read/write/edit 路径 taint (G703) | 🟢 LOW | ✅ B001 已覆盖 |
| B013 | memory indexer filepath.Walk race (G122) | 🟢 LOW | 🟡 后续切 WalkDir |
| **B014** | 文件/目录权限偏宽 0644/0755 (108 处 G301/G302/G306) | 🟡 MEDIUM | 🟡 部分修复 |
| B015 | 多处 err 静默丢弃 (G104) | 🟢 LOW | 📝 已接受技术债 |

---

## 4. 协作约定

### 提议格式

每个 PR-XXX 一个 markdown 文件，标准段：

```markdown
# PR-XXX · 标题

> 状态: 📝 spec 收集中 / 🟡 初稿 v0 / 🟢 ready for impl / 🔴 阻塞中
> 优先级: 🔴 P0 / 🟠 P1 / 🟡 P2
> 依赖: 无 / PR-XXX

## 0. 待用户提供
- [ ] (列出还缺的关键决策)

## 1. 背景
## 2. 设计
## 3. 实施步骤
## 4. 测试计划
## 5. experimental flag 名称与默认值
## 6. 兼容性 / 回滚
```

### 实施约定

1. **experimental flag 守卫**：所有 aiteam 新功能默认 off，例 `ZYHIVE_EXPERIMENTAL_BUDGETGUARD=1` 才启用
2. **无 schema 强制迁移**：aiteam 数据落到 `workspace/aiteam/` 子目录，不污染 ZyHive 现有目录
3. **测试隔离**：aiteam 测试加 `Test_AITeam_*` 前缀方便 grep
4. **CHANGELOG 标记**：版本条目里 aiteam 改动单独段标 `### aiteam (experimental)`

### 提交节奏

- 每个 PR 独立 git branch + 独立 PR
- B001 已经直接提到 main（CRITICAL 不等了）
- B002-B015 分批提，单 commit 单 bug 利于 review / cherry-pick

---

## 5. 归档结论

原“待填 spec”和模板填充任务已经失效，不再作为当前待办。aiteam 已作为默认关闭的 Labs 能力保留；后续是否继续产品化须重新立项。当前仍需跟踪的安全项以 `bugs/README.md` 为准。
