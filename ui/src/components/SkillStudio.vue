<template>
  <div class="skill-studio">
    <!-- ── 左：技能列表 ── -->
    <div class="studio-sidebar" :style="{ width: sideW + 'px' }">
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

    <!-- 拖拽手柄 1: 侧边栏 ↔ 编辑器 -->
    <div class="ss-handle" @mousedown="startResize($event, 'side')" :class="{ dragging: dragging === 'side' }"><div class="ss-handle-bar"/></div>

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
          <div class="file-tree" :style="{ width: treeW + 'px' }">
            <!-- 树顶部工具栏 -->
            <div class="tree-title">
              <span>文件</span>
              <div style="display:flex;gap:2px;margin-left:auto">
                <el-tooltip content="新建文件" placement="top" :show-after="500">
                  <el-button link size="small" @click="openNewFileDialog('')">
                    <el-icon><DocumentAdd /></el-icon>
                  </el-button>
                </el-tooltip>
                <el-tooltip content="新建目录" placement="top" :show-after="500">
                  <el-button link size="small" @click="openNewDirDialog('')">
                    <el-icon><FolderAdd /></el-icon>
                  </el-button>
                </el-tooltip>
                <el-button link size="small" :loading="dirLoading" @click="loadDirFiles">
                  <el-icon><Refresh /></el-icon>
                </el-button>
              </div>
            </div>

            <!-- 根目录行（不可删除） -->
            <div class="tree-item tree-dir-root">
              <el-icon style="color:#e6a23c"><Folder /></el-icon>
              <span class="tree-name">{{ selected.id }}/</span>
            </div>

            <!-- skill.json 固定入口 -->
            <div :class="['tree-item', { 'tree-active': activeFile === 'meta' }]" @click="activeFile = 'meta'">
              <span class="tree-indent" />
              <el-icon style="color:#409eff"><Setting /></el-icon>
              <span class="tree-name">skill.json</span>
            </div>

            <!-- 动态文件 / 目录列表 -->
            <template v-for="f in visibleFiles" :key="f.path">
              <div
                :class="['tree-item', {
                  'tree-active': activeFile === (f.path === 'SKILL.md' ? 'prompt' : f.path) && !f.isDir,
                  'tree-dir-row': f.isDir,
                }]"
                :style="{ paddingLeft: `${8 + (f.depth + 1) * 12}px` }"
                @click="f.isDir ? toggleDir(f.path) : openFile(f.path, false)"
              >
                <!-- 目录箭头 -->
                <el-icon v-if="f.isDir" class="dir-arrow" :class="{ 'dir-open': !collapsedDirs.has(f.path) }">
                  <ArrowRight />
                </el-icon>
                <el-icon v-if="f.isDir" style="color:#e6a23c"><Folder /></el-icon>
                <el-icon v-else style="color:#909399"><Document /></el-icon>
                <span class="tree-name">{{ f.name }}</span>
                <el-tag v-if="f.path === 'SKILL.md' && selected.enabled" size="small" type="success" effect="plain" style="margin-left:2px;font-size:10px;flex-shrink:0">注入</el-tag>

                <!-- 悬停操作按钮 -->
                <div class="tree-item-acts" @click.stop>
                  <el-tooltip v-if="f.isDir" content="在此目录新建文件" placement="top" :show-after="300">
                    <el-button link size="small" @click="openNewFileDialog(f.path)">
                      <el-icon><DocumentAdd /></el-icon>
                    </el-button>
                  </el-tooltip>
                  <el-tooltip v-if="f.isDir" content="在此目录新建子目录" placement="top" :show-after="300">
                    <el-button link size="small" @click="openNewDirDialog(f.path)">
                      <el-icon><FolderAdd /></el-icon>
                    </el-button>
                  </el-tooltip>
                  <el-tooltip v-if="!f.isDir" content="重命名" placement="top" :show-after="300">
                    <el-button link size="small" @click="openRenameDialog(f.path)">
                      <el-icon><Edit /></el-icon>
                    </el-button>
                  </el-tooltip>
                  <el-popconfirm
                    :title="`删除 ${f.name}？`"
                    @confirm="deleteFile(f.path)"
                    width="180"
                  >
                    <template #reference>
                      <el-button link size="small" type="danger">
                        <el-icon><Delete /></el-icon>
                      </el-button>
                    </template>
                  </el-popconfirm>
                </div>
              </div>
            </template>

            <div v-if="!dirLoading && dirFiles.length === 0" class="tree-empty">空目录</div>
          </div>

          <!-- 新建文件/目录 对话框 -->
          <el-dialog v-model="newEntryDialog.visible" :title="newEntryDialog.isDir ? '新建目录' : '新建文件'" width="360px" :close-on-click-modal="false">
            <el-form @submit.prevent="createEntry">
              <div style="font-size:12px;color:#909399;margin-bottom:8px" v-if="newEntryDialog.inDir">
                位置：{{ selected.id }}/{{ newEntryDialog.inDir }}/
              </div>
              <el-input
                v-model="newEntryDialog.name"
                :placeholder="newEntryDialog.isDir ? '目录名（如 tools）' : '文件名（如 config.json）'"
                autofocus
                @keyup.enter="createEntry"
              />
            </el-form>
            <template #footer>
              <el-button @click="newEntryDialog.visible = false">取消</el-button>
              <el-button type="primary" :loading="newEntryDialog.creating" @click="createEntry">创建</el-button>
            </template>
          </el-dialog>

          <!-- 重命名对话框 -->
          <el-dialog v-model="renameDialog.visible" title="重命名文件" width="360px" :close-on-click-modal="false">
            <el-input v-model="renameDialog.newName" :placeholder="renameDialog.oldPath" @keyup.enter="doRename" />
            <template #footer>
              <el-button @click="renameDialog.visible = false">取消</el-button>
              <el-button type="primary" :loading="renameDialog.saving" @click="doRename">重命名</el-button>
            </template>
          </el-dialog>

          <!-- 拖拽手柄 2: 文件树 ↔ 编辑区 -->
          <div class="ss-handle" @mousedown="startResize($event, 'tree')" :class="{ dragging: dragging === 'tree' }"><div class="ss-handle-bar"/></div>

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
                <span style="font-size:11px;color:#c0c4cc">{{ promptLineCount }} 行 · {{ promptContent.length }} 字符</span>
                <el-button size="small" circle :loading="promptLoading" @click="reloadPrompt" title="重新加载">
                  <el-icon><Refresh /></el-icon>
                </el-button>
                <el-button size="small" circle :type="editorFullscreen ? 'primary' : ''" @click="editorFullscreen = !editorFullscreen" :title="editorFullscreen ? '退出全屏编辑' : '全屏编辑'">
                  <el-icon><FullScreen /></el-icon>
                </el-button>
              </div>
            </div>
            <div class="code-editor-wrap">
              <div class="line-numbers" aria-hidden="true">
                <div v-for="n in promptLineCount" :key="n" class="line-num">{{ n }}</div>
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
            <!-- AI diff 预览条（Cursor 风格）-->
            <div v-if="pendingEdit?.file === 'SKILL.md'" class="diff-bar">
              <div class="diff-bar-left">
                <span class="diff-tag">AI 建议修改</span>
                <span class="diff-summary">{{ pendingEdit.summary }}</span>
                <span class="diff-stats">{{ pendingEditStats }}</span>
              </div>
              <div class="diff-bar-right">
                <el-button size="small" type="primary" @click="applyPendingEdit">✅ 应用</el-button>
                <el-button size="small" @click="pendingEdit = null">✕ 忽略</el-button>
              </div>
            </div>
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
            <div class="code-editor-wrap">
              <textarea
                v-model="genericContent"
                class="code-textarea"
                spellcheck="false"
                :placeholder="`编辑 ${activeFile} …`"
                @input="genericDirty = true"
              />
            </div>
          </div>
        </div>
      </template>
    </div>

    <!-- 拖拽手柄 3: 编辑器 ↔ 聊天（全屏时隐藏） -->
    <div v-show="!editorFullscreen" class="ss-handle" @mousedown="startResize($event, 'chat')" :class="{ dragging: dragging === 'chat' }"><div class="ss-handle-bar"/></div>

    <!-- ── 右：AI 协作聊天（全屏时隐藏） ── -->
    <div v-show="!editorFullscreen" class="studio-chat" :style="{ width: chatW + 'px' }">
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

