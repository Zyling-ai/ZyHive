<template>
  <div class="create-layout">
    <!-- ═══ 左栏：配置表单 ═══ -->
    <div class="create-left">
      <div class="create-header">
        <el-button text @click="$router.push('/agents')" class="back-btn">
          <el-icon><ArrowLeft /></el-icon> 返回
        </el-button>
        <h2 style="margin: 0">新建 AI 员工</h2>
      </div>

      <el-form :model="form" label-position="top" class="create-form">
        <!-- 基本信息 -->
        <div class="form-section">
          <div class="section-title">基本信息</div>
          <el-form-item label="名称" required>
            <el-input v-model="form.name" placeholder="如：电商客服助手" @input="autoId" />
          </el-form-item>
          <el-form-item label="ID">
            <el-input v-model="form.id" placeholder="英文标识（自动生成）" />
          </el-form-item>
          <el-form-item label="描述">
            <el-input v-model="form.description" type="textarea" :rows="2"
              placeholder="简短描述这个 Agent 的职责" />
          </el-form-item>
          <el-form-item label="头像颜色">
            <div class="color-row">
              <div v-for="color in avatarColors" :key="color" class="color-swatch"
                :class="{ active: form.avatarColor === color }"
                :style="{ background: color }"
                @click="form.avatarColor = color" />
            </div>
          </el-form-item>
        </div>

        <!-- 身份 & 灵魂 -->
        <div class="form-section">
          <div class="section-title">
            身份 & 灵魂
            <span v-if="aiFilledFields.has('identity') || aiFilledFields.has('soul')" class="ai-badge">AI 生成</span>
          </div>
          <el-form-item>
            <template #label>
              <span>IDENTITY <span class="field-hint">— 角色定义</span></span>
              <el-button v-if="aiFilledFields.has('identity')" text size="small"
                @click="revertField('identity')" class="revert-btn">↺ 撤销</el-button>
            </template>
            <el-input v-model="form.identity" type="textarea" :rows="5"
              :class="{ 'ai-filled': aiFilledFields.has('identity') }"
              placeholder="你是一个...（描述 Agent 的角色和能力）"
              @input="aiFilledFields.delete('identity')" />
          </el-form-item>
          <el-form-item>
            <template #label>
              <span>SOUL <span class="field-hint">— 行为风格</span></span>
              <el-button v-if="aiFilledFields.has('soul')" text size="small"
                @click="revertField('soul')" class="revert-btn">↺ 撤销</el-button>
            </template>
            <el-input v-model="form.soul" type="textarea" :rows="5"
              :class="{ 'ai-filled': aiFilledFields.has('soul') }"
              placeholder="语气亲切，回答简洁...（描述 Agent 的个性风格）"
              @input="aiFilledFields.delete('soul')" />
          </el-form-item>
        </div>

        <!-- 模型 -->
        <div class="form-section">
          <div class="section-title">模型</div>
          <el-form-item label="选择模型">
            <el-select v-model="form.modelId" placeholder="选择模型" style="width: 100%">
              <el-option v-for="m in modelList" :key="m.id"
                :label="`${m.name}（${m.provider}）`" :value="m.id" />
            </el-select>
          </el-form-item>
        </div>

        <!-- 消息通道 -->
        <div class="form-section">
          <div class="section-title">消息通道</div>
          <div v-if="channelList.length === 0" class="empty-hint">
            暂无通道，先在<el-link @click="$router.push('/config/channels')" type="primary"> 全局配置 </el-link>添加
          </div>
          <el-checkbox-group v-else v-model="form.channelIds">
            <el-checkbox v-for="ch in channelList" :key="ch.id" :label="ch.id" :value="ch.id">
              {{ ch.name }} <el-tag size="small" style="margin-left:4px">{{ ch.type }}</el-tag>
            </el-checkbox>
          </el-checkbox-group>
        </div>

        <!-- 能力 -->
        <div class="form-section">
          <div class="section-title">开启能力</div>
          <div v-if="toolList.length === 0" class="empty-hint">
            暂无能力，先在<el-link @click="$router.push('/config/tools')" type="primary"> 全局配置 </el-link>添加
          </div>
          <el-checkbox-group v-else v-model="form.toolIds">
            <el-checkbox v-for="t in toolList" :key="t.id" :label="t.id" :value="t.id">
              {{ t.name }}
            </el-checkbox>
          </el-checkbox-group>
        </div>

        <!-- Skills -->
        <div class="form-section">
          <div class="section-title">Skills</div>
          <div v-if="skillList.length === 0" class="empty-hint">暂无已安装的 Skills</div>
          <el-checkbox-group v-else v-model="form.skillIds">
            <el-checkbox v-for="s in skillList" :key="s.id" :label="s.id" :value="s.id">
              {{ s.name }} <el-text type="info" size="small" style="margin-left:4px">v{{ s.version }}</el-text>
            </el-checkbox>
          </el-checkbox-group>
        </div>
      </el-form>

      <!-- 底部操作 -->
      <div class="create-footer">
        <el-button @click="$router.push('/agents')">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">
          保存 Agent
        </el-button>
      </div>
    </div>

    <!-- ═══ 右栏：AI 对话 ═══ -->
    <div class="create-right">
      <!-- Agent Tab 切换器 -->
      <div class="agent-tabs-bar">
        <div class="agent-tabs-scroll">
          <!-- 固定：配置助手 -->
          <div class="agent-tab" :class="{ active: activeAgentTab === '__assist__' }"
            @click="switchTab('__assist__')">
            <span class="tab-icon">🤖</span> 配置助手
          </div>
          <!-- 其他已有 agent -->
          <div v-for="ag in agentList" :key="ag.id"
            class="agent-tab" :class="{ active: activeAgentTab === ag.id }"
            @click="switchTab(ag.id)">
            <div class="tab-avatar" :style="{ background: ag.avatarColor || '#409eff' }">
              {{ ag.name.charAt(0) }}
            </div>
            {{ ag.name }}
            <el-icon class="tab-close" @click.stop="closeTab(ag.id)"><Close /></el-icon>
          </div>
          <!-- 添加更多 -->
          <el-dropdown @command="openTab" trigger="click">
            <div class="agent-tab add-tab">
              <el-icon><Plus /></el-icon> 更多
            </div>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item v-for="ag in allAgents" :key="ag.id" :command="ag.id">
                  {{ ag.name }}
                </el-dropdown-item>
                <el-dropdown-item v-if="allAgents.length === 0" disabled>
                  暂无其他 Agent
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </div>

      <!-- 配置助手提示（仅在该Tab显示）-->
      <div v-if="activeAgentTab === '__assist__'" class="assist-hint">
        <el-icon><ChatDotRound /></el-icon>
        告诉我这个 Agent 要做什么，我来帮你生成配置
      </div>

      <!-- 聊天区域 -->
      <div class="chat-messages" ref="chatMsgRef">
        <template v-if="currentMessages.length === 0">
          <div class="chat-empty">
            <template v-if="activeAgentTab === '__assist__'">
              <p>✨ 例如：</p>
              <div class="example-chip" @click="fillExample('我需要一个电商客服 Agent，负责解答订单问题，语气亲切友好')">
                我需要一个电商客服 Agent...
              </div>
              <div class="example-chip" @click="fillExample('帮我创建一个代码审查助手，专注于 Python 代码规范')">
                帮我创建一个代码审查助手...
              </div>
              <div class="example-chip" @click="fillExample('创建一个每天早上发送天气报告的 Agent')">
                创建一个天气报告 Agent...
              </div>
            </template>
            <template v-else>
              <p>与 <strong>{{ currentAgentName }}</strong> 对话</p>
            </template>
          </div>
        </template>

        <div v-for="(msg, i) in currentMessages" :key="i"
          :class="['chat-msg', msg.role]">
          <div class="msg-bubble">
            <div v-if="msg.role === 'assistant' && msg.applyData" class="apply-card">
              <div class="apply-fields">
                <div v-for="(val, key) in msg.applyData" :key="key" class="apply-field">
                  <span class="field-name">{{ fieldLabel(key) }}</span>
                  <span class="field-preview">{{ val.slice(0, 60) }}{{ val.length > 60 ? '...' : '' }}</span>
                </div>
              </div>
              <el-button type="primary" size="small" @click="applyToForm(msg.applyData)">
                应用到表单 ↙
              </el-button>
            </div>
            <div v-else class="msg-text" v-html="renderText(msg.text)" />
          </div>
        </div>

        <div v-if="chatStreaming" class="chat-msg assistant">
          <div class="msg-bubble">
            <div class="msg-text">
              {{ streamText }}<span class="cursor-blink">▊</span>
            </div>
          </div>
        </div>
      </div>

      <!-- 输入框 -->
      <div class="chat-input-area">
        <el-input v-model="chatInput" type="textarea" :rows="2"
          placeholder="输入需求，或问任何问题... (Ctrl+Enter 发送)"
          @keydown.enter.ctrl.prevent="sendChat"
          :disabled="chatStreaming" />
        <el-button type="primary" :loading="chatStreaming" @click="sendChat" class="send-btn">
          发送
        </el-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, nextTick, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { ArrowLeft, Plus, Close, ChatDotRound } from '@element-plus/icons-vue'
