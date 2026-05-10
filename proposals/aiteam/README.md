# `proposals/aiteam/` — aiteam 实验项目对 ZyHive 的需求与提议

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
| **PR-001** | Wallet 钱包抽象 | 📝 待用户填 spec | per-agent 账户、计价单位、计费维度、API |
| **PR-002** | Payroll 工资发放 | 📝 待用户填 spec | 评估 → 给 agent 发钱 |
| **PR-003** | Budget Guard 预算护栏 + panic-stop | 🟡 初稿 v0 已写 | per-agent 软警告 + 硬上限 panic-stop |
| **PR-004** | Judge Agent 评判智能体 | 📝 待用户填 spec | 独立 agent 评分/否决其他 agent 输出 |

### 🟠 P1 · 收入 / 可观测性

| ID | 标题 | 状态 |
|----|------|------|
| PR-005 | Revenue Engine 收入引擎 | 📝 待用户填 spec |
| PR-006 | aiteam-specific Observability | 📝 待用户填 spec（与 ZyHive P0-01/02 协同） |

### 🛡️ 安全护栏（B5/B6 系列）

| ID | 标题 | 状态 |
|----|------|------|
| PR-007 | B5 工具沙箱（exec 隔离） | 📝 待用户填 spec |
| PR-008 | B6 提示词注入防御 | 📝 待用户填 spec |

---

## 3. Bug 报告（B001-B015）

QA 在 ZyHive 主项目发现的漏洞，编号 B001~B015。详见 `bugs/` 子目录。

| ID | 标题 | 严重度 | ZyHive 状态 |
|----|------|--------|-----------|
| **B001** | API + AI 工具路径穿越（含兄弟前缀混淆 / symlink / abs path） | 🔴 CRITICAL | ✅ 已修复 26.5.10v2 |
| **B002** | Bearer / download / media token 时延侧信道 | 🟠 HIGH | ✅ 已修复 26.5.10v3 |
| **B003** | 无界请求体 OOM DoS（gin ShouldBindJSON 无 size cap） | 🟠 HIGH | ✅ 已修复 26.5.10v4 |
| B003 | (待用户贴 markdown) | ? | 未提 |
| B004 | (待用户贴 markdown) | ? | 未提 |
| B005 | (待用户贴 markdown) | ? | 未提 |
| B006 | (待用户贴 markdown) | ? | 未提 |
| B007 | (待用户贴 markdown) | ? | 未提 |
| B008 | (待用户贴 markdown) | ? | 未提 |
| B009 | (待用户贴 markdown) | ? | 未提 |
| B010 | (待用户贴 markdown) | ? | 未提 |
| B011 | (待用户贴 markdown) | ? | 未提 |
| B012 | (待用户贴 markdown) | ? | 未提 |
| B013 | (待用户贴 markdown) | ? | 未提 |
| B014 | (待用户贴 markdown) | ? | 未提 |
| B015 | (待用户贴 markdown) | ? | 未提 |

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

## 5. 待办

- [ ] 用户贴 PR-001/002/004 spec markdown 到对应 stub
- [ ] 用户贴 B002-B015 bug 描述到 `bugs/B0xx-template.md`
- [ ] 用户决定 PR-003 BudgetGuard 初稿里的单位/策略问题（详见 `PR-003-budget-guard.md` § 0）
- [ ] 用户决定是否走 GitHub Security Advisory 私下报 B001（建议私下，CVE 流程）
- [ ] aiteam 项目自己 README / 立项文档放哪里？（建议另开一个 repo 或在本仓 `aiteam/` 顶级目录）
