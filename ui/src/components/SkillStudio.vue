<template>
  <div class="skill-studio">
    <!-- ── 左：技能列表 ── -->
    <div class="studio-sidebar">
      <div class="sidebar-top">
        <span class="sidebar-title">技能库</span>
        <div class="sidebar-acts">
          <el-button size="small" :loading="listLoading" circle @click="loadList">
            <el-icon><Refresh /></el-icon>
          </el-button>
          <el-button size="small" type="primary" circle :loading="creating" @click="openNew">
            <el-icon><Plus /></el-icon>
          </el-button>
        </div>
      </div>

      <div class="skill-list">
        <div v-if="!listLoading && skills.length === 0" class="list-empty">暂无技能</div>
        <div
          v-for="sk in skills" :key="sk.id"
          :class="['skill-item', { active: selected?.id === sk.id }]"
          @click="selectSkill(sk)"
        >
          <span class="sk-icon">
            <span v-if="sk.icon">{{ sk.icon }}</span>
            <el-icon v-else><Tools /></el-icon>
          </span>
          <div class="sk-info">
            <div class="sk-name">{{ sk.name }}</div>
            <div class="sk-id">{{ sk.id }}</div>
          </div>
          <div class="sk-right">
            <el-tag v-if="sk.category" size="small" effect="plain" style="margin-right:6px;font-size:11px">{{ sk.category }}</el-tag>
            <el-switch
              :model-value="sk.enabled"
              size="small"
              @change="(v: boolean) => toggleSkill(sk, v)"
              @click.stop
            />
          </div>
        </div>
      </div>
    </div>

    <!-- ── 中：编辑器 ── -->
    <div class="studio-editor">
      <!-- 空态 -->
      <div v-if="!selected" class="editor-empty">
        <el-icon size="48" color="#c0c4cc"><Setting /></el-icon>
        <p>从左侧选择一个技能开始编辑</p>
        <el-button type="primary" @click="openNew"><el-icon><Plus /></el-icon> 新建技能</el-button>
      </div>

      <template v-else>
        <!-- 顶部工具栏 -->
        <div class="editor-toolbar">
          <div class="editor-breadcrumb">
            <el-icon style="color:#909399"><FolderOpened /></el-icon>
            <span class="crumb-sep">skills /</span>
            <span class="crumb-name">{{ selected.id }}</span>
          </div>
          <div class="toolbar-acts">
            <el-button size="small" @click="sendTestToChat">
              <el-icon><VideoPlay /></el-icon> 测试
            </el-button>
            <el-button size="small" type="primary" :loading="saving" @click="saveSkill">
              <el-icon><DocumentChecked /></el-icon> 保存
            </el-button>
            <el-popconfirm title="确认删除该技能？" @confirm="deleteSkill">
              <template #reference>
                <el-button size="small" type="danger" plain><el-icon><Delete /></el-icon></el-button>
              </template>
            </el-popconfirm>
          </div>
        </div>

        <!-- 文件树 + 编辑区 -->
        <div class="editor-body">
          <!-- 文件树 -->
          <div class="file-tree">
            <div class="tree-title">目录</div>
            <div class="tree-item tree-dir">
              <el-icon><Folder /></el-icon>
              <span>{{ selected.id }}/</span>
            </div>
            <div
              :class="['tree-item', { 'tree-active': activeFile === 'meta' }]"
              @click="activeFile = 'meta'"
            >
              <el-icon><Document /></el-icon>
              <span>skill.json</span>
            </div>
            <div
              :class="['tree-item', { 'tree-active': activeFile === 'prompt' }]"
              @click="switchToPrompt"
            >
              <el-icon><Document /></el-icon>
              <span>SKILL.md</span>
              <el-tag v-if="selected.enabled" size="small" type="success" effect="plain" style="margin-left:4px;font-size:10px">注入中</el-tag>
            </div>
          </div>

          <!-- skill.json 元数据编辑 -->
          <div v-if="activeFile === 'meta'" class="file-editor">
            <div class="file-editor-head">
              <el-icon><Document /></el-icon> skill.json
              <span class="file-hint">技能元信息，影响列表展示和 runner 行为</span>
            </div>
            <el-form :model="metaForm" label-width="72px" size="small" style="padding:16px 20px">
              <el-form-item label="技能 ID">
                <el-input :value="selected.id" disabled />
              </el-form-item>
              <el-row :gutter="12">
                <el-col :span="14">
                  <el-form-item label="名称">
                    <el-input v-model="metaForm.name" placeholder="如 翻译助手" />
                  </el-form-item>
                </el-col>
                <el-col :span="10">
                  <el-form-item label="图标">
                    <el-input v-model="metaForm.icon" placeholder="emoji" />
                  </el-form-item>
                </el-col>
              </el-row>
              <el-row :gutter="12">
                <el-col :span="14">
                  <el-form-item label="分类">
                    <el-input v-model="metaForm.category" placeholder="如 语言" />
                  </el-form-item>
                </el-col>
                <el-col :span="10">
                  <el-form-item label="版本">
                    <el-input v-model="metaForm.version" placeholder="1.0.0" />
                  </el-form-item>
                </el-col>
              </el-row>
              <el-form-item label="描述">
                <el-input v-model="metaForm.description" type="textarea" :rows="2" placeholder="简要描述技能功能" />
              </el-form-item>
              <el-form-item label="状态">
                <div style="display:flex;align-items:center;gap:10px">
                  <el-switch v-model="metaForm.enabled" />
                  <span style="font-size:12px;color:#909399">{{ metaForm.enabled ? '已启用，SKILL.md 将注入系统提示' : '已禁用' }}</span>
                </div>
              </el-form-item>

              <!-- JSON 预览 -->
              <el-collapse style="margin-top:8px">
                <el-collapse-item title="查看 skill.json 原文">
                  <pre class="json-preview">{{ JSON.stringify({ id: selected.id, ...metaForm }, null, 2) }}</pre>
                </el-collapse-item>
              </el-collapse>
            </el-form>
          </div>

          <!-- SKILL.md 编辑 -->
          <div v-else class="file-editor">
            <div class="file-editor-head">
              <el-icon><Document /></el-icon> SKILL.md
              <span class="file-hint">注入到 AI System Prompt 的指令内容</span>
              <div style="margin-left:auto;display:flex;align-items:center;gap:8px">
                <el-tag v-if="promptDirty" type="warning" size="small">未保存</el-tag>
                <span style="font-size:11px;color:#c0c4cc">{{ promptContent.length }} 字符</span>
                <el-button size="small" circle :loading="promptLoading" @click="reloadPrompt" title="重新加载">
                  <el-icon><Refresh /></el-icon>
                </el-button>
              </div>
            </div>
            <textarea
              v-model="promptContent"
              class="code-textarea"
              spellcheck="false"
              placeholder="# 技能名称