import { agents as agentsApi, models, channels, tools, skills, chatSSE, type AgentInfo, type ModelEntry, type ChannelEntry, type ToolEntry, type SkillEntry } from '../api'

const router = useRouter()

// ── Form state ───────────────────────────────────────────────────────────
const form = reactive({
  name: '',
  id: '',
  description: '',
  avatarColor: '#409eff',
  identity: '',
  soul: '',
  modelId: '',
  channelIds: [] as string[],
  toolIds: [] as string[],
  skillIds: [] as string[],
})

// Track which fields were AI-filled (show badge + revert btn)
const aiFilledFields = reactive(new Set<string>())
const aiFilledSnapshot: Record<string, string> = {}

const saving = ref(false)

const avatarColors = ['#409eff', '#67c23a', '#e6a23c', '#f56c6c', '#909399', '#9b59b6', '#1abc9c', '#e74c3c']

function autoId() {
  form.id = form.name.toLowerCase()
    .replace(/[^a-z0-9\u4e00-\u9fff\s-]/g, '')
    .trim()
    .replace(/[\s\u4e00-\u9fff]+/g, '-')
    .replace(/-+/g, '-')
    .slice(0, 32)
}

function revertField(field: string) {
  const key = field as keyof typeof form
  ;(form as any)[key] = aiFilledSnapshot[field] || ''
  aiFilledFields.delete(field)
}

