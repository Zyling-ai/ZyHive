<template>
  <div class="wc-layout">

    <!-- ── 左栏：文件树 ── -->
    <div class="wc-panel wc-panel-left" :style="{ width: leftW + 'px' }">
      <div class="wc-panel-header">
        <span class="wc-panel-title">📁 工作区文件</span>
        <div class="wc-header-actions">
          <el-tooltip content="新建文件" placement="top" :show-after="500">
            <button class="wc-icon-btn" @click="showNewFile = true">＋</button>
          </el-tooltip>
          <el-tooltip content="刷新" placement="top" :show-after="500">
            <button class="wc-icon-btn" @click="loadTree">↺</button>
          </el-tooltip>
        </div>
      </div>

      <div class="wc-panel-body file-tree-body">
        <div v-if="treeLoading" class="wc-loading">
          <el-icon class="rotating"><Loading /></el-icon> 加载中…
        </div>
        <div v-else-if="!treeData.length" class="wc-empty">工作区为空</div>

        <!-- ── el-tree 文件树 ── -->
        <el-tree
          v-else
          ref="treeRef"
          :data="treeData"
          :props="treeProps"
          :highlight-current="true"
          :expand-on-click-node="false"
          :default-expand-all="false"
          node-key="path"
          class="wc-file-tree"
          @node-click="onNodeClick"
          @node-contextmenu="onNodeContextmenu"
        >
          <template #default="{ node, data }">
            <span class="tree-node" :class="{ 'is-file': !data.isDir, 'is-active': data.path === openFilePath }">
              <!-- 图标 -->
              <span class="tree-node-icon">
                <span v-if="data.isDir">{{ node.expanded ? '📂' : '📁' }}</span>
                <span v-else>{{ fileNodeIcon(data.name) }}</span>
              </span>
              <!-- 文件名（重命名模式时改为 input）-->
              <input
                v-if="renaming === data.path"
                v-model="renameValue"
                class="tree-rename-input"
                @keyup.enter="commitRename(data)"
                @keyup.escape="renaming = ''"
                @blur="commitRename(data)"
                @click.stop
                ref="renameInputRef"
              />
              <span v-else class="tree-node-name">{{ data.name }}</span>
              <!-- 文件大小 -->
              <span v-if="!data.isDir" class="tree-node-size">{{ fmtSize(data.size) }}</span>
              <!-- 悬停操作按钮 -->
              <span class="tree-node-actions" @click.stop>
                <button v-if="!data.isDir" class="tree-act-btn" title="重命名" @click="startRename(data)">✎</button>
                <button class="tree-act-btn danger" title="删除" @click="deleteNode(data)">✕</button>
              </span>
            </span>
          </template>
        </el-tree>
      </div>

      <!-- 右键菜单 -->
      <Teleport to="body">
        <div v-if="ctxMenu.visible" class="ctx-menu"
          :style="{ left: ctxMenu.x + 'px', top: ctxMenu.y + 'px' }"
          @mouseleave="ctxMenu.visible = false">
          <div class="ctx-item" @click="ctxNewFile">📄 新建文件</div>
          <div class="ctx-item" @click="ctxNewFolder">📁 新建文件夹</div>
          <div v-if="ctxMenu.node && !ctxMenu.node.isDir" class="ctx-item" @click="ctxRename">✎ 重命名</div>
          <div class="ctx-divider" />
          <div class="ctx-item danger" @click="ctxDelete">✕ 删除</div>
        </div>
      </Teleport>
    </div>

    <!-- ── 左分隔线 ── -->
    <div class="wc-handle" @mousedown="startResizeLeft" :class="{ dragging: draggingLeft }">
      <div class="wc-handle-bar" />
    </div>

    <!-- ── 中栏：编辑器 ── -->
    <div class="wc-panel wc-panel-mid" :style="{ width: midW + 'px' }">
      <div class="wc-panel-header">
        <span v-if="openFilePath" class="wc-panel-title file-path-title">
          <span class="file-ext-badge">{{ fileExt(openFilePath) }}</span>
          <span class="file-path-text" :title="openFilePath">{{ openFilePath }}</span>
        </span>
        <span v-else class="wc-panel-title muted">选择文件查看</span>
        <div class="wc-header-actions">
          <el-tag v-if="fileDirty" size="small" type="warning" style="margin-right:4px;">未保存</el-tag>
          <button v-if="openFilePath && fileDirty" class="wc-save-btn" @click="saveFile">保存</button>
          <button v-if="openFilePath" class="wc-icon-btn" title="刷新" @click="refreshFile">↺</button>
          <button v-if="openFilePath" class="wc-icon-btn danger" title="删除文件" @click="deleteFile">✕</button>
        </div>
      </div>

      <div class="wc-panel-body editor-body">
        <div v-if="!openFilePath" class="wc-empty-editor">
          <div class="wc-empty-icon">✏️</div>
          <div>从左侧选择文件</div>
          <div class="wc-empty-hint">支持拖拽文件到右侧聊天框</div>
        </div>
        <div v-else-if="fileBinary" class="wc-binary-notice">⛔ 二进制文件，无法编辑</div>
        <template v-else>
          <div class="editor-wrap">
            <div class="line-numbers" ref="lineNumRef">
              <div v-for="n in lineCount" :key="n" class="line-num">{{ n }}</div>
            </div>
            <textarea
              ref="editorRef"
              v-model="fileContent"
              class="code-editor"
              spellcheck="false"
              autocorrect="off"
              autocapitalize="off"
              @input="fileDirty = true; syncScroll()"
              @scroll="syncScroll"
              @keydown.tab.prevent="insertTab"
              @keydown.ctrl.s.prevent="saveFile"
              @keydown.meta.s.prevent="saveFile"
            />
          </div>
          <div class="editor-statusbar">
            <span class="stat-chip">{{ fileExt(openFilePath) }}</span>
            <span>{{ lineCount }} 行</span>
            <span>{{ fileContent.length }} 字符</span>
            <span v-if="fileInfo">{{ formatSize(fileInfo.size) }}</span>
            <span class="stat-flex" />
            <span v-if="fileDirty" class="status-dirty">● 未保存</span>
            <span v-else class="status-saved">✓ 已保存</span>
          </div>
        </template>
      </div>
    </div>

    <!-- ── 右分隔线 ── -->
    <div class="wc-handle" @mousedown="startResizeRight" :class="{ dragging: draggingRight }">
      <div class="wc-handle-bar" />
    </div>

    <!-- ── 右栏：AI 对话（含历史会话选择）── -->
    <div class="wc-panel wc-panel-right">
      <!-- 会话历史选择栏 -->
      <div class="session-bar">
        <div class="session-bar-left">
          <el-icon style="color:#64748b; font-size:13px;"><ChatDotRound /></el-icon>
          <el-select
            v-model="currentSessionId"
            placeholder="新对话"
            size="small"
            clearable
            class="session-select"
            @change="onSessionSelect"
          >
            <el-option
              v-for="s in sessionList"
              :key="s.id"
              :value="s.id"
              :label="s.title || ('对话 ' + s.id.slice(0, 8))"
            >
              <div class="session-opt">
                <span class="session-opt-title">{{ s.title || '无标题' }}</span>
                <span class="session-opt-time">{{ fmtTs(s.lastAt) }}</span>
              </div>
            </el-option>
          </el-select>
        </div>
        <button class="session-new-btn" title="新建对话" @click="newSession">＋ 新对话</button>
      </div>

      <!-- AiChat 占满剩余高度 -->
      <div class="chat-area">
        <AiChat
          :agent-id="agentId"
          :session-id="currentSessionId || undefined"
          :context="chatContext"
          height="100%"
          ref="chatRef"
          @response="onChatResponse"
          @session-change="onSessionCreated"
        />
      </div>
    </div>

    <!-- ── 新建文件 Modal ── -->
    <Teleport to="body">
      <div v-if="showNewFile" class="wc-modal-mask" @click.self="showNewFile = false">
        <div class="wc-modal">
          <div class="wc-modal-title">新建文件</div>
          <input v-model="newFilePath" class="wc-modal-input"
            placeholder="如 notes.md 或 scripts/run.sh"
            @keyup.enter="createFile"
            ref="newFileInput" />
          <div class="wc-modal-footer">
            <button class="wc-btn" @click="showNewFile = false">取消</button>
            <button class="wc-btn primary" @click="createFile">创建</button>
          </div>
        </div>
      </div>
    </Teleport>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Loading, ChatDotRound } from '@element-plus/icons-vue'
