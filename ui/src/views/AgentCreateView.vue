<template>
  <div class="create-layout">
    <!-- ═══ 左栏：配置表单 ═══ -->
    <div class="create-left">
      <div class="create-header">
        <el-button text @click="$router.push('/agents')" class="back-btn">
          <el-icon><ArrowLeft /></el-icon> 返回
        </el-button>
        <h2 style="margin: 0">新建 AI 成员</h2>
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

        <!-- 消息通道（每个成员独立配置） -->
        <div class="form-section">
          <div class="section-title" style="display:flex;align-items:center;justify-content:space-between;">
            <span>消息通道</span>
            <el-button size="small" type="primary" plain @click="openAddChannelInline">
              <el-icon><Plus /></el-icon> 添加
            </el-button>
          </div>

          <!-- 已添加的通道列表 -->
          <div v-if="form.agentChannels.length" style="display:flex;flex-direction:column;gap:6px;margin-bottom:8px;">
            <div
              v-for="(ch, idx) in form.agentChannels"
              :key="idx"
              style="display:flex;align-items:center;justify-content:space-between;padding:6px 10px;background:#f5f7fa;border-radius:6px;font-size:13px;"
            >
              <div style="display:flex;align-items:center;gap:8px;">
                <el-tag size="small">{{ ch.type === 'telegram' ? 'Telegram' : 'Web' }}</el-tag>
                <span style="font-weight:500;">{{ ch.name || '未命名' }}</span>
                <span v-if="ch.config?.botName" style="color:#909399;">@{{ ch.config.botName }}</span>
              </div>
              <el-button text size="small" type="danger" @click="removeChannel(idx)">
                <el-icon><Close /></el-icon>
              </el-button>
            </div>
          </div>
          <div v-else class="empty-hint">暂未添加通道（可在创建后配置）</div>

          <!-- 内联添加表单 -->
          <div v-if="showAddChannel" class="add-channel-form">
            <el-form :model="newChannelForm" label-width="90px" size="small">
              <el-form-item label="类型">
                <el-select v-model="newChannelForm.type" style="width:100%">
                  <el-option label="Telegram" value="telegram" />
                  <el-option label="Web 聊天页" value="web" />
                </el-select>
              </el-form-item>
              <el-form-item label="名称">
                <el-input v-model="newChannelForm.name" placeholder="如：客服 Bot" />
              </el-form-item>
              <template v-if="newChannelForm.type === 'telegram'">
                <el-form-item label="Bot Token">
                  <div style="display:flex;gap:6px;width:100%;">
                    <el-input
                      v-model="newChannelForm.botToken"
                      type="password"
                      show-password
                      placeholder="从 @BotFather 获取"
                      style="flex:1"
                      :status="newTokenCheck.status === 'error' ? 'error' : newTokenCheck.status === 'ok' ? 'success' : ''"
                    />
                    <el-button size="small" :loading="newTokenCheck.loading" @click="checkNewToken">验证</el-button>
                  </div>
                  <div v-if="newTokenCheck.status === 'ok'" style="color:#67c23a;font-size:12px;margin-top:4px;">
                    ✓ Token 有效，Bot：@{{ newTokenCheck.botName }}
                  </div>
                  <div v-else-if="newTokenCheck.status === 'error'" style="color:#f56c6c;font-size:12px;margin-top:4px;">
                    ✗ {{ newTokenCheck.error }}
                  </div>
                </el-form-item>
                <el-form-item label="白名单 ID">
                  <el-input v-model="newChannelForm.allowedFrom" placeholder="Telegram 用户 ID，多个用逗号分隔（留空=配对模式）" />
                </el-form-item>
              </template>
              <template v-if="newChannelForm.type === 'web'">
                <el-form-item label="访问密码">
                  <el-input v-model="newChannelForm.webPassword" type="password" show-password placeholder="留空则无需密码" />
                </el-form-item>
                <el-form-item label="欢迎语">
                  <el-input v-model="newChannelForm.webWelcome" placeholder="你好！有什么可以帮你的？" />
                </el-form-item>
              </template>
            </el-form>
            <div style="display:flex;gap:8px;justify-content:flex-end;margin-top:8px;">
              <el-button size="small" @click="showAddChannel = false">取消</el-button>
              <el-button size="small" type="primary" @click="confirmAddChannel">确认添加</el-button>
            </div>
          </div>
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
          <div class="agent-tab" :class="{ active: activeAgentTab === '__assist__' }"
            @click="switchTab('__assist__')">
            <el-icon class="tab-icon"><User /></el-icon> 配置助手
          </div>
          <div v-for="ag in agentList" :key="ag.id"
            class="agent-tab" :class="{ active: activeAgentTab === ag.id }"
            @click="switchTab(ag.id)">
            <div class="tab-avatar" :style="{ background: ag.avatarColor || '#409eff' }">
              {{ ag.name.charAt(0) }}
            </div>
            {{ ag.name }}
            <el-icon class="tab-close" @click.stop="closeTab(ag.id)"><Close /></el-icon>
          </div>
          <el-dropdown @command="openTab" trigger="click">
            <div class="agent-tab add-tab"><el-icon><Plus /></el-icon> 更多</div>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item v-for="ag in allAgents" :key="ag.id" :command="ag.id">
                  {{ ag.name }}
                </el-dropdown-item>
                <el-dropdown-item v-if="allAgents.length === 0" disabled>暂无其他 Agent</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </div>

      <!-- 配置助手 AiChat（用 v-show 保留对话历史） -->
      <template v-if="activeAgentTab === '__assist__'">
        <AiChat
          v-if="assistAgentId"
          :agent-id="assistAgentId"
          :context="assistContext"
          scenario="agent-creation"
          placeholder="告诉我这个 Agent 要做什么..."
          :examples="[
            '我需要一个电商客服 Agent，负责解答订单问题，语气亲切',
            '帮我创建一个代码审查助手，专注于 Python 代码规范',
            '创建一个每天早上发送天气报告的 Agent',
          ]"
          height="100%"
          :compact="true"
          :show-thinking="true"
          :applyable="true"
          @apply="applyToForm"
        />
        <div v-else class="no-agent-hint">
          <el-icon size="32"><InfoFilled /></el-icon>
          <p>配置助手需要至少一个 AI 成员才能运行</p>
          <p class="hint-sub">请先在左侧填写基本信息并创建第一个成员</p>
        </div>
      </template>

      <!-- 其他已打开的 Agent（每个保留独立对话历史） -->
      <template v-for="ag in agentList" :key="ag.id">
        <AiChat
          v-show="activeAgentTab === ag.id"
          :agent-id="ag.id"
          scenario="general"
          :welcome-message="`你好，我是 **${ag.name}**，有什么需要帮忙的？`"
          height="100%"
          :compact="true"
          :show-thinking="true"
        />
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { ArrowLeft, Plus, Close, InfoFilled } from '@element-plus/icons-vue'
import { agents as agentsApi, files as filesApi, models, channels, tools, skills, memoryConfigApi, agentChannels as agentChannelsApi, type AgentInfo, type ModelEntry, type ChannelEntry, type ToolEntry, type SkillEntry } from '../api'
import AiChat from '../components/AiChat.vue'

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
  agentChannels: [] as Array<{
    id: string; type: string; name: string; enabled: boolean;
    config: Record<string, string>;
  }>,
})