function applyToForm(data: Record<string, string>) {
  const fieldMap: Record<string, keyof typeof form> = {
    name: 'name', id: 'id', description: 'description',
    identity: 'identity', soul: 'soul',
  }
  for (const [key, val] of Object.entries(data)) {
    const formKey = fieldMap[key]
    if (formKey) {
      aiFilledSnapshot[key] = (form as any)[formKey]
      ;(form as any)[formKey] = val
      aiFilledFields.add(key)
      if (key === 'name') autoId()
    }
  }
  ElMessage.success('已应用到左侧表单')
}

async function save() {
  if (!form.name.trim()) { ElMessage.warning('请填写名称'); return }
  if (!form.id.trim()) { ElMessage.warning('请填写 ID'); return }
  saving.value = true
  try {
    await agentsApi.create({
      ...form,
      model: form.modelId || '',
    })
    ElMessage.success('Agent 创建成功！')
    router.push(`/agents/${form.id}`)
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '创建失败')
  } finally {
    saving.value = false
  }
}

// ── Config lists ─────────────────────────────────────────────────────────
const modelList = ref<ModelEntry[]>([])
const channelList = ref<ChannelEntry[]>([])
const toolList = ref<ToolEntry[]>([])
const skillList = ref<SkillEntry[]>([])
const allAgentsFull = ref<AgentInfo[]>([])