import { files as filesApi, sessions as sessionsApi, type SessionSummary } from '../api'
import AiChat from './AiChat.vue'

// ── Props & Emits ──────────────────────────────────────────────────────────
const props = defineProps<{ agentId: string }>()
const emit  = defineEmits<{ (e: 'session-change', id: string): void }>()

// ── Tree node type ─────────────────────────────────────────────────────────
interface FNode { name: string; path: string; isDir: boolean; size?: number; children?: FNode[] }

// el-tree props mapping
const treeProps = { label: 'name', children: 'children', isLeaf: (d: FNode) => !d.isDir }

// ── Panel sizes (px) ──────────────────────────────────────────────────────
const leftW = ref(200)
const midW  = ref(460)
const MIN_W = 140; const MAX_LEFT = 380; const MAX_MID = 900

// ── Refs ──────────────────────────────────────────────────────────────────
const treeRef        = ref<any>()
const editorRef      = ref<HTMLTextAreaElement>()
const lineNumRef     = ref<HTMLElement>()
const newFileInput   = ref<HTMLInputElement>()
const renameInputRef = ref<HTMLInputElement>()
const chatRef        = ref<InstanceType<typeof AiChat>>()

// ── File tree state ────────────────────────────────────────────────────────
const treeData     = ref<FNode[]>([])
const treeLoading  = ref(false)
const openFilePath = ref('')
const fileContent  = ref('')
const fileDirty    = ref(false)
const fileBinary   = ref(false)
const fileInfo     = ref<{ size: number; modTime: string } | null>(null)
const showNewFile  = ref(false)
const newFilePath  = ref('')

