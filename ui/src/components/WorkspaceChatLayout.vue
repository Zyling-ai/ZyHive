<template>
  <div class="wc-layout">

    <!-- ── 左栏：文件树 ── -->
    <div class="wc-panel wc-panel-left" :style="{ width: leftW + 'px' }">
      <div class="wc-panel-header">
        <span class="wc-panel-title">
          <span style="font-size:12px; margin-right:4px;">📁</span>工作区
        </span>
        <div class="wc-header-actions">
          <button class="wc-icon-btn" title="新建文件" @click="showNewFile = true">＋</button>
          <button class="wc-icon-btn" title="刷新" @click="loadTree">↺</button>
        </div>
      </div>
      <div class="wc-panel-body file-tree-body">
        <div v-if="treeLoading" class="wc-loading">加载中…</div>
        <div v-else-if="!treeData.length" class="wc-empty">工作区为空</div>
        <FileTreeNode
          v-for="node in treeData" :key="node.path"
          :node="node"
          :active-path="openFilePath"
          :depth="0"
          @open="openFile"
        />
      </div>
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
          {{ openFilePath }}
        </span>
        <span v-else class="wc-panel-title muted">选择文件查看</span>
        <div class="wc-header-actions">
          <button v-if="openFilePath && fileDirty" class="wc-save-btn" @click="saveFile">保存</button>
          <button v-if="openFilePath" class="wc-icon-btn" title="刷新文件" @click="refreshFile">↺</button>
          <button v-if="openFilePath" class="wc-icon-btn danger" title="删除文件" @click="deleteFile">✕</button>
        </div>
      </div>

      <div class="wc-panel-body editor-body">
        <div v-if="!openFilePath" class="wc-empty-editor">
          <div class="wc-empty-icon">✏️</div>
          <div>从左侧选择文件</div>
          <div class="wc-empty-hint">支持拖拽文件到聊天框</div>
        </div>
        <div v-else-if="fileBinary" class="wc-binary-notice">
          二进制文件，无法编辑
        </div>
        <template v-else>
          <!-- 行号 + 编辑器 -->
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
          <!-- 底部状态栏 -->
          <div class="editor-statusbar">
            <span>{{ fileExt(openFilePath) }}</span>
            <span>{{ lineCount }} 行</span>
            <span>{{ fileContent.length }} 字符</span>
            <span v-if="fileInfo">{{ formatSize(fileInfo.size) }}</span>
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

    <!-- ── 右栏：AI 对话 ── -->
    <div class="wc-panel wc-panel-right">
      <AiChat
        :agent-id="agentId"
        :session-id="sessionId"
        :context="chatContext"
        height="100%"
        ref="chatRef"
        @response="onChatResponse"
        @session-change="(sid) => $emit('session-change', sid)"
      />
    </div>

    <!-- 新建文件对话框 -->
    <div v-if="showNewFile" class="wc-modal-mask" @click.self="showNewFile = false">
      <div class="wc-modal">
        <div class="wc-modal-title">新建文件</div>
        <input v-model="newFilePath" class="wc-modal-input" placeholder="如 notes.md 或 scripts/run.sh"
          @keyup.enter="createFile" ref="newFileInput" />
        <div class="wc-modal-footer">
          <button class="wc-btn" @click="showNewFile = false">取消</button>
          <button class="wc-btn primary" @click="createFile">创建</button>
        </div>
      </div>
    </div>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onUnmounted, defineComponent, h } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { files as filesApi } from '../api'
import AiChat from './AiChat.vue'

// ── Props ──────────────────────────────────────────────────────────────────
interface Props {
  agentId: string
  sessionId?: string
}
const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'session-change', sessionId: string): void
}>()

// ── File tree node type ───────────────────────────────────────────────────
interface TreeNode {
  name: string
  path: string
  isDir: boolean
  size?: number
  children?: TreeNode[]
  _open?: boolean
}

// ── Panel widths (px) ─────────────────────────────────────────────────────
const leftW = ref(220)
const midW  = ref(480)
const MIN_W = 140
const MAX_LEFT = 400
const MAX_MID  = 900

// ── State ─────────────────────────────────────────────────────────────────
const editorRef    = ref<HTMLTextAreaElement>()
const lineNumRef   = ref<HTMLElement>()
const newFileInput = ref<HTMLInputElement>()

const treeData    = ref<TreeNode[]>([])
const treeLoading = ref(false)
const openFilePath = ref('')
const fileContent  = ref('')
const fileDirty    = ref(false)
const fileBinary   = ref(false)
const fileInfo     = ref<{ size: number; modTime: string } | null>(null)
const showNewFile  = ref(false)
const newFilePath  = ref('')
const draggingLeft  = ref(false)
const draggingRight = ref(false)