## 功能说明
描述该技能的用途…

## 行为规范
- 规范 1
- 规范 2"
              @input="promptDirty = true"
            />
          </div>
        </div>
      </template>
    </div>

    <!-- ── 右：AI 协作聊天 ── -->
    <div class="studio-chat">
      <div class="chat-panel-head">
        <el-icon><ChatLineRound /></el-icon>
        AI 协作配置
        <span v-if="selected" style="margin-left:auto;font-size:11px;color:#c0c4cc">当前: {{ selected.name }}</span>
      </div>
      <div class="chat-wrap">
        <AiChat
          ref="aiChatRef"
          :agent-id="agentId"
          :context="chatContext"
          scenario="skill-studio"
          :welcome-message="chatWelcome"
          :examples="chatExamples"
          compact
          @response="onAiResponse"
        />
      </div>
    </div>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { agentSkills as skillsApi, files as filesApi, type AgentSkillMeta } from '../api'
import AiChat from './AiChat.vue'

const props = defineProps<{ agentId: string }>()
const agentId = props.agentId

// ── State ──────────────────────────────────────────────────────────────────
const skills = ref<AgentSkillMeta[]>([])
const listLoading = ref(false)
const selected = ref<AgentSkillMeta | null>(null)
const activeFile = ref<'meta' | 'prompt'>('meta')

