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
            <!-- 后台生成中指示器 -->
            <span v-if="streamingSkills.has(sk.id)" class="sk-streaming-dot" title="AI 生成中…" />
            <el-tag v-else-if="sk.category" size="small" effect="plain" style="margin-right:6px;font-size:11px">{{ sk.category }}</el-tag>
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
            <div class="tree-title">
              目录
              <el-button link size="small" :loading="dirLoading" @click="loadDirFiles" style="margin-left:auto;padding:0">
                <el-icon><Refresh /></el-icon>
              </el-button>
            </div>
            <div class="tree-item tree-dir">
              <el-icon><Folder /></el-icon>
              <span>{{ selected.id }}/</span>
            </div>
            <!-- skill.json 固定入口 -->
            <div :class="['tree-item', { 'tree-active': activeFile === 'meta' }]" @click="activeFile = 'meta'">
              <el-icon><Document /></el-icon>
              <span>skill.json</span>
            </div>
            <!-- 动态文件列表（递归，排除 skill.json） -->
            <div
              v-for="f in dirFiles" :key="f.path"
              :class="['tree-item', { 'tree-active': activeFile === (f.path === 'SKILL.md' ? 'prompt' : f.path), 'tree-dir-row': f.isDir }]"
              :style="{ paddingLeft: `${12 + f.depth * 12}px` }"
              @click="openFile(f.path, f.isDir)"
            >
              <el-icon v-if="f.isDir" style="color:#e6a23c"><Folder /></el-icon>
              <el-icon v-else><Document /></el-icon>
              <span>{{ f.name }}</span>
              <el-tag v-if="f.path === 'SKILL.md' && selected.enabled" size="small" type="success" effect="plain" style="margin-left:4px;font-size:10px">注入中</el-tag>
            </div>
            <div v-if="!dirLoading && dirFiles.length === 0" class="tree-empty">空目录</div>
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
          <div v-else-if="activeFile === 'prompt'" class="file-editor">
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

          <!-- 通用文件编辑器（AI 生成的工具文件等） -->
          <div v-else class="file-editor">
            <div class="file-editor-head">
              <el-icon><Document /></el-icon> {{ activeFile }}
              <div style="margin-left:auto;display:flex;align-items:center;gap:8px">
                <el-tag v-if="genericDirty" type="warning" size="small">未保存</el-tag>
                <span style="font-size:11px;color:#c0c4cc">{{ genericContent.length }} 字符</span>
                <el-button size="small" circle :loading="genericLoading" @click="reloadGenericFile" title="重新加载">
                  <el-icon><Refresh /></el-icon>
                </el-button>
                <el-popconfirm title="确认删除该文件？" @confirm="deleteFile(activeFile)">
                  <template #reference>
                    <el-button size="small" circle type="danger" plain><el-icon><Delete /></el-icon></el-button>
                  </template>
                </el-popconfirm>
              </div>
            </div>
            <textarea
              v-model="genericContent"
              class="code-textarea"
              spellcheck="false"
              :placeholder="`编辑 ${activeFile} …`"
              @input="genericDirty = true"
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
        <span v-if="selected" style="margin-left:auto;font-size:11px;color:#c0c4cc">
          当前: {{ selected.name }}
          <span v-if="streamingSkills.size > 1" style="margin-left:6px;color:#e6a23c">
            ({{ streamingSkills.size }} 个并行生成中)
          </span>
        </span>
      </div>
      <!-- 每个 skill 一个独立 AiChat 实例，v-show 切换可见性，支持并发后台生成 -->
      <div class="chat-wrap">
        <AiChat
          v-for="sk in skills"
          v-show="selected?.id === sk.id"
          :key="sk.id"
          :ref="(el) => setChatRef(sk.id, el)"
          :agent-id="agentId"
          :context="selected?.id === sk.id ? chatContext : ''"
          scenario="skill-studio"
          :skill-id="sk.id"
          :welcome-message="selected?.id === sk.id ? chatWelcome : ''"
          :examples="selected?.id === sk.id ? chatExamples : []"
          compact
          @response="(text: string) => onAiResponse(sk.id, text)"
          @streaming-change="(v: boolean) => onStreamingChange(sk.id, v)"
        />
      </div>
    </div>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import { agentSkills as skillsApi, files as filesApi, type AgentSkillMeta } from '../api'
import AiChat from './AiChat.vue'

const props = defineProps<{ agentId: string }>()
const agentId = props.agentId

// ── State ──────────────────────────────────────────────────────────────────
const skills = ref<AgentSkillMeta[]>([])
const listLoading = ref(false)
const selected = ref<AgentSkillMeta | null>(null)
const activeFile = ref<string>('meta')

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

// 等待 AI 生成完成后再切换的目标 session ID