// ── Right panel: Agent tabs ──────────────────────────────────────────────
const activeAgentTab = ref('__assist__')
const openedAgentIds = ref<string[]>([])  // agents opened as tabs

const agentList = computed(() =>
  allAgentsFull.value.filter(a => openedAgentIds.value.includes(a.id))
)
const allAgents = computed(() =>
  allAgentsFull.value.filter(a => !openedAgentIds.value.includes(a.id))
)
const currentAgentName = computed(() => {
  if (activeAgentTab.value === '__assist__') return '配置助手'
  return allAgentsFull.value.find(a => a.id === activeAgentTab.value)?.name || activeAgentTab.value
})

function switchTab(id: string) {
  activeAgentTab.value = id
  nextTick(() => scrollToBottom())
}

function openTab(id: string) {
  if (!openedAgentIds.value.includes(id)) openedAgentIds.value.push(id)
  switchTab(id)
}

function closeTab(id: string) {
  openedAgentIds.value = openedAgentIds.value.filter(x => x !== id)
  if (activeAgentTab.value === id) switchTab('__assist__')
}

// ── Chat ─────────────────────────────────────────────────────────────────
interface ChatMsg {
  role: 'user' | 'assistant'
  text: string
  applyData?: Record<string, string>
}

const allMessages = reactive<Record<string, ChatMsg[]>>({ '__assist__': [] })
const currentMessages = computed(() => allMessages[activeAgentTab.value] || [])

const chatInput = ref('')
const chatStreaming = ref(false)
const streamText = ref('')
const chatMsgRef = ref<HTMLElement>()

function scrollToBottom() {
  nextTick(() => {
    if (chatMsgRef.value) {
      chatMsgRef.value.scrollTop = chatMsgRef.value.scrollHeight
    }
  })
}

function fillExample(text: string) {
  chatInput.value = text
}

function renderText(text: string) {
  return text.replace(/\n/g, '<br>')
}

function fieldLabel(key: string) {
  const map: Record<string, string> = {
    name: '名称', id: 'ID', description: '描述',
    identity: 'IDENTITY', soul: 'SOUL',
  }
  return map[key] || key
}

async function sendChat() {
  const msg = chatInput.value.trim()
  if (!msg || chatStreaming.value) return

  if (!allMessages[activeAgentTab.value]) {
    allMessages[activeAgentTab.value] = []
  }
  ;(allMessages[activeAgentTab.value] as ChatMsg[]).push({ role: 'user', text: msg })
  chatInput.value = ''
  chatStreaming.value = true
  streamText.value = ''
  scrollToBottom()

  if (activeAgentTab.value === '__assist__') {
    // AI 配置助手：调用配置助手 API
    await runAssist(msg)
  } else {
    // 普通 Agent 对话
    await runAgentChat(activeAgentTab.value, msg)
  }
}

async function runAssist(userMsg: string) {
  // 用配置助手 Agent 生成配置，解析 JSON 字段
  // 临时：把当前表单状态拼入 system context
  const context = `当前表单状态：名称="${form.name || '（未填）'}", 描述="${form.description || '（未填）'}"。
用户要求：${userMsg}

请生成 Agent 配置。如果有足够信息，在回答末尾附上JSON块（字段：name/description/identity/soul），格式：
\`\`\`json
{"name":"...","description":"...","identity":"...","soul":"..."}
\`\`\``

  let fullText = ''
  let applyData: Record<string, string> | undefined

  try {
    // 用 main agent 作为配置助手（有 LLM 能力）
    const assistAgentId = allAgentsFull.value[0]?.id || 'main'
    await new Promise<void>((resolve) => {
      chatSSE(assistAgentId, context, (ev) => {
        if (ev.type === 'text') {
          streamText.value += ev.text
          fullText += ev.text
          scrollToBottom()
        } else if (ev.type === 'done' || ev.type === 'error') {
          resolve()
        }
      })
    })

    // 解析 JSON 块
    const jsonMatch = fullText.match(/```json\s*([\s\S]+?)\s*```/)
    if (jsonMatch) {
      try {
        applyData = JSON.parse(jsonMatch[1] as string)
        fullText = fullText.replace(/```json[\s\S]+?```/, '').trim()
      } catch {}
    }
  } catch (e) {
    fullText = '抱歉，生成配置时出错了。请检查是否已配置模型 API Key。'
  }

  ;(allMessages['__assist__'] as ChatMsg[]).push({
    role: 'assistant',
    text: fullText || streamText.value,
    applyData,
  })
  streamText.value = ''
  chatStreaming.value = false
  scrollToBottom()
}