// Rename
const renaming      = ref('')
const renameValue   = ref('')

// Context menu
const ctxMenu = ref({ visible: false, x: 0, y: 0, node: null as FNode | null })

// ── Session state ──────────────────────────────────────────────────────────
const sessionList      = ref<SessionSummary[]>([])
const currentSessionId = ref<string | undefined>()

// ── Resize ────────────────────────────────────────────────────────────────
let resStartX = 0, resStartW = 0, resSide: 'left'|'right'|null = null
const draggingLeft  = ref(false)
const draggingRight = ref(false)

function startResizeLeft(e: MouseEvent)  { startResize(e, 'left') }
function startResizeRight(e: MouseEvent) { startResize(e, 'right') }
function startResize(e: MouseEvent, side: 'left'|'right') {
  resStartX = e.clientX
  resStartW = side === 'left' ? leftW.value : midW.value
  resSide = side
  side === 'left' ? draggingLeft.value = true : draggingRight.value = true
  window.addEventListener('mousemove', onResize)
  window.addEventListener('mouseup', stopResize)
  document.body.style.cssText += 'cursor:col-resize;user-select:none;'
}
function onResize(e: MouseEvent) {
  const d = e.clientX - resStartX
  if (resSide === 'left') leftW.value = Math.max(MIN_W, Math.min(MAX_LEFT, resStartW + d))
  else if (resSide === 'right') midW.value = Math.max(MIN_W, Math.min(MAX_MID, resStartW + d))
}
function stopResize() {
  draggingLeft.value = false; draggingRight.value = false; resSide = null
  window.removeEventListener('mousemove', onResize)
  window.removeEventListener('mouseup', stopResize)
  document.body.style.cursor = ''; document.body.style.userSelect = ''
}