// ── Panel resize ──────────────────────────────────────────────────────────
const sideW    = ref(200)  // sidebar width
const treeW    = ref(140)  // file tree width
const chatW    = ref(340)  // chat panel width
const dragging = ref<'side'|'tree'|'chat'|''>('')

function startResize(e: MouseEvent, target: 'side'|'tree'|'chat') {
  const startX = e.clientX
  const startW = target === 'side' ? sideW.value : target === 'tree' ? treeW.value : chatW.value
  dragging.value = target
  const onMove = (ev: MouseEvent) => {
    const d = ev.clientX - startX
    if      (target === 'side') sideW.value = Math.max(140, Math.min(340, startW + d))
    else if (target === 'tree') treeW.value = Math.max(100, Math.min(280, startW + d))
    else                        chatW.value = Math.max(240, Math.min(560, startW - d))
  }
  const onUp = () => {
    dragging.value = ''
    window.removeEventListener('mousemove', onMove)
    window.removeEventListener('mouseup', onUp)
    document.body.style.cursor = ''; document.body.style.userSelect = ''
  }
  window.addEventListener('mousemove', onMove)
  window.addEventListener('mouseup', onUp)
  document.body.style.cssText += 'cursor:col-resize;user-select:none;'
}

// ── State ──────────────────────────────────────────────────────────────────
const skills = ref<AgentSkillMeta[]>([])
const listLoading = ref(false)
const selected = ref<AgentSkillMeta | null>(null)
const activeFile = ref<string>('meta')
const editorFullscreen = ref(false)  // 全屏编辑模式（隐藏右侧 AI 聊天）