// ── Per-agent channel add form ────────────────────────────────────────────
const showAddChannel = ref(false)
const newChannelForm = reactive({
  type: 'telegram',
  name: '',
  botToken: '',
  allowedFrom: '',
  webPassword: '',
  webWelcome: '',
})
const newTokenCheck = reactive({ loading: false, status: '' as ''|'ok'|'error'|'duplicate', botName: '', error: '' })

function openAddChannelInline() {
  Object.assign(newChannelForm, { type: 'telegram', name: '', botToken: '', allowedFrom: '', webPassword: '', webWelcome: '' })
  Object.assign(newTokenCheck, { loading: false, status: '', botName: '', error: '' })
  showAddChannel.value = true
}

async function checkNewToken() {
  if (!newChannelForm.botToken) return
  newTokenCheck.loading = true
  newTokenCheck.status = ''
  try {
    // Use a temp check via the __config__ agent (any agent will do for token validation)
    const tmpId = form.id || '__config__'
    const res = await agentChannelsApi.checkToken(tmpId, newChannelForm.botToken)
    const d = res.data
    if (d.duplicate) {
      Object.assign(newTokenCheck, { loading: false, status: 'duplicate', botName: '', error: `已被「${d.usedBy}」使用` })
    } else if (d.valid) {
      Object.assign(newTokenCheck, { loading: false, status: 'ok', botName: d.botName || '', error: '' })
      if (!newChannelForm.name && d.botName) newChannelForm.name = d.botName
    } else {
      Object.assign(newTokenCheck, { loading: false, status: 'error', botName: '', error: d.error || 'Token 无效' })
    }
  } catch {
    Object.assign(newTokenCheck, { loading: false, status: 'error', botName: '', error: '验证失败' })
  }
}