async function runAgentChat(agentId: string, msg: string) {
  let fullText = ''

  chatSSE(agentId, msg, (ev) => {
    if (ev.type === 'text') {
      streamText.value += ev.text
      fullText += ev.text
      scrollToBottom()
    } else if (ev.type === 'done' || ev.type === 'error') {
      if (!allMessages[agentId]) allMessages[agentId] = []
      ;(allMessages[agentId] as ChatMsg[]).push({
        role: 'assistant',
        text: fullText || streamText.value || (ev.error ? `❌ ${ev.error}` : ''),
      })
      streamText.value = ''
      chatStreaming.value = false
      scrollToBottom()
    }
  })
}

// ── Init ─────────────────────────────────────────────────────────────────
onMounted(async () => {
  const [ml, cl, tl, sl, al] = await Promise.allSettled([
    models.list(), channels.list(), tools.list(), skills.list(), agentsApi.list()
  ])
  if (ml.status === 'fulfilled') modelList.value = ml.value.data
  if (cl.status === 'fulfilled') channelList.value = cl.value.data
  if (tl.status === 'fulfilled') toolList.value = tl.value.data
  if (sl.status === 'fulfilled') skillList.value = sl.value.data
  if (al.status === 'fulfilled') allAgentsFull.value = al.value.data
  if (modelList.value.length > 0) form.modelId = modelList.value[0]?.id ?? ''
})
</script>

<style scoped>
.create-layout {
  display: flex;
  height: 100vh;
  overflow: hidden;
}

/* ─── 左栏 ─── */
.create-left {
  width: 52%;
  display: flex;
  flex-direction: column;
  border-right: 1px solid #e4e7ed;
  background: #fff;
}

.create-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px 24px;
  border-bottom: 1px solid #f0f0f0;
  flex-shrink: 0;
}