// Dynamic directory listing (recursive)
interface DirEntry { name: string; path: string; isDir: boolean; depth: number }
const dirFiles = ref<DirEntry[]>([])
const dirLoading = ref(false)

// Generic file editor (for non-skill.json / non-SKILL.md files)
const genericContent = ref('')
const genericDirty = ref(false)
const genericLoading = ref(false)


// 每个 skill 独立的 AiChat 实例（支持并发后台生成）
const chatRefsMap: Record<string, any> = {}
function setChatRef(skillId: string, el: any) {
  if (el) chatRefsMap[skillId] = el
  else delete chatRefsMap[skillId]
}
function getChatRef(skillId?: string): any {
  return skillId ? chatRefsMap[skillId] : null
}

// 正在流式生成的 skill 集合（用于 UI 指示器）
const streamingSkills = ref<Set<string>>(new Set())
function onStreamingChange(skillId: string, streaming: boolean) {
  const next = new Set(streamingSkills.value)
  if (streaming) next.add(skillId)
  else next.delete(skillId)
  streamingSkills.value = next
}

// 已初始化过 session 的 skill 集合
const initializedSessions = ref<Set<string>>(new Set())

// 当选中技能变化时，首次初始化其 chat session
watch(selected, async (sk) => {
  if (!sk) return
  if (initializedSessions.value.has(sk.id)) return
  initializedSessions.value.add(sk.id)
  await nextTick()  // 等 DOM 渲染出对应的 AiChat 实例
  await getChatRef(sk.id)?.resumeSession?.(`skill-studio-${sk.id}`)
}, { flush: 'post' })

// ── AI Chat context ────────────────────────────────────────────────────────
const chatContext = computed(() => {
  if (!selected.value) return '你是一个技能配置助手，帮助用户设计和优化 AI 技能的系统提示词。'
  const skillJsonTemplate = JSON.stringify({
    id: selected.value.id,
    name: selected.value.name,
    icon: selected.value.icon || '',
    category: selected.value.category || '',
    description: selected.value.description || '',
    version: selected.value.version || '1.0.0',
    enabled: selected.value.enabled,
    source: 'local',
    installedAt: ''
  }, null, 2)
  return `你是一个技能配置助手，正在帮助用户配置技能（ID: ${selected.value.id}）。

## ⚠️ 文件路径规则（必须遵守）
所有文件必须在 skills/${selected.value.id}/ 目录下：
- ✅ skills/${selected.value.id}/SKILL.md
- ✅ skills/${selected.value.id}/skill.json
- ✅ skills/${selected.value.id}/tools/helper.py
- ❌ 任何其他路径

## ⚠️ 创建或更新技能时，必须同时写两个文件

### 1. skills/${selected.value.id}/skill.json（技能元数据）
格式如下，修改 name/icon/category/description 字段：
\`\`\`json
${skillJsonTemplate}
\`\`\`

### 2. skills/${selected.value.id}/SKILL.md（系统提示词）
当前内容：
\`\`\`markdown
${promptContent.value || '（空）'}
\`\`\`

## 你可以帮助：
- 创建/优化技能：同时写 skill.json（名称、分类、描述）和 SKILL.md（提示词）
- 在技能目录下创建工具文件（Python 等）
- 测试技能效果（直接对话即可测试）`
})

const chatWelcome = computed(() => {
  if (!selected.value) return '选择一个技能后，我可以帮你优化配置、写 SKILL.md、测试效果。'
  if (isNewSkill.value) return `新技能已创建（ID: ${selected.value.id}）。告诉我你想要什么功能，我来帮你生成完整的 SKILL.md 内容，生成后直接点保存即可。`
  return `当前编辑「${selected.value.name}」。你可以让我优化 SKILL.md、测试效果，或者直接对话体验当前技能。`
})