// ── File tree ─────────────────────────────────────────────────────────────
async function loadTree() {
  treeLoading.value = true
  try {
    const res = await filesApi.readTree(props.agentId)
    treeData.value = buildTree(res.data)
  } catch { treeData.value = [] }
  finally { treeLoading.value = false }
}

function buildTree(data: any): FNode[] {
  const arr: any[] = Array.isArray(data) ? data : data?.children ?? []
  return arr.map((item: any) => ({
    name: item.name,
    path: item.path ?? item.name,
    isDir: !!(item.isDir ?? item.type === 'dir'),
    size: item.size,
    children: item.children?.length ? buildTree(item.children) : (item.isDir ? [] : undefined),
  })).sort((a, b) => (+b.isDir - +a.isDir) || a.name.localeCompare(b.name))
}

function onNodeClick(data: FNode) {
  ctxMenu.value.visible = false
  if (!data.isDir) openFile(data.path)
}

function onNodeContextmenu(e: MouseEvent, data: FNode) {
  e.preventDefault()
  ctxMenu.value = { visible: true, x: e.clientX, y: e.clientY, node: data }
}

async function openFile(path: string) {
  if (fileDirty.value && openFilePath.value) {
    const ok = await ElMessageBox.confirm('有未保存更改，继续切换？', '提示', {
      confirmButtonText: '继续', cancelButtonText: '取消', type: 'warning',
    }).then(() => true).catch(() => false)
    if (!ok) return
  }
  openFilePath.value = path
  fileDirty.value = false
  nextTick(() => treeRef.value?.setCurrentKey(path))
  await refreshFile()
}

async function refreshFile() {
  if (!openFilePath.value) return
  try {
    const res = await filesApi.read(props.agentId, openFilePath.value)
    const d = res.data
    fileBinary.value = d.binary ?? d.encoding === 'base64'
    if (!fileBinary.value) {
      fileContent.value = d.content ?? ''
      fileInfo.value = d.size != null ? { size: d.size, modTime: d.modTime } : null
    }
    fileDirty.value = false
  } catch { fileContent.value = '' }
}

async function saveFile() {
  if (!openFilePath.value || fileBinary.value) return
  try {
    await filesApi.write(props.agentId, openFilePath.value, fileContent.value)
    fileDirty.value = false
    ElMessage.success('已保存')
  } catch { ElMessage.error('保存失败') }
}

async function deleteFile() {
  if (!openFilePath.value) return
  const ok = await ElMessageBox.confirm(`删除 ${openFilePath.value}？`, '确认', {
    confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning',
  }).then(() => true).catch(() => false)
  if (!ok) return
  try {
    await filesApi.delete(props.agentId, openFilePath.value)
    openFilePath.value = ''; fileContent.value = ''; fileDirty.value = false
    await loadTree(); ElMessage.success('已删除')
  } catch { ElMessage.error('删除失败') }
}

async function createFile() {
  const p = newFilePath.value.trim()
  if (!p) return
  try {
    await filesApi.write(props.agentId, p, '')
    showNewFile.value = false; newFilePath.value = ''
    await loadTree(); await openFile(p)
  } catch { ElMessage.error('创建失败') }
}

// ── Delete node (from hover button) ──────────────────────────────────────
async function deleteNode(data: FNode) {
  const ok = await ElMessageBox.confirm(`删除 ${data.path}？`, '确认', {
    confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning',
  }).then(() => true).catch(() => false)
  if (!ok) return
  try {
    await filesApi.delete(props.agentId, data.path)
    if (openFilePath.value === data.path) { openFilePath.value = ''; fileContent.value = '' }
    await loadTree(); ElMessage.success('已删除')
  } catch { ElMessage.error('删除失败') }
}