// ── AI 直接编辑文件（Cursor 风格 diff 预览）────────────────────────────────
interface PendingEdit { file: string; content: string; summary: string }
const pendingEdit = ref<PendingEdit | null>(null)
const pendingEditStats = computed(() => {
  if (!pendingEdit.value) return ''
  const oldLines = (pendingEdit.value.file === 'SKILL.md' ? promptContent.value : genericContent.value).split('\n')
  const newLines = pendingEdit.value.content.split('\n')
  const added   = newLines.filter(l => !oldLines.includes(l)).length
  const removed = oldLines.filter(l => !newLines.includes(l)).length
  return `+${added} / -${removed} 行`
})
function applyPendingEdit() {
  if (!pendingEdit.value) return
  if (pendingEdit.value.file === 'SKILL.md') {
    promptContent.value = pendingEdit.value.content
    promptDirty.value = true
    activeFile.value = 'prompt'
  } else {
    genericContent.value = pendingEdit.value.content
    genericDirty.value = true
    activeFile.value = pendingEdit.value.file
  }
  ElMessage.success('已应用 AI 修改')
  pendingEdit.value = null
}

// Metadata form (mirrors selected skill)
const metaForm = ref({ name: '', icon: '', category: '', description: '', version: '1.0.0', enabled: true })

// SKILL.md
const promptContent = ref('')
const promptLoading = ref(false)
const promptDirty = ref(false)
const promptLineCount = computed(() => Math.max(1, promptContent.value.split('\n').length))

const saving = ref(false)

// Create
const creating = ref(false)
const isNewSkill = ref(false)  // true when just created — AI should guide user

// 等待 AI 生成完成后再切换的目标 session ID

// Dynamic directory listing (recursive)
interface DirEntry { name: string; path: string; isDir: boolean; depth: number }
const dirFiles = ref<DirEntry[]>([])
const dirLoading = ref(false)

// ── 目录展开/收起 ──────────────────────────────────────────────────────────
const collapsedDirs = ref(new Set<string>())

function toggleDir(path: string) {
  if (collapsedDirs.value.has(path)) collapsedDirs.value.delete(path)
  else collapsedDirs.value.add(path)
  // Trigger reactivity
  collapsedDirs.value = new Set(collapsedDirs.value)
}

const visibleFiles = computed(() => dirFiles.value.filter(f => {
  const parts = f.path.split('/')
  for (let i = 1; i < parts.length; i++) {
    if (collapsedDirs.value.has(parts.slice(0, i).join('/'))) return false
  }
  return true
}))

// ── 新建文件/目录 ──────────────────────────────────────────────────────────
const newEntryDialog = ref({ visible: false, isDir: false, inDir: '', name: '', creating: false })

function openNewFileDialog(inDir: string) {
  newEntryDialog.value = { visible: true, isDir: false, inDir, name: '', creating: false }
}
function openNewDirDialog(inDir: string) {
  newEntryDialog.value = { visible: true, isDir: true, inDir, name: '', creating: false }
}