// Metadata form (mirrors selected skill)
const metaForm = ref({ name: '', icon: '', category: '', description: '', version: '1.0.0', enabled: true })

// SKILL.md
const promptContent = ref('')
const promptLoading = ref(false)
const promptDirty = ref(false)

const saving = ref(false)

// Create
const creating = ref(false)
const isNewSkill = ref(false)  // true when just created — AI should guide user

// AI chat ref (for sending test messages)
const aiChatRef = ref<InstanceType<typeof AiChat> | null>(null)

// ── AI Chat context ────────────────────────────────────────────────────────
const chatContext = computed(() => {
  if (!selected.value) return '你是一个技能配置助手，帮助用户设计和优化 AI 技能的系统提示词。'
  return `你是一个技能配置助手，正在帮助用户配置技能「${selected.value.name}」（ID: ${selected.value.id}）。

当前 SKILL.md 内容：
\`\`\`markdown
${promptContent.value || '（空）'}
\`\`\`

你可以帮助：
- 优化或重写 SKILL.md 系统提示词
- 测试技能效果（用户发消息即可测试）
- 给出技能设计建议`
})

const chatWelcome = computed(() => {
  if (!selected.value) return '选择一个技能后，我可以帮你优化配置、写 SKILL.md、测试效果。'
  if (isNewSkill.value) return `新技能已创建（ID: ${selected.value.id}）。告诉我你想要什么功能，我来帮你生成完整的 SKILL.md 内容，生成后直接点保存即可。`
  return `当前编辑「${selected.value.name}」。你可以让我优化 SKILL.md、测试效果，或者直接对话体验当前技能。`
})

const chatExamples = computed(() => {
  if (!selected.value) return ['帮我设计一个代码审查技能', '帮我写一个翻译助手的 SKILL.md']
  if (isNewSkill.value) return [
    '我需要一个中英互译技能，风格自然流畅',
    '帮我做一个 Go 代码审查专家，严格遵循最佳实践',
    '创建一个数据分析助手，擅长 SQL 和 Python',
  ]
  return [
    `帮我优化「${selected.value.name}」的 SKILL.md`,
    '这个技能怎么写效果更好？',
    '用中文回答一道历史题，测试当前技能效果',
  ]
})

// ── Load ───────────────────────────────────────────────────────────────────
async function loadList() {
  listLoading.value = true
  try {
    const res = await skillsApi.list(agentId)
    skills.value = res.data || []
    // Keep selected in sync
    if (selected.value) {
      const updated = skills.value.find(s => s.id === selected.value!.id)
      if (updated) {
        selected.value = updated
        syncMetaForm(updated)
      }
    }
  } catch { /* silent */ }
  finally { listLoading.value = false }
}

function syncMetaForm(sk: AgentSkillMeta) {
  metaForm.value = {
    name: sk.name, icon: sk.icon || '', category: sk.category || '',
    description: sk.description || '', version: sk.version || '1.0.0', enabled: sk.enabled,
  }
}

async function selectSkill(sk: AgentSkillMeta) {
  selected.value = sk
  syncMetaForm(sk)
  activeFile.value = 'meta'
  promptDirty.value = false
  promptContent.value = ''
  isNewSkill.value = false
}

async function switchToPrompt() {
  if (!selected.value) return
  if (activeFile.value === 'prompt') return
  activeFile.value = 'prompt'
  if (!promptContent.value) await reloadPrompt()
}