// ── Rename ────────────────────────────────────────────────────────────────
function startRename(data: FNode) {
  renaming.value = data.path
  renameValue.value = data.name
  nextTick(() => renameInputRef.value?.focus())
}
async function commitRename(data: FNode) {
  if (!renaming.value || renameValue.value === data.name) { renaming.value = ''; return }
  const dir = data.path.includes('/') ? data.path.substring(0, data.path.lastIndexOf('/') + 1) : ''
  const newPath = dir + renameValue.value
  try {
    // Read content → write to new path → delete old
    const res = await filesApi.read(props.agentId, data.path)
    await filesApi.write(props.agentId, newPath, res.data?.content ?? '')
    await filesApi.delete(props.agentId, data.path)
    if (openFilePath.value === data.path) openFilePath.value = newPath
    await loadTree()
    ElMessage.success('已重命名')
  } catch { ElMessage.error('重命名失败') }
  renaming.value = ''
}

// ── Context menu actions ──────────────────────────────────────────────────
function ctxNewFile()   { ctxMenu.value.visible = false; showNewFile.value = true }
function ctxNewFolder() { ctxMenu.value.visible = false; showNewFile.value = true }
function ctxRename()    { const n = ctxMenu.value.node; ctxMenu.value.visible = false; if (n) startRename(n) }
async function ctxDelete() {
  const node = ctxMenu.value.node; ctxMenu.value.visible = false
  if (!node) return
  const ok = await ElMessageBox.confirm(`删除 ${node.path}？`, '确认', {
    confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning',
  }).then(() => true).catch(() => false)
  if (!ok) return
  try {
    await filesApi.delete(props.agentId, node.path)
    if (openFilePath.value === node.path) { openFilePath.value = ''; fileContent.value = '' }
    await loadTree(); ElMessage.success('已删除')
  } catch { ElMessage.error('删除失败') }
}

// Close ctx menu on any click
function onDocClick() { if (ctxMenu.value.visible) ctxMenu.value.visible = false }

// ── Session history ────────────────────────────────────────────────────────
async function loadSessions() {
  try {
    const res = await sessionsApi.list({ agentId: props.agentId, limit: 50 })
    sessionList.value = (res.data?.sessions || []).sort((a, b) => b.lastAt - a.lastAt)
  } catch { sessionList.value = [] }
}

function onSessionSelect(sid: string | undefined) {
  currentSessionId.value = sid || undefined
  if (sid) {
    chatRef.value?.resumeSession(sid)
  } else {
    chatRef.value?.startNewSession()
  }
}

function newSession() {
  currentSessionId.value = undefined
  chatRef.value?.startNewSession()
}

function onSessionCreated(sid: string) {
  currentSessionId.value = sid
  emit('session-change', sid)
  // Reload session list so the new session appears
  loadSessions()
}

// ── Chat → Editor sync ────────────────────────────────────────────────────
async function onChatResponse() {
  await loadTree()
  if (openFilePath.value) {
    await new Promise(r => setTimeout(r, 300))
    await refreshFile()
  }
}

// ── Editor helpers ─────────────────────────────────────────────────────────
const lineCount = computed(() => (fileContent.value.match(/\n/g) ?? []).length + 1)
const chatContext = computed(() =>
  openFilePath.value ? `用户当前打开的文件: ${openFilePath.value}` : undefined
)

function insertTab(_e: KeyboardEvent) {
  const ta = editorRef.value!
  const s = ta.selectionStart
  fileContent.value = ta.value.slice(0, s) + '  ' + ta.value.slice(ta.selectionEnd)
  nextTick(() => { ta.selectionStart = ta.selectionEnd = s + 2 })
}
function syncScroll() {
  if (lineNumRef.value && editorRef.value) lineNumRef.value.scrollTop = editorRef.value.scrollTop
}