function confirmAddChannel() {
  const t = newChannelForm.type
  const cfg: Record<string, string> = {}
  if (t === 'telegram') {
    if (!newChannelForm.botToken) { ElMessage.warning('请填写 Bot Token'); return }
    cfg.botToken = newChannelForm.botToken
    if (newTokenCheck.botName) cfg.botName = newTokenCheck.botName
    if (newChannelForm.allowedFrom) cfg.allowedFrom = newChannelForm.allowedFrom
  } else if (t === 'web') {
    if (newChannelForm.webPassword) cfg.password = newChannelForm.webPassword
    if (newChannelForm.webWelcome) cfg.welcome = newChannelForm.webWelcome
  }
  const chId = `${t}-${Date.now()}`
  form.agentChannels.push({ id: chId, type: t, name: newChannelForm.name || chId, enabled: true, config: cfg })
  showAddChannel.value = false
  ElMessage.success('已添加，保存 Agent 后生效')
}

function removeChannel(idx: number) {
  form.agentChannels.splice(idx, 1)
}

// Track which fields were AI-filled (show badge + revert btn)
const aiFilledFields = reactive(new Set<string>())
const aiFilledSnapshot: Record<string, string> = {}

const saving = ref(false)

const avatarColors = ['#409eff', '#67c23a', '#e6a23c', '#f56c6c', '#909399', '#9b59b6', '#1abc9c', '#e74c3c']

function autoId() {
  const raw = form.name.toLowerCase()
    .replace(/[^a-z0-9\u4e00-\u9fff\s-]/g, '')
    .trim()

  // 1. 先尝试保留 ASCII 字母数字
  let slug = raw.replace(/[\u4e00-\u9fff]/g, '').replace(/[\s_]+/g, '-').replace(/-+/g, '-').replace(/^-+|-+$/g, '')

  // 2. ASCII 不足时：用中文字符数量生成语义 ID（取每个字对应的拼音首字母近似）
  if (slug.length < 2) {
    // 用名字长度 + 时间戳尾缀做 fallback
    const ts = Date.now().toString(36).slice(-4)
    // 中文名取字符数作为前缀描述
    const zhLen = (raw.match(/[\u4e00-\u9fff]/g) || []).length
    slug = zhLen > 0 ? `agent-${zhLen}ch-${ts}` : `agent-${ts}`
  }

  form.id = slug.slice(0, 32)
}

function revertField(field: string) {
  const key = field as keyof typeof form
  ;(form as any)[key] = aiFilledSnapshot[field] || ''
  aiFilledFields.delete(field)
}