async function createEntry() {
  const { isDir, inDir, name } = newEntryDialog.value
  if (!name.trim() || !selected.value) return
  newEntryDialog.value.creating = true
  const relPath = inDir ? `${inDir}/${name.trim()}` : name.trim()
  const skillBase = `skills/${selected.value.id}`
  try {
    if (isDir) {
      // Create a .gitkeep placeholder so the directory exists
      await filesApi.write(agentId, `${skillBase}/${relPath}/.gitkeep`, '')
    } else {
      await filesApi.write(agentId, `${skillBase}/${relPath}`, '')
    }
    newEntryDialog.value.visible = false
    await loadDirFiles()
    if (!isDir) {
      // Auto-open the new file
      await openFile(relPath, false)
    } else {
      // Auto-expand the new dir
      collapsedDirs.value.delete(relPath)
    }
    ElMessage.success(`${isDir ? '目录' : '文件'} ${relPath} 已创建`)
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '创建失败')
  } finally {
    newEntryDialog.value.creating = false
  }
}

// ── 重命名文件 ──────────────────────────────────────────────────────────────
const renameDialog = ref({ visible: false, oldPath: '', newName: '', saving: false })

function openRenameDialog(path: string) {
  const parts = path.split('/')
  renameDialog.value = { visible: true, oldPath: path, newName: parts[parts.length - 1] ?? '', saving: false }
}

async function doRename() {
  const { oldPath, newName } = renameDialog.value
  if (!newName.trim() || !selected.value) return
  renameDialog.value.saving = true
  const parts = oldPath.split('/')
  const dir = parts.slice(0, -1).join('/')
  const newPath = dir ? `${dir}/${newName.trim()}` : newName.trim()
  const skillBase = `skills/${selected.value.id}`
  try {
    // Read old, write new, delete old
    const res = await filesApi.read(agentId, `${skillBase}/${oldPath}`)
    await filesApi.write(agentId, `${skillBase}/${newPath}`, res.data?.content || '')
    await filesApi.delete(agentId, `${skillBase}/${oldPath}`)
    renameDialog.value.visible = false
    if (activeFile.value === oldPath) activeFile.value = newPath === 'SKILL.md' ? 'prompt' : newPath
    await loadDirFiles()
    ElMessage.success('重命名成功')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '重命名失败')
  } finally {
    renameDialog.value.saving = false
  }
}

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
  if (!selected.value) return '你是一个技能架构师，帮助用户设计和生成完整的 AI 技能包。'
  const sid = selected.value.id
  const base = `skills/${sid}`
  const currentFiles = dirFiles.value.map(f => f.path).join(', ') || '（空）'
  const skillState = selected.value.name ? `名称: ${selected.value.name}，分类: ${selected.value.category || '未设置'}` : '（新建，尚未配置）'

  return `你是一个专业的技能架构师，负责为 AI 成员生成完整、规范的技能包。
当前技能目录: ${base}/（技能 ID: ${sid}）
当前技能状态: ${skillState}
目录中已有文件: ${currentFiles}

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
## 一、技能包标准结构

一个完整的技能包包含以下文件：

${'```'}
skills/{id}/
├── SKILL.md          # 核心：注入 AI System Prompt 的指令（必须）
├── skill.json        # 元数据：名称/图标/分类/描述（自动管理，无需手写）
└── tools/            # 可选：工具脚本（仅需外部计算/数据处理时创建）
    ├── main.py       # 工具入口
    └── README.md     # 工具说明
${'```'}

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
## 二、SKILL.md 渐进式披露规范

**核心原则：按层次组织，让 LLM 先理解角色，再理解规则，最后才处理细节。**

SKILL.md 标准分层结构（越往后的章节越少被触发）：

${'```'}markdown
# [技能名称]

## 🎯 角色
[一句话定义：你是谁，核心使命是什么]

## ⚡ 核心能力
- 能力1：[简洁描述]
- 能力2：[简洁描述]
- 能力3：[简洁描述]（不超过5条）

## 📋 工作流程
[遇到任务时的标准思路/步骤，3-6步]

## 📐 输出规范
[格式要求：结构、语言风格、长度]

## ⚠️ 边界规则
[什么情况下拒绝 / 降级 / 澄清]

## 🔧 工具使用（可选，仅需工具时加此章节）
[工具调用规范和参数说明]
${'```'}