const lineCount = computed(() => {
  const n = (fileContent.value.match(/\n/g) ?? []).length + 1
  return n
})

const chatContext = computed(() =>
  openFilePath.value
    ? `用户当前打开的文件: ${openFilePath.value}\n（修改后会实时刷新编辑器）`
    : undefined
)

// ── Resize ────────────────────────────────────────────────────────────────
let resideStartX = 0
let resizeStartW = 0
let resizeTarget: 'left' | 'right' | null = null

function startResizeLeft(e: MouseEvent) {
  resideStartX = e.clientX
  resizeStartW = leftW.value
  resizeTarget = 'left'
  draggingLeft.value = true
  window.addEventListener('mousemove', onResize)
  window.addEventListener('mouseup', stopResize)
  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'
}

function startResizeRight(e: MouseEvent) {
  resideStartX = e.clientX
  resizeStartW = midW.value
  resizeTarget = 'right'
  draggingRight.value = true
  window.addEventListener('mousemove', onResize)
  window.addEventListener('mouseup', stopResize)
  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'
}

function onResize(e: MouseEvent) {
  const delta = e.clientX - resideStartX
  if (resizeTarget === 'left') {
    leftW.value = Math.max(MIN_W, Math.min(MAX_LEFT, resizeStartW + delta))
  } else if (resizeTarget === 'right') {
    midW.value = Math.max(MIN_W, Math.min(MAX_MID, resizeStartW + delta))
  }
}

function stopResize() {
  draggingLeft.value = false
  draggingRight.value = false
  resizeTarget = null
  window.removeEventListener('mousemove', onResize)
  window.removeEventListener('mouseup', stopResize)
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
}

// ── File tree ─────────────────────────────────────────────────────────────
async function loadTree() {
  treeLoading.value = true
  try {
    const res = await filesApi.readTree(props.agentId)
    treeData.value = buildTree(res.data)
  } catch {
    treeData.value = []
  } finally {
    treeLoading.value = false
  }
}

function buildTree(data: any): TreeNode[] {
  if (!data) return []
  // If array of nodes
  if (Array.isArray(data)) {
    return data.map((item: any) => ({
      name: item.name,
      path: item.path ?? item.name,
      isDir: item.isDir ?? item.type === 'dir',
      size: item.size,
      children: item.children ? buildTree(item.children) : undefined,
    })).sort((a, b) => (b.isDir ? 1 : 0) - (a.isDir ? 1 : 0) || a.name.localeCompare(b.name))
  }
  // If object with children/files
  if (data.children) return buildTree(data.children)
  return []
}

async function openFile(path: string) {
  if (fileDirty.value && openFilePath.value) {
    const ok = await ElMessageBox.confirm('有未保存的更改，切换文件将丢失。继续？', '提示', {
      confirmButtonText: '继续', cancelButtonText: '取消', type: 'warning',
    }).then(() => true).catch(() => false)
    if (!ok) return
  }
  openFilePath.value = path
  fileDirty.value = false
  await refreshFile()
}

async function refreshFile() {
  if (!openFilePath.value) return
  try {
    const res = await filesApi.read(props.agentId, openFilePath.value)
    const data = res.data
    fileBinary.value = data.binary ?? false
    if (!fileBinary.value) {
      fileContent.value = data.content ?? ''
      fileInfo.value = data.size != null ? { size: data.size, modTime: data.modTime } : null
    }
    fileDirty.value = false
  } catch {
    fileContent.value = ''
  }
}

async function saveFile() {
  if (!openFilePath.value || fileBinary.value) return
  try {
    await filesApi.write(props.agentId, openFilePath.value, fileContent.value)
    fileDirty.value = false
    ElMessage.success('已保存')
  } catch {
    ElMessage.error('保存失败')
  }
}

async function deleteFile() {
  const ok = await ElMessageBox.confirm(`删除 ${openFilePath.value}？`, '确认删除', {
    confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning',
  }).then(() => true).catch(() => false)
  if (!ok) return
  try {
    await filesApi.delete(props.agentId, openFilePath.value)
    openFilePath.value = ''
    fileContent.value = ''
    fileDirty.value = false
    await loadTree()
    ElMessage.success('已删除')
  } catch {
    ElMessage.error('删除失败')
  }
}

async function createFile() {
  if (!newFilePath.value.trim()) return
  try {
    await filesApi.write(props.agentId, newFilePath.value.trim(), '')
    showNewFile.value = false
    await loadTree()
    await openFile(newFilePath.value.trim())
    newFilePath.value = ''
  } catch {
    ElMessage.error('创建失败')
  }
}

// Watch showNewFile to focus input
watch(showNewFile, async (v) => {
  if (v) {
    await nextTick()
    newFileInput.value?.focus()
  }
})