.back-btn { color: #606266; }

.create-form {
  flex: 1;
  overflow-y: auto;
  padding: 20px 24px;
}

.form-section {
  background: #fafafa;
  border-radius: 8px;
  padding: 16px 20px;
  margin-bottom: 16px;
}

.section-title {
  font-weight: 600;
  font-size: 14px;
  color: #303133;
  margin-bottom: 14px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.ai-badge {
  font-size: 11px;
  background: #ecf5ff;
  color: #409eff;
  padding: 1px 6px;
  border-radius: 4px;
  font-weight: 400;
}

.field-hint { color: #909399; font-weight: 400; font-size: 12px; margin-left: 4px; }

.revert-btn {
  margin-left: auto;
  color: #909399 !important;
  font-size: 12px;
  padding: 0 4px;
}

.ai-filled :deep(.el-textarea__inner),
.ai-filled :deep(.el-input__inner) {
  background: #f0f9ff;
  border-color: #b3d8ff;
}

.color-row { display: flex; gap: 8px; flex-wrap: wrap; }

.color-swatch {
  width: 28px; height: 28px;
  border-radius: 50%;
  cursor: pointer;
  border: 3px solid transparent;
  transition: transform 0.15s;
}
.color-swatch:hover { transform: scale(1.15); }
.color-swatch.active { border-color: #303133; box-shadow: 0 0 0 2px #fff inset; }

.empty-hint {
  color: #909399;
  font-size: 13px;
  padding: 4px 0;
}

.create-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 14px 24px;
  border-top: 1px solid #f0f0f0;
  background: #fff;
  flex-shrink: 0;
}

/* ─── 右栏 ─── */
.create-right {
  flex: 1;
  display: flex;
  flex-direction: column;
  background: #f5f7fa;
  overflow: hidden;
}

/* Agent Tab Bar */
.agent-tabs-bar {
  background: #fff;
  border-bottom: 1px solid #e4e7ed;
  flex-shrink: 0;
  overflow: hidden;
}

.agent-tabs-scroll {
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 6px 12px;
  overflow-x: auto;
  scrollbar-width: none;
}
.agent-tabs-scroll::-webkit-scrollbar { display: none; }

.agent-tab {
  display: flex;
  align-items: center;
  gap: 5px;
  padding: 5px 12px;
  border-radius: 6px;
  cursor: pointer;
  white-space: nowrap;
  font-size: 13px;
  color: #606266;
  transition: all 0.15s;
  flex-shrink: 0;
}
.agent-tab:hover { background: #f0f2f5; color: #303133; }
.agent-tab.active { background: #ecf5ff; color: #409eff; font-weight: 500; }
.agent-tab.add-tab { color: #909399; }

.tab-icon { font-size: 15px; }

.tab-avatar {
  width: 18px; height: 18px;
  border-radius: 50%;
  color: #fff;
  font-size: 11px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.tab-close {
  font-size: 12px;
  color: #c0c4cc;
  margin-left: 2px;
  border-radius: 3px;
}
.tab-close:hover { color: #f56c6c; background: #fef0f0; }

/* 助手提示栏 */
.assist-hint {
  background: #ecf5ff;
  color: #409eff;
  font-size: 13px;
  padding: 8px 16px;
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

/* 消息区 */
.chat-messages {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.chat-empty {
  margin: auto;
  text-align: center;
  color: #909399;
  font-size: 14px;
}

.example-chip {
  display: inline-block;
  background: #fff;
  border: 1px solid #dcdfe6;
  border-radius: 16px;
  padding: 6px 14px;
  font-size: 13px;
  cursor: pointer;
  margin: 4px;
  color: #409eff;
  transition: all 0.15s;
}
.example-chip:hover { background: #ecf5ff; border-color: #409eff; }

.chat-msg { display: flex; }
.chat-msg.user { justify-content: flex-end; }
.chat-msg.assistant { justify-content: flex-start; }

.msg-bubble {
  max-width: 80%;
  padding: 10px 14px;
  border-radius: 12px;
  font-size: 14px;
  line-height: 1.6;
}

.chat-msg.user .msg-bubble {
  background: #409eff;
  color: #fff;
  border-bottom-right-radius: 4px;
}

.chat-msg.assistant .msg-bubble {
  background: #fff;
  color: #303133;
  border-bottom-left-radius: 4px;
  box-shadow: 0 1px 4px rgba(0,0,0,.08);
}

/* AI 应用卡片 */
.apply-card {
  border-top: 1px solid #f0f0f0;
  padding-top: 10px;
  margin-top: 8px;
}

.apply-fields { margin-bottom: 10px; }

.apply-field {
  display: flex;
  gap: 8px;
  font-size: 12px;
  padding: 3px 0;
}
.field-name { color: #909399; flex-shrink: 0; min-width: 60px; }
.field-preview { color: #303133; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

@keyframes blink { 50% { opacity: 0; } }
.cursor-blink { animation: blink 0.8s infinite; }

/* 输入区 */
.chat-input-area {
  padding: 12px;
  background: #fff;
  border-top: 1px solid #e4e7ed;
  flex-shrink: 0;
  display: flex;
  gap: 8px;
  align-items: flex-end;
}

.send-btn { height: auto; padding: 8px 18px; align-self: flex-end; }
</style>