**写作要点：**
- 角色 + 核心能力：简短有力，LLM 每次都读
- 工作流程：结构化步骤，触发频率高
- 输出规范 / 边界规则：只在需要时展开，避免冗余
- 避免重复：同一信息只在一个章节出现

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
## 三、生成技能的完整流程（每次创建技能必须全部执行）

当用户描述一个技能需求，**按以下顺序依次完成**：

**步骤1：填写元数据**（输出 JSON，页面自动填充表单）
${'```'}json
{"action":"fill_skill","data":{"name":"技能名称","icon":"📊","category":"分类","description":"一句话功能描述","enabled":true}}
${'```'}

**步骤2：写入 SKILL.md**（write 工具，直接写文件）
路径：\`${base}/SKILL.md\`
按渐进式披露规范写完整内容。

**步骤3：按需创建工具文件**（仅技能需要执行代码/外部数据时）
如需工具，创建 \`${base}/tools/\` 目录并写入脚本。

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
## 四、修改技能

- **修改 SKILL.md**：先用 read 工具读取当前内容，理解后再用 write 工具写回
- **新增工具文件**：直接 write 到对应路径
- **优化提示词**：遵循渐进式披露，减少冗余
- **所有操作直接用工具完成，不要把内容输出给用户复制**

${promptContent.value && promptContent.value.length <= 1200
  ? `\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n## 当前 SKILL.md 内容\n\`\`\`markdown\n${promptContent.value}\n\`\`\``
  : promptContent.value ? `\n当前 SKILL.md 已有内容（${promptContent.value.length} 字符），如需修改请先 read 工具读取。` : ''}`
})

const chatWelcome = computed(() => {
  if (!selected.value) return '选择一个技能后，我可以帮你一键生成完整技能包（元数据 + SKILL.md + 工具文件）。'
  if (isNewSkill.value) return `新技能已创建（ID: ${selected.value.id}）。\n\n告诉我这个技能要做什么，我会**一次性生成完整技能包**：自动填写名称/图标/描述，写入规范的 SKILL.md（渐进式披露结构），如需工具也一并创建。`
  return `当前技能：「${selected.value.name || selected.value.id}」\n\n告诉我需要如何调整——我会直接用工具修改对应文件，不会让你手动复制内容。`
})

const chatExamples = computed(() => {
  if (!selected.value) return ['帮我生成一个财务报表审核技能', '帮我设计一个代码审查技能']
  if (isNewSkill.value) return [
    '生成完整技能包',
    '这个技能需要能分析利润表，识别异常数据，给出审核意见',
  ]
  return [
    `重新生成完整的 ${selected.value.name} 技能包`,
    '优化 SKILL.md 的渐进式披露结构',
    '为这个技能添加 Python 工具脚本',
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
async function onAiResponse(skillId: string, text: string) {
  if (skillId === selected.value?.id) isNewSkill.value = false

  // 尝试解析 fill_skill / edit_file JSON（兼容旧模式 + 备用协议）
  if (skillId === selected.value?.id) tryFillSkill(text)

  // 始终刷新编辑器：AI 可能通过 write 工具直接写了文件
  await loadList()
  if (skillId === selected.value?.id) {
    const prevContent = promptContent.value
    await Promise.all([loadDirFiles(), reloadPrompt()])
    // 如果 SKILL.md 内容有变化，自动切到编辑器并提示
    if (promptContent.value && promptContent.value !== prevContent) {
      activeFile.value = 'prompt'
      ElMessage.success({ message: 'SKILL.md 已更新', duration: 2000 })
    }
    if (activeFile.value !== 'meta' && activeFile.value !== 'prompt') {
      await reloadGenericFile()
    }
  }
}

// 解析并应用 fill_skill / edit_file JSON
function tryFillSkill(text: string): boolean {
  const tryApply = (jsonStr: string): boolean => {
    try {
      const obj = JSON.parse(jsonStr)

      // ── edit_file：AI 直接修改文件内容，显示 diff 预览 ──────────────────
      if (obj.action === 'edit_file' && obj.file && typeof obj.content === 'string') {
        pendingEdit.value = {
          file: obj.file,
          content: obj.content,
          summary: obj.summary || obj.file + ' 内容已更新',
        }
        activeFile.value = obj.file === 'SKILL.md' ? 'prompt' : obj.file
        return true
      }

      if (obj.action === 'fill_skill' && obj.data) {
        const d = obj.data
        if (d.name)        metaForm.value.name        = d.name
        if (d.icon)        metaForm.value.icon        = d.icon
        if (d.category)    metaForm.value.category    = d.category
        if (d.description) metaForm.value.description = d.description
        if (typeof d.enabled === 'boolean') metaForm.value.enabled = d.enabled
        if (d.prompt) {
          promptContent.value = d.prompt
          promptDirty.value = true
          activeFile.value = 'prompt'  // 自动切到 SKILL.md 编辑器
        }
        ElMessage.success('AI 已填写技能信息，确认后点击保存')
        return true
      }
    } catch {}
    return false
  }

  // 代码块内
  const codeBlock = text.match(/```(?:json)?\s*(\{[\s\S]*?\})\s*```/)
  if (codeBlock?.[1] && tryApply(codeBlock[1])) return true

  // 裸 JSON (fill_skill / edit_file)
  const bare = text.match(/(\{"action"\s*:\s*"(?:fill_skill|edit_file)"[\s\S]*?\})\s*(?:```|$)/)
  if (bare?.[1] && tryApply(bare[1])) return true

  return false
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
  height: 100%;   /* 由父元素传入 style="height: calc(...)" 控制 */
  min-height: 400px;
  overflow: hidden;
  gap: 0;
  background: #f5f7fa;
}