// ── Watch showNewFile ──────────────────────────────────────────────────────
watch(showNewFile, async v => { if (v) { await nextTick(); newFileInput.value?.focus() } })

// ── Utils ──────────────────────────────────────────────────────────────────
function fileExt(path: string): string {
  const ext = path.split('.').pop()?.toLowerCase()
  return ext ? `.${ext}` : 'txt'
}
function fmtSize(bytes?: number): string {
  if (!bytes) return ''
  if (bytes < 1024) return `${bytes}B`
  if (bytes < 1048576) return `${(bytes / 1024).toFixed(0)}K`
  return `${(bytes / 1048576).toFixed(1)}M`
}
function formatSize(bytes: number): string { return fmtSize(bytes) ?? '' }

const FILE_ICONS: Record<string, string> = {
  go:'🐹', js:'📜', ts:'📘', tsx:'📘', jsx:'📜', vue:'💚', py:'🐍',
  md:'📝', markdown:'📝', json:'📋', yaml:'⚙️', yml:'⚙️', toml:'⚙️',
  sh:'⚡', bash:'⚡', html:'🌐', css:'🎨', scss:'🎨',
  sql:'🗄️', rs:'🦀', txt:'📄', env:'🔒', dockerfile:'🐳',
  png:'🖼️', jpg:'🖼️', jpeg:'🖼️', gif:'🖼️', svg:'🖼️', webp:'🖼️',
}
function fileNodeIcon(name: string): string {
  const ext = name.split('.').pop()?.toLowerCase() ?? ''
  return FILE_ICONS[ext] ?? (name.startsWith('.') ? '🔒' : '📄')
}