// ── Save ───────────────────────────────────────────────────────────────────
async function saveSkill() {
  if (!selected.value) return
  saving.value = true
  try {
    // Save metadata
    await skillsApi.update(props.agentId, selected.value.id, {
      name: metaForm.value.name,
      icon: metaForm.value.icon,
      category: metaForm.value.category,
      description: metaForm.value.description,
      enabled: metaForm.value.enabled,
    })
    // Save SKILL.md if in prompt mode or if content was loaded
    if (activeFile.value === 'prompt' || promptContent.value) {
      await filesApi.write(agentId, `skills/${selected.value.id}/SKILL.md`, promptContent.value)
      promptDirty.value = false
    }
    ElMessage.success('保存成功')
    await loadList()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存失败')
  } finally { saving.value = false }
}

// ── Toggle ─────────────────────────────────────────────────────────────────
async function toggleSkill(sk: AgentSkillMeta, enabled: boolean) {
  try {
    await skillsApi.update(props.agentId, sk.id, { enabled })
    await loadList()
  } catch { ElMessage.error('操作失败') }
}

// ── Delete ─────────────────────────────────────────────────────────────────
async function deleteSkill() {
  if (!selected.value) return
  try {
    await skillsApi.remove(props.agentId, selected.value.id)
    ElMessage.success('已删除')
    selected.value = null
    await loadList()
  } catch { ElMessage.error('删除失败') }
}

// ── Create ─────────────────────────────────────────────────────────────────
// 直接在左侧新增空白技能，无弹窗
async function openNew() {
  if (creating.value) return
  creating.value = true
  // 生成唯一 ID：skill_ + base36 timestamp
  const id = 'skill_' + Date.now().toString(36)
  try {
    await skillsApi.create(props.agentId, {
      meta: {
        id, name: '新技能', icon: '', category: '', description: '',
        version: '1.0.0', enabled: false, source: 'local', installedAt: '',
      },
      promptContent: '',
    })
    await loadList()
    const sk = skills.value.find(s => s.id === id)
    if (sk) {
      await selectSkill(sk)
      // 直接跳到 SKILL.md 编辑器，引导用户用 AI 生成内容
      activeFile.value = 'prompt'
      promptContent.value = ''
      isNewSkill.value = true
    }
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '创建失败')
  } finally { creating.value = false }
}

// ── AI response hook ──────────────────────────────────────────────────────
async function onAiResponse(_text: string) {
  if (!selected.value) return
  isNewSkill.value = false
  // Reload skill metadata (in case AI modified it)
  await loadList()
  // Reload SKILL.md if currently viewing it
  if (activeFile.value === 'prompt') {
    await reloadPrompt()
  }
}

async function reloadPrompt() {
  if (!selected.value) return
  promptLoading.value = true
  try {
    const res = await filesApi.read(agentId, `skills/${selected.value.id}/SKILL.md`)
    promptContent.value = res.data?.content || ''
    promptDirty.value = false
  } catch { promptContent.value = '' }
  finally { promptLoading.value = false }
}

// ── Test ───────────────────────────────────────────────────────────────────
async function sendTestToChat() {
  if (!selected.value) return
  // Load SKILL.md if not yet loaded
  if (!promptContent.value) await switchToPrompt()
  const testMsg = `请用「${selected.value.name}」技能效果回复：你好，请介绍一下你的功能。`
  aiChatRef.value?.fillInput?.(testMsg)
  ElMessage.info('测试消息已填入右侧聊天框，点击发送即可测试')
}

onMounted(loadList)
</script>

<style scoped>
.skill-studio {
  display: flex;
  height: 600px;
  min-height: 400px;
  overflow: hidden;
  gap: 0;
  background: #f5f7fa;
}