// ── Chat ↔ Editor sync ─────────────────────────────────────────────────────
// After each AI response, refresh the tree + current file (AI may have written it)
async function onChatResponse() {
  await loadTree()
  if (openFilePath.value) {
    // Small delay to let server flush writes
    await new Promise(r => setTimeout(r, 300))
    await refreshFile()
  }
}

// ── Editor helpers ─────────────────────────────────────────────────────────
function insertTab(_e: KeyboardEvent) {
  const ta = editorRef.value!
  const s = ta.selectionStart
  const v = ta.value
  fileContent.value = v.slice(0, s) + '  ' + v.slice(ta.selectionEnd)
  nextTick(() => { ta.selectionStart = ta.selectionEnd = s + 2 })
}

function syncScroll() {
  if (lineNumRef.value && editorRef.value) {
    lineNumRef.value.scrollTop = editorRef.value.scrollTop
  }
}

// ── Utils ──────────────────────────────────────────────────────────────────
function fileExt(path: string): string {
  const ext = path.split('.').pop()?.toLowerCase()
  return ext ? `.${ext}` : 'txt'
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes}B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)}KB`
  return `${(bytes / 1024 / 1024).toFixed(1)}MB`
}

onMounted(() => loadTree())
onUnmounted(() => stopResize())

// ── FileTreeNode inline component (recursive) ──────────────────────────────
const FileTreeNode = defineComponent({
  name: 'FileTreeNode',
  props: {
    node: Object as () => any,
    activePath: String,
    depth: { type: Number, default: 0 },
  },
  emits: ['open'],
  setup(props, { emit }) {
    const open = ref(false)
    function toggle() {
      if (props.node.isDir) open.value = !open.value
      else emit('open', props.node.path)
    }
    return () => h('div', { class: 'tree-item-wrap' }, [
      h('div', {
        class: ['tree-item', props.node.isDir && 'is-dir',
          !props.node.isDir && props.node.path === props.activePath && 'is-active'],
        style: { paddingLeft: (props.depth * 14 + 10) + 'px' },
        onClick: toggle,
      }, [
        h('span', { class: 'tree-icon' }, props.node.isDir ? (open.value ? '📂' : '📁') : fileIcon(props.node.name)),
        h('span', { class: 'tree-name' }, props.node.name),
        !props.node.isDir && props.node.size != null
          ? h('span', { class: 'tree-size' }, fmtSz(props.node.size))
          : null,
      ]),
      props.node.isDir && open.value && props.node.children?.length
        ? h('div', { class: 'tree-children' },
            props.node.children.map((child: any) => h(FileTreeNode, {
              node: child, activePath: props.activePath, depth: props.depth + 1,
              onOpen: (p: string) => emit('open', p),
            }))
          )
        : null,
    ])
  },
})

function fileIcon(name: string): string {
  const ext = name.split('.').pop()?.toLowerCase() ?? ''
  const m: Record<string, string> = {
    go:'🐹', js:'📜', ts:'📘', vue:'💚', py:'🐍', md:'📝', json:'📋',
    sh:'⚡', yaml:'⚙️', yml:'⚙️', toml:'⚙️', html:'🌐', css:'🎨',
    sql:'🗄️', rs:'🦀', txt:'📄', env:'🔒',
  }
  return m[ext] ?? '📄'
}
function fmtSz(b: number): string {
  return b < 1024 ? `${b}B` : b < 1048576 ? `${(b/1024).toFixed(0)}K` : `${(b/1048576).toFixed(1)}M`
}
// FileTreeNode is available via script setup auto-imports in this SFC
</script>

<style scoped>
/* ── Layout ── */
.wc-layout {
  display: flex;
  height: 100%;
  overflow: hidden;
  background: #f8fafc;
  user-select: none; /* prevent text selection during resize */
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
.wc-panel-right {
  flex: 1;
  min-width: 260px;
  border-right: none;
}

.wc-panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 10px;
  height: 36px;
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
.file-ext-badge {
  display: inline-block;
  background: #e2e8f0;
  color: #64748b;
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 4px;
  margin-right: 5px;
  font-family: monospace;
}
.wc-header-actions { display: flex; gap: 3px; flex-shrink: 0; }
.wc-icon-btn {
  padding: 2px 6px;
  background: transparent;
  border: 1px solid transparent;
  border-radius: 4px;
  cursor: pointer;
  font-size: 13px;
  color: #64748b;
  transition: all .15s;
}
.wc-icon-btn:hover { background: #e2e8f0; border-color: #cbd5e1; }
.wc-icon-btn.danger:hover { background: #fee2e2; color: #dc2626; border-color: #fca5a5; }
.wc-save-btn {
  padding: 2px 10px;
  background: #3b82f6;
  color: #fff;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  font-size: 12px;
  font-weight: 500;
}
.wc-save-btn:hover { background: #2563eb; }

.wc-panel-body {
  flex: 1;
  overflow: hidden;
  position: relative;
}

/* ── File tree ── */
.file-tree-body { overflow-y: auto; padding: 4px 0; user-select: none; }
.tree-item-wrap { }
.tree-item {
  display: flex;
  align-items: center;
  gap: 5px;
  padding: 3px 10px;
  cursor: pointer;
  font-size: 12px;
  color: #334155;
  border-radius: 4px;
  margin: 0 4px;
  transition: background .1s;
  white-space: nowrap;
}
.tree-item:hover { background: #f1f5f9; }
.tree-item.is-active { background: #eff6ff; color: #2563eb; }
.tree-item.is-dir { font-weight: 500; }
.tree-icon  { flex-shrink: 0; font-size: 13px; }
.tree-name  { flex: 1; overflow: hidden; text-overflow: ellipsis; }
.tree-size  { font-size: 10px; color: #94a3b8; flex-shrink: 0; }
.tree-children { }

/* ── Editor ── */
.editor-body { display: flex; flex-direction: column; }
.editor-wrap { flex: 1; display: flex; overflow: hidden; }
.line-numbers {
  width: 40px;
  background: #f8fafc;
  border-right: 1px solid #e2e8f0;
  padding: 8px 0;
  overflow: hidden;
  flex-shrink: 0;
  user-select: none;
}
.line-num {
  height: 19px;
  text-align: right;
  padding-right: 8px;
  font-size: 11px;
  font-family: 'Menlo', 'Monaco', monospace;
  color: #94a3b8;
  line-height: 19px;
}
.code-editor {
  flex: 1;
  padding: 8px 12px;
  font-family: 'Menlo', 'Monaco', 'Courier New', monospace;
  font-size: 13px;
  line-height: 19px;
  background: #fff;
  color: #1e293b;
  border: none;
  outline: none;
  resize: none;
  overflow-y: auto;
  white-space: pre;
  overflow-x: auto;
  tab-size: 2;
  caret-color: #3b82f6;
}
.code-editor:focus { background: #fafafa; }

.editor-statusbar {
  display: flex;
  gap: 12px;
  align-items: center;
  padding: 3px 10px;
  font-size: 11px;
  color: #94a3b8;
  background: #f8fafc;
  border-top: 1px solid #e2e8f0;
  flex-shrink: 0;
}
.status-dirty { color: #f59e0b; font-weight: 500; }
.status-saved { color: #22c55e; }

/* ── Resize handle ── */
.wc-handle {
  width: 4px;
  background: #e2e8f0;
  cursor: col-resize;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background .15s;
  position: relative;
  z-index: 10;
}
.wc-handle:hover, .wc-handle.dragging { background: #3b82f6; }
.wc-handle-bar {
  width: 2px;
  height: 32px;
  background: rgba(255,255,255,.5);
  border-radius: 2px;
}

/* ── Empty / loading states ── */
.wc-loading, .wc-empty {
  padding: 16px;
  font-size: 12px;
  color: #94a3b8;
  text-align: center;
}
.wc-empty-editor {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: #94a3b8;
  font-size: 14px;
  gap: 10px;
}
.wc-empty-icon { font-size: 40px; }
.wc-empty-hint { font-size: 12px; color: #cbd5e1; }
.wc-binary-notice {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: #94a3b8;
  font-size: 13px;
}

/* ── Modal (new file) ── */
.wc-modal-mask {
  position: fixed;
  inset: 0;
  background: rgba(0,0,0,.4);
  z-index: 1000;
  display: flex;
  align-items: center;
  justify-content: center;
}
.wc-modal {
  background: #fff;
  border-radius: 12px;
  padding: 20px 24px;
  width: 380px;
  box-shadow: 0 20px 60px rgba(0,0,0,.15);
}
.wc-modal-title { font-size: 15px; font-weight: 600; color: #1e293b; margin-bottom: 14px; }
.wc-modal-input {
  width: 100%;
  padding: 8px 12px;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  font-size: 13px;
  outline: none;
  transition: border-color .15s;
  margin-bottom: 14px;
}
.wc-modal-input:focus { border-color: #3b82f6; }
.wc-modal-footer { display: flex; gap: 8px; justify-content: flex-end; }
.wc-btn {
  padding: 6px 16px;
  border-radius: 6px;
  border: 1px solid #e2e8f0;
  background: #f8fafc;
  color: #334155;
  font-size: 13px;
  cursor: pointer;
}
.wc-btn.primary { background: #3b82f6; color: #fff; border-color: #3b82f6; }
.wc-btn:hover { opacity: .85; }
</style>