const chatExamples = computed(() => {
  if (!selected.value) return ['帮我设计一个代码审查技能', '帮我写一个翻译助手的 SKILL.md']
  if (isNewSkill.value) return []  // AI 会自动推荐，不用静态按钮
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
  // 已选中同一个技能：跳过
  if (selected.value?.id === sk.id) return

  // 切换编辑器视图（立即生效，不影响任何 AiChat 的流）
  selected.value = sk
  syncMetaForm(sk)
  activeFile.value = 'meta'
  promptDirty.value = false
  promptContent.value = ''
  isNewSkill.value = false
  loadDirFiles()
  reloadPrompt()
  // session 初始化由 watch(selected) 处理（首次选中时）
}

async function switchToPrompt() {
  if (!selected.value) return
  if (activeFile.value === 'prompt') return
  activeFile.value = 'prompt'
  if (!promptContent.value) await reloadPrompt()
}

// 递归读取目录，返回扁平列表（含深度和相对 path）
async function readDirRecursive(apiPath: string, relPrefix: string, depth: number): Promise<DirEntry[]> {
  const res = await filesApi.read(agentId, apiPath)
  const entries: any[] = Array.isArray(res.data) ? res.data : []
  const result: DirEntry[] = []
  for (const f of entries) {
    if (depth === 0 && f.name === 'skill.json') continue  // skill.json 固定显示，跳过
    const relPath = relPrefix ? `${relPrefix}/${f.name}` : f.name
    result.push({ name: f.name, path: relPath, isDir: f.isDir, depth })
    if (f.isDir) {
      const children = await readDirRecursive(
        `skills/${selected.value!.id}/${relPath}`,
        relPath, depth + 1
      )
      result.push(...children)
    }
  }
  return result
}

async function loadDirFiles() {
  if (!selected.value) return
  dirLoading.value = true
  try {
    dirFiles.value = await readDirRecursive(`skills/${selected.value.id}/`, '', 0)
  } catch {
    dirFiles.value = [{ name: 'SKILL.md', path: 'SKILL.md', isDir: false, depth: 0 }]
  } finally {
    dirLoading.value = false
  }
}

// path = 相对于 skills/{skillId}/ 的路径，如 "SKILL.md" 或 "tools/eda.py"
async function openFile(path: string, isDir: boolean) {
  if (isDir) return  // 目录不可打开
  if (path === 'SKILL.md') { await switchToPrompt(); return }
  activeFile.value = path
  genericDirty.value = false
  await reloadGenericFile()
}

async function reloadGenericFile() {
  if (!selected.value || !activeFile.value || activeFile.value === 'meta' || activeFile.value === 'prompt') return
  genericLoading.value = true
  try {
    const res = await filesApi.read(agentId, `skills/${selected.value.id}/${activeFile.value}`)
    genericContent.value = res.data?.content || ''
    genericDirty.value = false
  } catch { genericContent.value = '' }
  finally { genericLoading.value = false }
}

async function deleteFile(path: string) {
  if (!selected.value) return
  try {
    await filesApi.delete(agentId, `skills/${selected.value.id}/${path}`)
    if (activeFile.value === path) activeFile.value = 'prompt'
    await loadDirFiles()
    ElMessage.success('已删除')
  } catch { ElMessage.error('删除失败') }
}

// ── Save ───────────────────────────────────────────────────────────────────
async function saveSkill() {
  if (!selected.value) return
  saving.value = true
  try {
    if (activeFile.value === 'meta' || activeFile.value === 'prompt') {
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
      await loadList()
    } else {
      // 通用文件保存
      await filesApi.write(agentId, `skills/${selected.value.id}/${activeFile.value}`, genericContent.value)
      genericDirty.value = false
    }
    ElMessage.success('保存成功')
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
      // 等 watch(selected) 初始化 session 完成（resumeSession 404→空）
      await nextTick()
      // 确保 initializedSessions 已处理
      if (!initializedSessions.value.has(id)) {
        initializedSessions.value.add(id)
        await getChatRef(id)?.resumeSession?.(`skill-studio-${id}`)
      }
      // 欢迎词已通过 chatWelcome computed + :welcome-message 展示，无需 AI 自动发消息
    }
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '创建失败')
  } finally { creating.value = false }
}

// ── AI response hook ──────────────────────────────────────────────────────
// skillId: 哪个 skill 的 AI 刚生成完（可能是后台 skill，不一定是当前选中的）
async function onAiResponse(skillId: string, _text: string) {
  // 清除新建状态
  if (skillId === selected.value?.id) isNewSkill.value = false

  // 刷新该 skill 的元数据 + 目录（AI 可能创建了新文件）
  await loadList()

  // 只有当前选中的 skill 响应时，才刷新编辑器
  if (skillId === selected.value?.id) {
    await Promise.all([loadDirFiles(), reloadPrompt()])
    if (activeFile.value !== 'meta' && activeFile.value !== 'prompt') {
      await reloadGenericFile()
    }
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
  getChatRef(selected.value?.id)?.fillInput?.(testMsg)
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
.sk-right { display: flex; align-items: center; flex-shrink: 0; gap: 6px; }
.sk-streaming-dot {
  display: inline-block;
  width: 7px; height: 7px;
  border-radius: 50%;
  background: #67c23a;
  animation: pulse-dot 1.2s ease-in-out infinite;
  flex-shrink: 0;
}
@keyframes pulse-dot {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.4; transform: scale(0.7); }
}

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
  display: flex;
  align-items: center;
  font-size: 11px;
  color: #c0c4cc;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  padding: 4px 12px 8px;
}
.tree-empty {
  font-size: 11px;
  color: #dcdfe6;
  padding: 4px 12px;
  font-family: monospace;
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