/* ── Sidebar ── */
.studio-sidebar {
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
.tree-item.tree-dir-root { color: #e6a23c; cursor: default; font-weight: 500; }
.tree-item.tree-dir-root:hover { background: transparent; }
.tree-name { overflow: hidden; text-overflow: ellipsis; flex: 1; min-width: 0; }
.tree-indent { display: inline-block; width: 12px; flex-shrink: 0; }
.tree-item-acts {
  display: flex; align-items: center; gap: 0;
  opacity: 0; transition: opacity 0.12s; margin-left: auto; flex-shrink: 0;
}
.tree-item:hover .tree-item-acts { opacity: 1; }
.tree-item-acts .el-button { padding: 0 2px; height: 18px; }
.dir-arrow { transition: transform 0.15s; color: #c0c4cc; flex-shrink: 0; font-size: 10px; }
.dir-arrow.dir-open { transform: rotate(90deg); }

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

/* ── Code editor with line numbers ── */
.code-editor-wrap {
  flex: 1;
  min-height: 0;
  display: flex;
  overflow: hidden;
  background: #fff;
}
.line-numbers {
  flex-shrink: 0;
  width: 44px;
  padding: 16px 0;
  background: #f8f9fa;
  border-right: 1px solid #ebeef5;
  overflow: hidden;
  user-select: none;
  text-align: right;
}
.line-num {
  padding: 0 8px 0 0;
  font-family: 'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace;
  font-size: 12px;
  line-height: 1.65;
  color: #c0c4cc;
  white-space: nowrap;
}
.code-textarea {
  flex: 1;
  min-width: 0;
  height: 100%;
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

/* ── Diff bar (Cursor-style apply prompt) ── */
.diff-bar {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 14px;
  background: #ecf5ff;
  border-top: 2px solid #409eff;
  gap: 12px;
  animation: diffSlide 0.2s ease;
}
@keyframes diffSlide {
  from { opacity: 0; transform: translateY(8px); }
  to   { opacity: 1; transform: translateY(0); }
}
.diff-bar-left  { display: flex; align-items: center; gap: 8px; min-width: 0; overflow: hidden; }
.diff-bar-right { display: flex; align-items: center; gap: 6px; flex-shrink: 0; }
.diff-tag {
  background: #409eff; color: #fff;
  padding: 1px 7px; border-radius: 10px;
  font-size: 11px; font-weight: 600; white-space: nowrap;
}
.diff-summary {
  font-size: 12px; color: #303133;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 300px;
}
.diff-stats {
  font-size: 11px; color: #67c23a; font-family: monospace; white-space: nowrap;
}

/* ── Chat ── */
.studio-chat {
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

/* ── 拖拽手柄 ── */
.ss-handle {
  width: 4px;
  background: #e4e7ed;
  cursor: col-resize;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background .15s;
  position: relative;
  z-index: 10;
}
.ss-handle:hover, .ss-handle.dragging { background: #409eff; }
.ss-handle-bar {
  width: 2px; height: 28px;
  background: rgba(255,255,255,.6);
  border-radius: 2px;
}
</style>
