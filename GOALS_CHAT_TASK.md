# GoalsView 右侧对话面板 — 增量任务

## 背景
GoalsView.vue（`ui/src/views/GoalsView.vue`）已实现甘特图+表单。  
现在要参照 `ui/src/components/SkillStudio.vue` 的右侧 AiChat 面板模式，  
为 GoalsView 添加右侧对话区域——用户可以通过和 AI 成员聊天来创建/修改目标，省去填写表单的麻烦。

## 必读参考文件（先读懂再动手）
- `ui/src/components/SkillStudio.vue` — 右侧 AiChat 面板完整实现
- `ui/src/components/AiChat.vue` — AiChat 组件接口（props: agentId, sessionKey, context, welcomeMessage, examples）
- `ui/src/views/GoalsView.vue` — 当前 Goals 页面，需要在此基础上改

## 需要修改的文件

### 1. `ui/src/views/GoalsView.vue`

**布局改为两栏（参考 SkillStudio 的 `.studio-main` 布局）：**
```
┌─────────────────────────────────┬─────────────────┐
│  左侧：甘特图 + 目标列表（现有）   │ ║  右侧：AI 对话  │
│  （可伸缩，最小 500px）           │ ║  面板（340px）  │
└─────────────────────────────────┴─────────────────┘
```

**具体改动：**

1. 在现有内容外层套一个 flex 容器 `.goals-layout`（flex-direction: row）

2. 左侧 `.goals-main` 包裹现有所有甘特图+列表内容，flex: 1

3. 右侧 `.goals-chat` 宽度默认 340px，可拖拽调整（参考 SkillStudio 的 `chatW` + `startResize`）：
   - 拖拽手柄 `.ss-handle`（复用 SkillStudio 的样式）
   - 顶部 header：图标（ChatLineRound）+ 「AI 目标助手」+ 成员选择下拉
   - 主体：`<AiChat>` 组件

4. AiChat 配置：
```vue
<AiChat
  :agent-id="chatAgentId"
  :session-key="`goals-chat-${chatAgentId}`"
  :context="goalChatContext"
  welcome-message="你好！我可以帮你创建和管理目标。比如：「帮我创建一个Q2增长目标，让引引负责，3月到6月，每周检查一次」"
  :examples="[
    '帮我创建一个团队目标：Q2用户增长，3月1日到6月30日',
    '给「产品发布」目标添加3个里程碑',
    '查看当前所有进行中的目标',
    '帮我设置每周一检查「增长目标」的进度',
  ]"
  style="height: 100%"
/>
```

5. `chatAgentId`：默认取 agentList 第一个非系统成员，顶部下拉可切换

6. `goalChatContext`（注入给 AI 的上下文，让它知道如何操作目标）：
```typescript
const goalChatContext = computed(() => `
## 目标规划助手

你是团队的目标规划助手。你可以通过调用 API 来帮用户创建和管理目标。

### 可用操作（通过 bash 工具调用）

**创建目标：**
\`\`\`bash
curl -s -X POST http://localhost:8080/api/goals \\
  -H "Authorization: Bearer TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{"title":"目标名","type":"team","agentIds":["agentId"],"startAt":"2026-03-01T00:00:00Z","endAt":"2026-06-30T00:00:00Z","status":"active"}'
\`\`\`

**列出目标：**
\`\`\`bash
curl -s http://localhost:8080/api/goals -H "Authorization: Bearer TOKEN"
\`\`\`

**更新进度：**
\`\`\`bash
curl -s -X PATCH http://localhost:8080/api/goals/{id}/progress \\
  -H "Authorization: Bearer TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{"progress": 50}'
\`\`\`

**添加里程碑：**（通过 PATCH /api/goals/{id} 更新 milestones 数组）

**添加定期检查：**
\`\`\`bash
curl -s -X POST http://localhost:8080/api/goals/{id}/checks \\
  -H "Authorization: Bearer TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{"name":"每周检查","schedule":"0 9 * * 1","agentId":"agentId","prompt":"请检查目标「{goal.title}」本周进展（当前{goal.progress}）"}'
\`\`\`

### 当前团队成员
${agentListContext}

### 当前目标列表
需要时可先 list 查询再操作。

创建完成后告诉用户「目标已创建，页面会自动刷新」。
`.trim())
```

注意：TOKEN 和 localhost:8080 从 store/config 里取，agentListContext 是当前 agentList 格式化后的字符串（id: name）。

7. 目标列表自动刷新：在 AiChat 发送消息后（监听 AiChat 的 `@message-sent` 事件或定时 poll），每次 AI 回复后 2 秒自动 `loadGoals()`

8. CSS 补充（复用 SkillStudio 的 `.ss-handle`、`.ss-handle-bar` 样式，或直接在 GoalsView scoped style 里重写）：
```css
.goals-layout {
  display: flex;
  height: 100%;
  overflow: hidden;
}
.goals-main {
  flex: 1;
  min-width: 500px;
  overflow-y: auto;
  padding: 0 16px 16px;
}
.goals-chat {
  display: flex;
  flex-direction: column;
  border-left: 1px solid var(--el-border-color-light);
  background: var(--el-bg-color);
  overflow: hidden;
}
.chat-panel-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border-bottom: 1px solid var(--el-border-color-light);
  font-size: 13px;
  font-weight: 500;
  color: var(--el-text-color-primary);
  flex-shrink: 0;
}
```

### 2. 不需要改后端

AI 通过 bash 工具直接调用现有 REST API，不需要新增后端接口。

## 完成后
1. `cd ui && npm run build`
2. `make build`
3. 确认 `/goals` 页面右侧有 AI 对话面板，可以对话后自动刷新目标列表

完成后执行：
openclaw system event --text "GoalsView 右侧对话面板完成" --mode now