function fmtTs(ms: number): string {
  if (!ms) return ''
  const d = new Date(ms)
  const now = Date.now()
  const diff = now - ms
  if (diff < 60000) return '刚刚'
  if (diff < 3600000) return `${Math.floor(diff / 60000)}分钟前`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)}小时前`
  return d.toLocaleDateString()
}

// ── Lifecycle ──────────────────────────────────────────────────────────────
onMounted(() => {
  loadTree()
  loadSessions()
  document.addEventListener('click', onDocClick)
})
onUnmounted(() => {
  stopResize()
  document.removeEventListener('click', onDocClick)
})
</script>

<style scoped>
/* ── Layout ── */
.wc-layout {
  display: flex;
  height: 100%;
  overflow: hidden;
  background: #f8fafc;
}
.wc-layout * { box-sizing: border-box; }

/* ── Panels ── */
.wc-panel {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: #fff;
  border-right: 1px solid #e2e8f0;
}
.wc-panel:last-child { border-right: none; }
.wc-panel-right { flex: 1; min-width: 280px; }

.wc-panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 8px 0 10px;
  height: 34px;
  border-bottom: 1px solid #e2e8f0;
  background: #f8fafc;
  flex-shrink: 0;
}
.wc-panel-title {
  font-size: 12px;
  font-weight: 600;
  color: #334155;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
  min-width: 0;
}
.wc-panel-title.muted { color: #94a3b8; font-weight: 400; }
.file-path-title { display: flex; align-items: center; gap: 5px; }
.file-path-text  { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.file-ext-badge  {
  background: #e2e8f0; color: #64748b; font-size: 10px; padding: 1px 5px;
  border-radius: 4px; font-family: monospace; flex-shrink: 0;
}
.wc-header-actions { display: flex; align-items: center; gap: 3px; flex-shrink: 0; }
.wc-icon-btn {
  padding: 2px 6px; background: transparent; border: 1px solid transparent;
  border-radius: 4px; cursor: pointer; font-size: 13px; color: #64748b;
}
.wc-icon-btn:hover { background: #e2e8f0; border-color: #cbd5e1; }
.wc-icon-btn.danger:hover { background: #fee2e2; color: #dc2626; border-color: #fca5a5; }
.wc-save-btn {
  padding: 2px 10px; background: #3b82f6; color: #fff;
  border: none; border-radius: 4px; cursor: pointer; font-size: 12px; font-weight: 500;
}
.wc-save-btn:hover { background: #2563eb; }
.wc-panel-body { flex: 1; overflow: hidden; position: relative; }

/* ── File tree (el-tree) ── */
.file-tree-body { overflow-y: auto; padding: 4px 0; }

:deep(.wc-file-tree) {
  background: transparent;
  font-size: 12px;
}
:deep(.wc-file-tree .el-tree-node__content) {
  height: 28px;
  padding-right: 4px;
  border-radius: 4px;
  margin: 0 4px;
}
:deep(.wc-file-tree .el-tree-node__content:hover) {
  background: #f1f5f9;
}
:deep(.wc-file-tree .el-tree-node.is-current > .el-tree-node__content) {
  background: #eff6ff;
  color: #2563eb;
}

.tree-node {
  display: flex;
  align-items: center;
  gap: 4px;
  flex: 1;
  min-width: 0;
  overflow: hidden;
  position: relative;
  padding-right: 50px; /* space for action buttons */
}
.tree-node-icon  { font-size: 13px; flex-shrink: 0; }
.tree-node-name  { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: #334155; }
.tree-node-size  { font-size: 10px; color: #94a3b8; flex-shrink: 0; }
.tree-node-actions {
  position: absolute;
  right: 0;
  display: none;
  gap: 2px;
}
.tree-node:hover .tree-node-actions { display: flex; }
.tree-act-btn {
  padding: 1px 4px; border: none; background: #e2e8f0; border-radius: 3px;
  cursor: pointer; font-size: 11px; color: #475569;
}
.tree-act-btn:hover { background: #cbd5e1; }
.tree-act-btn.danger:hover { background: #fee2e2; color: #dc2626; }
.tree-rename-input {
  flex: 1; border: 1px solid #3b82f6; border-radius: 4px; outline: none;
  padding: 1px 4px; font-size: 12px; min-width: 0;
}

.wc-loading { padding: 16px; font-size: 12px; color: #94a3b8; display: flex; align-items: center; gap: 6px; }
.wc-empty   { padding: 16px; font-size: 12px; color: #94a3b8; text-align: center; }
.rotating   { animation: spin .8s linear infinite; }
@keyframes spin { from { transform: rotate(0) } to { transform: rotate(360deg) } }

/* ── Context menu ── */
.ctx-menu {
  position: fixed;
  z-index: 9999;
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 4px;
  box-shadow: 0 8px 24px rgba(0,0,0,.12);
  min-width: 140px;
}
.ctx-item {
  padding: 7px 12px;
  font-size: 12px;
  color: #334155;
  border-radius: 4px;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 6px;
}
.ctx-item:hover { background: #f1f5f9; }
.ctx-item.danger { color: #dc2626; }
.ctx-item.danger:hover { background: #fee2e2; }
.ctx-divider { height: 1px; background: #e2e8f0; margin: 3px 0; }

/* ── Editor ── */
.editor-body { display: flex; flex-direction: column; }
.editor-wrap { flex: 1; display: flex; overflow: hidden; }
.line-numbers {
  width: 42px; background: #f8fafc; border-right: 1px solid #e2e8f0;
  padding: 8px 0; overflow: hidden; flex-shrink: 0; user-select: none;
}
.line-num {
  height: 19px; text-align: right; padding-right: 8px;
  font-size: 11px; font-family: monospace; color: #94a3b8; line-height: 19px;
}
.code-editor {
  flex: 1; padding: 8px 12px;
  font-family: 'Menlo','Monaco','Courier New',monospace;
  font-size: 13px; line-height: 19px;
  background: #fff; color: #1e293b;
  border: none; outline: none; resize: none;
  overflow-y: auto; overflow-x: auto;
  white-space: pre; tab-size: 2; caret-color: #3b82f6;
}
.editor-statusbar {
  display: flex; gap: 10px; align-items: center;
  padding: 3px 10px; font-size: 11px; color: #94a3b8;
  background: #f8fafc; border-top: 1px solid #e2e8f0; flex-shrink: 0;
}
.stat-chip  { background: #e2e8f0; border-radius: 3px; padding: 1px 5px; font-family: monospace; color: #64748b; }
.stat-flex  { flex: 1; }
.status-dirty { color: #f59e0b; font-weight: 600; }
.status-saved { color: #22c55e; }
.wc-empty-editor {
  display: flex; flex-direction: column; align-items: center; justify-content: center;
  height: 100%; color: #94a3b8; font-size: 14px; gap: 10px;
}
.wc-empty-icon { font-size: 40px; }
.wc-empty-hint { font-size: 12px; color: #cbd5e1; }
.wc-binary-notice {
  display: flex; align-items: center; justify-content: center;
  height: 100%; color: #94a3b8; font-size: 13px;
}

/* ── Resize handle ── */
.wc-handle {
  width: 4px; background: #e2e8f0; cursor: col-resize; flex-shrink: 0;
  display: flex; align-items: center; justify-content: center;
  transition: background .15s;
}
.wc-handle:hover, .wc-handle.dragging { background: #3b82f6; }
.wc-handle-bar { width: 2px; height: 32px; background: rgba(255,255,255,.5); border-radius: 2px; }

/* ── Session selector bar ── */
.session-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 8px;
  border-bottom: 1px solid #e2e8f0;
  background: #f8fafc;
  flex-shrink: 0;
  gap: 6px;
  height: 38px;
}
.session-bar-left { display: flex; align-items: center; gap: 6px; flex: 1; min-width: 0; }
.session-select { flex: 1; }
:deep(.session-select .el-input__wrapper) {
  font-size: 12px; padding: 0 8px; background: #fff;
  box-shadow: 0 0 0 1px #e2e8f0 inset;
}
.session-opt { display: flex; justify-content: space-between; gap: 8px; align-items: center; }
.session-opt-title { font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1; color: #334155; }
.session-opt-time  { font-size: 11px; color: #94a3b8; flex-shrink: 0; }
.session-new-btn {
  padding: 3px 10px; background: #f1f5f9; color: #334155;
  border: 1px solid #e2e8f0; border-radius: 6px; cursor: pointer; font-size: 12px; white-space: nowrap;
}
.session-new-btn:hover { background: #e2e8f0; border-color: #cbd5e1; }

.chat-area { flex: 1; overflow: hidden; }

/* ── Modal ── */
.wc-modal-mask {
  position: fixed; inset: 0; background: rgba(0,0,0,.4);
  z-index: 1000; display: flex; align-items: center; justify-content: center;
}
.wc-modal {
  background: #fff; border-radius: 12px; padding: 20px 24px;
  width: 380px; box-shadow: 0 20px 60px rgba(0,0,0,.15);
}
.wc-modal-title  { font-size: 15px; font-weight: 600; color: #1e293b; margin-bottom: 14px; }
.wc-modal-input  {
  width: 100%; padding: 8px 12px; border: 1px solid #e2e8f0; border-radius: 6px;
  font-size: 13px; outline: none; margin-bottom: 14px; transition: border-color .15s;
}
.wc-modal-input:focus { border-color: #3b82f6; }
.wc-modal-footer { display: flex; gap: 8px; justify-content: flex-end; }
.wc-btn         { padding: 6px 16px; border-radius: 6px; border: 1px solid #e2e8f0; background: #f8fafc; color: #334155; font-size: 13px; cursor: pointer; }
.wc-btn.primary { background: #3b82f6; color: #fff; border-color: #3b82f6; }
.wc-btn:hover   { opacity: .85; }
</style>