/* ── Sidebar ── */
.studio-sidebar {
  width: 220px;
  flex-shrink: 0;
  background: #fff;
  border-right: 1px solid #e4e7ed;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.sidebar-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 14px;
  border-bottom: 1px solid #f0f0f0;
}
.sidebar-title { font-size: 13px; font-weight: 600; color: #303133; }
.sidebar-acts { display: flex; gap: 6px; }

.skill-list { flex: 1; overflow-y: auto; padding: 6px 0; }
.list-empty { text-align: center; color: #c0c4cc; font-size: 13px; padding: 32px 0; }

.skill-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 9px 14px;
  cursor: pointer;
  border-left: 3px solid transparent;
  transition: all 0.15s;
}
.skill-item:hover { background: #f5f7fa; }
.skill-item.active { background: #ecf5ff; border-left-color: #409eff; }

.sk-icon { font-size: 18px; flex-shrink: 0; width: 24px; text-align: center; }
.sk-info { flex: 1; min-width: 0; }
.sk-name { font-size: 13px; font-weight: 500; color: #303133; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.sk-id { font-size: 11px; color: #c0c4cc; font-family: monospace; }
.sk-right { display: flex; align-items: center; flex-shrink: 0; }

/* ── Editor ── */
.studio-editor {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: #fff;
  border-right: 1px solid #e4e7ed;
}

.editor-empty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  color: #c0c4cc;
  font-size: 14px;
}

.editor-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 16px;
  border-bottom: 1px solid #f0f0f0;
  background: #fafafa;
  flex-shrink: 0;
}
.editor-breadcrumb {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: #909399;
}
.crumb-sep { color: #c0c4cc; }
.crumb-name { font-weight: 600; color: #303133; font-family: monospace; }
.toolbar-acts { display: flex; gap: 8px; }

.editor-body {
  flex: 1;
  display: flex;
  overflow: hidden;
}

/* File tree */
.file-tree {
  width: 150px;
  flex-shrink: 0;
  border-right: 1px solid #f0f0f0;
  background: #fafafa;
  overflow-y: auto;
  padding: 8px 0;
}
.tree-title {
  font-size: 11px;
  color: #c0c4cc;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  padding: 4px 12px 8px;
}
.tree-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  font-size: 12px;
  color: #606266;
  cursor: pointer;
  transition: background 0.1s;
  font-family: monospace;
}
.tree-item:hover { background: #f0f0f0; }
.tree-item.tree-active { background: #ecf5ff; color: #409eff; font-weight: 600; }
.tree-item.tree-dir { color: #e6a23c; cursor: default; }
.tree-item.tree-dir:hover { background: transparent; }

/* File editor */
.file-editor {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.file-editor-head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 16px;
  font-size: 12px;
  color: #606266;
  background: #f5f7fa;
  border-bottom: 1px solid #f0f0f0;
  flex-shrink: 0;
}
.file-hint { color: #c0c4cc; font-size: 11px; margin-left: 4px; }

.json-preview {
  font-size: 12px;
  font-family: monospace;
  color: #606266;
  background: #f5f7fa;
  padding: 10px;
  border-radius: 4px;
  margin: 0;
  white-space: pre;
  overflow-x: auto;
}

.code-textarea {
  flex: 1;
  width: 100%;
  height: 100%;
  min-height: 0;
  resize: none;
  border: none;
  outline: none;
  padding: 16px 20px;
  font-family: 'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace;
  font-size: 13px;
  line-height: 1.65;
  color: #303133;
  background: #fff;
  tab-size: 2;
  box-sizing: border-box;
}
.code-textarea:focus { background: #fffef8; }
.code-textarea::placeholder { color: #c0c4cc; }

/* ── Chat ── */
.studio-chat {
  width: 340px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: #fff;
}
.chat-panel-head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 11px 14px;
  font-size: 13px;
  font-weight: 600;
  color: #303133;
  border-bottom: 1px solid #f0f0f0;
  background: #fafafa;
  flex-shrink: 0;
}
.chat-wrap {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}
</style>