function applyToForm(data: Record<string, string>) {
  // support both lowercase (name/identity/soul) and uppercase (IDENTITY/SOUL)
  const fieldMap: Record<string, keyof typeof form> = {
    name: 'name', NAME: 'name',
    id: 'id', ID: 'id',
    description: 'description', DESCRIPTION: 'description', desc: 'description',
    identity: 'identity', IDENTITY: 'identity',
    soul: 'soul', SOUL: 'soul',
  }
  let applied = 0
  for (const [key, val] of Object.entries(data)) {
    const formKey = fieldMap[key]
    if (formKey && val) {
      aiFilledSnapshot[key.toLowerCase()] = (form as any)[formKey]
      ;(form as any)[formKey] = val
      aiFilledFields.add(key.toLowerCase())
      if (key.toLowerCase() === 'name') autoId()
      applied++
    }
  }
  if (applied > 0) {
    ElMessage.success(`已填入 ${applied} 个字段到左侧表单 ✓`)
  } else {
    ElMessage.warning('未识别到可填入的字段，请手动复制')
  }
}

async function save() {
  if (!form.name.trim()) { ElMessage.warning('请填写名称'); return }
  if (!form.id.trim() || form.id === '-' || !/^[a-z0-9][a-z0-9-_]{0,30}$/.test(form.id)) {
    ElMessage.warning('ID 格式不对，请手动填写（只能用小写字母、数字、连字符）')
    return
  }
  if (saving.value) return   // 防重复提交
  saving.value = true
  try {
    // 1. 创建 Agent 基本信息
    await agentsApi.create({
      ...form,
      model: form.modelId || '',
    })

    // 2. 写入 IDENTITY.md / SOUL.md（如果有内容）
    const writes: Promise<any>[] = []
    if (form.identity.trim()) {
      writes.push(filesApi.write(form.id, 'IDENTITY.md', form.identity))
    }
    if (form.soul.trim()) {
      writes.push(filesApi.write(form.id, 'SOUL.md', form.soul))
    }
    if (writes.length) await Promise.all(writes)

    // 3. 保存 per-agent 消息通道
    if (form.agentChannels.length) {
      try {
        await agentChannelsApi.set(form.id, form.agentChannels as any)
      } catch { /* 非致命错误，忽略 */ }
    }

    // 4. 默认开启自动记忆
    try {
      await memoryConfigApi.setConfig(form.id, {
        enabled: true,
        schedule: 'daily',
        keepTurns: 3,
        focusHint: '',
        cronJobId: '',
      })
    } catch { /* 非致命错误，忽略 */ }

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

// 配置助手固定使用系统内置 agent
const assistAgentId = '__config__'

// 实时将左侧表单状态注入对话上下文
const assistContext = computed(() => {
  const parts = [
    '你是一个 AI 配置助手，帮助用户设计和生成 AI Agent 的配置文件（IDENTITY 和 SOUL）。',
    '用户正在新建一个 Agent，当前表单状态如下（未填字段为空）：',
    `- 名称: ${form.name || '（未填）'}`,
    `- ID: ${form.id || '（未填）'}`,
    `- 描述: ${form.description || '（未填）'}`,
    form.identity ? `- IDENTITY（已填）: ${form.identity.slice(0, 100)}...` : '- IDENTITY: （未填）',
    form.soul ? `- SOUL（已填）: ${form.soul.slice(0, 100)}...` : '- SOUL: （未填）',
    '',
    '当你为用户生成配置时，请在回答末尾附上如下格式的 JSON 块，方便用户一键应用：',
    '```json',
    '{"name":"...","description":"...","identity":"...","soul":"..."}',
    '```',
    '如果某个字段不需要更改，就省略它。',
  ]
  return parts.join('\n')
})

function switchTab(id: string) {
  activeAgentTab.value = id
}

function openTab(id: string) {
  if (!openedAgentIds.value.includes(id)) openedAgentIds.value.push(id)
  switchTab(id)
}

function closeTab(id: string) {
  openedAgentIds.value = openedAgentIds.value.filter(x => x !== id)
  if (activeAgentTab.value === id) switchTab('__assist__')
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

.add-channel-form {
  margin-top: 8px;
  padding: 12px 16px;
  background: #f5f7fa;
  border-radius: 8px;
  border: 1px solid #e4e7ed;
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

/* 无 Agent 提示 */
.no-agent-hint {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  color: #909399;
  font-size: 14px;
}
.no-agent-hint p { margin: 0; }
.no-agent-hint .hint-sub { font-size: 12px; color: #c0c4cc; }
</style>
