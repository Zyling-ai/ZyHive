<template>
  <div class="projects-layout">
    <!-- 左栏：项目列表 -->
    <div class="projects-sidebar">
      <div class="sidebar-header">
        <span class="sidebar-title">项目</span>
        <el-button text size="small" @click="showCreate = true" title="新建项目">
          <el-icon><Plus /></el-icon>
        </el-button>
      </div>

      <div class="project-list">
        <div
          v-for="p in projectList"
          :key="p.id"
          class="project-item"
          :class="{ active: currentProjectId === p.id }"
          @click="selectProject(p.id)"
        >
          <el-icon class="project-icon"><FolderOpened /></el-icon>
          <div class="project-item-body">
            <div class="project-name">{{ p.name }}</div>
            <div v-if="p.description" class="project-desc">{{ p.description }}</div>
          </div>
          <el-dropdown trigger="click" @click.stop>
            <el-icon class="project-menu-btn"><MoreFilled /></el-icon>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item @click="openEdit(p)">编辑信息</el-dropdown-item>
                <el-dropdown-item divided @click="confirmDelete(p)" style="color:#f56c6c">删除项目</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>

        <div v-if="!projectList.length" class="empty-projects">
          <el-icon size="32" style="color:#c0c4cc"><FolderOpened /></el-icon>
          <p>暂无项目</p>
          <el-button size="small" type="primary" plain @click="showCreate = true">新建项目</el-button>
        </div>
      </div>
    </div>

    <!-- 右侧：文件浏览器 -->
    <div class="projects-main" v-if="currentProject">
      <div class="main-header">
        <div class="main-title">
          <el-icon style="color:#e6a23c"><FolderOpened /></el-icon>
          <span>{{ currentProject.name }}</span>
          <el-tag v-for="tag in (currentProject.tags || [])" :key="tag" size="small" style="margin-left:4px">{{ tag }}</el-tag>
        </div>
        <el-text v-if="currentProject.description" type="info" size="small">{{ currentProject.description }}</el-text>
      </div>

      <el-row :gutter="12" style="flex:1;overflow:hidden;">
        <!-- 文件树 -->
        <el-col :span="6" style="height:100%;display:flex;flex-direction:column;">
          <div class="file-panel">
            <div class="file-panel-header">
              <span>文件</span>
              <div style="display:flex;gap:4px;">
                <el-button text size="small" @click="showNewFile = true" title="新建文件">
                  <el-icon><DocumentAdd /></el-icon>
                </el-button>
                <el-button text size="small" @click="showNewFolder = true" title="新建文件夹">
                  <el-icon><FolderAdd /></el-icon>
                </el-button>
                <el-button text size="small" @click="loadTree" title="刷新">
                  <el-icon><Refresh /></el-icon>
                </el-button>
              </div>
            </div>
            <div style="flex:1;overflow-y:auto;">
              <el-tree
                v-if="treeData.length"
                :data="treeData"
                :props="{ label: 'name', children: 'children' }"
                highlight-current
                default-expand-all
                @node-click="onFileClick"
                style="font-size:13px;"
              >
                <template #default="{ data }">
                  <span class="tree-node">
                    <el-icon v-if="data.isDir" style="color:#e6a23c;font-size:13px;flex-shrink:0"><FolderOpened /></el-icon>
                    <el-icon v-else :style="{ color: fileColor(data.name), fontSize: '13px', flexShrink: 0 }"><Document /></el-icon>
                    <span class="tree-label">{{ data.name }}</span>
                    <span v-if="!data.isDir" class="tree-size">{{ fmtSize(data.size) }}</span>
                  </span>
                </template>
              </el-tree>
              <el-empty v-else description="暂无文件" :image-size="48" />
            </div>
          </div>
        </el-col>

        <!-- 编辑器 -->
        <el-col :span="18" style="height:100%;display:flex;flex-direction:column;">
          <div class="editor-panel">
            <div class="editor-header" v-if="currentFile">
              <span class="editor-path">{{ currentFile }}</span>
              <el-tag size="small" style="font-size:11px;">{{ fileExt(currentFile) }}</el-tag>
              <div style="flex:1" />
              <el-button text size="small" type="danger" @click="deleteCurrentFile" title="删除">
                <el-icon><Delete /></el-icon>
              </el-button>
            </div>
            <div class="editor-header" v-else>
              <span style="color:#c0c4cc;font-size:13px;">从左侧选择文件</span>
            </div>
            <el-input
              v-if="currentFile && !isBinary"
              v-model="fileContent"
              type="textarea"
              :placeholder="'（空文件）'"
              :autosize="false"
              style="flex:1;font-family:monospace;font-size:13px;"
            />
            <div v-else-if="currentFile && isBinary" class="binary-hint">
              <el-icon size="32"><Document /></el-icon>
              <p>二进制文件，无法编辑</p>
            </div>
            <div v-else class="editor-empty">
              <el-icon size="48" style="color:#e4e7ed"><EditPen /></el-icon>
              <p>选择文件后在此编辑</p>
            </div>
            <div v-if="currentFile && !isBinary" class="editor-footer">
              <el-button type="primary" size="small" @click="saveFile">保存</el-button>
              <el-text type="info" size="small" v-if="fileInfo">
                {{ fmtSize(fileInfo.size) }} · {{ fmtTime(fileInfo.modTime) }}
              </el-text>
            </div>
          </div>
        </el-col>
      </el-row>
    </div>

    <!-- 未选择项目 -->
    <div class="projects-main projects-empty" v-else>
      <el-icon size="64" style="color:#e4e7ed;margin-bottom:16px"><FolderOpened /></el-icon>
      <p style="color:#909399;font-size:15px;">从左侧选择或新建一个项目</p>
      <el-button type="primary" @click="showCreate = true">
        <el-icon><Plus /></el-icon> 新建项目
      </el-button>
    </div>

    <!-- 新建项目 Dialog -->
    <el-dialog v-model="showCreate" title="新建项目" width="440px" @close="resetForm">
      <el-form :model="createForm" label-width="80px">
        <el-form-item label="项目 ID" required>
          <el-input v-model="createForm.id" placeholder="如 ai-panel（小写字母/数字/连字符）" />
        </el-form-item>
        <el-form-item label="名称" required>
          <el-input v-model="createForm.name" placeholder="项目名称" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="createForm.description" placeholder="简短描述（可选）" />
        </el-form-item>
        <el-form-item label="标签">
          <el-input v-model="createForm.tagsStr" placeholder="多个标签用逗号分隔，如 go,vue" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreate = false">取消</el-button>
        <el-button type="primary" @click="doCreate">创建</el-button>
      </template>
    </el-dialog>

    <!-- 编辑项目 Dialog -->
    <el-dialog v-model="showEdit" title="编辑项目信息" width="440px">
      <el-form :model="editForm" label-width="80px">
        <el-form-item label="名称">
          <el-input v-model="editForm.name" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="editForm.description" />
        </el-form-item>
        <el-form-item label="标签">
          <el-input v-model="editForm.tagsStr" placeholder="多个标签用逗号分隔" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showEdit = false">取消</el-button>
        <el-button type="primary" @click="doEdit">保存</el-button>
      </template>
    </el-dialog>

    <!-- 新建文件 Dialog -->
    <el-dialog v-model="showNewFile" title="新建文件" width="380px">
      <el-input v-model="newFilePath" placeholder="如 README.md 或 src/main.go" @keyup.enter="doNewFile" />
      <template #footer>
        <el-button @click="showNewFile = false">取消</el-button>
        <el-button type="primary" @click="doNewFile">创建</el-button>
      </template>
    </el-dialog>

    <!-- 新建文件夹 Dialog -->
    <el-dialog v-model="showNewFolder" title="新建文件夹" width="380px">
      <el-input v-model="newFolderPath" placeholder="如 src 或 docs/api" @keyup.enter="doNewFolder" />
      <template #footer>
        <el-button @click="showNewFolder = false">取消</el-button>
        <el-button type="primary" @click="doNewFolder">创建</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import {
  Plus, FolderOpened, MoreFilled, Document, DocumentAdd, FolderAdd,
  Refresh, Delete, EditPen
} from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { projects as projectsApi, type ProjectInfo, type FileNode } from '../api'

// ── State ─────────────────────────────────────────────────────────────────
const projectList = ref<ProjectInfo[]>([])
const currentProjectId = ref('')
const currentProject = computed(() => projectList.value.find(p => p.id === currentProjectId.value))

const treeData = ref<FileNode[]>([])
const currentFile = ref('')
const fileContent = ref('')
const fileInfo = ref<any>(null)
const isBinary = ref(false)

// Dialogs
const showCreate = ref(false)
const showEdit = ref(false)
const showNewFile = ref(false)
const showNewFolder = ref(false)
const newFilePath = ref('')
const newFolderPath = ref('')

const createForm = ref({ id: '', name: '', description: '', tagsStr: '' })
const editForm = ref({ id: '', name: '', description: '', tagsStr: '' })

// ── Init ──────────────────────────────────────────────────────────────────
onMounted(loadProjects)

async function loadProjects() {
  try {
    const res = await projectsApi.list()
    projectList.value = res.data || []
    // Auto-select first project
    if (projectList.value.length && !currentProjectId.value) {
      const first = projectList.value[0]
      if (first) selectProject(first.id)
    }
  } catch { /* ignore */ }
}

// ── Project selection ─────────────────────────────────────────────────────
async function selectProject(id: string) {
  currentProjectId.value = id
  currentFile.value = ''
  fileContent.value = ''
  fileInfo.value = null
  await loadTree()
}

async function loadTree() {
  if (!currentProjectId.value) return
  try {
    const res = await projectsApi.readTree(currentProjectId.value)
    treeData.value = Array.isArray(res.data) ? res.data : []
  } catch { treeData.value = [] }
}

// ── File operations ───────────────────────────────────────────────────────
async function onFileClick(data: any) {
  if (data.isDir) return
  currentFile.value = data.path || data.name
  fileInfo.value = data
  isBinary.value = false
  try {
    const res = await projectsApi.readFile(currentProjectId.value, currentFile.value)
    if (res.data?.encoding === 'base64') {
      isBinary.value = true
      fileContent.value = ''
    } else {
      fileContent.value = res.data?.content ?? ''
    }
  } catch { fileContent.value = '' }
}

async function saveFile() {
  if (!currentFile.value) return
  try {
    await projectsApi.writeFile(currentProjectId.value, currentFile.value, fileContent.value)
    ElMessage.success('已保存')
    loadTree()
  } catch { ElMessage.error('保存失败') }
}

async function deleteCurrentFile() {
  if (!currentFile.value) return
  try {
    await ElMessageBox.confirm(`删除「${currentFile.value}」？`, '删除文件', {
      confirmButtonText: '确认', cancelButtonText: '取消', type: 'warning',
      confirmButtonClass: 'el-button--danger',
    })
    await projectsApi.deleteFile(currentProjectId.value, currentFile.value)
    ElMessage.success('已删除')
    currentFile.value = ''
    fileContent.value = ''
    fileInfo.value = null
    loadTree()
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error('删除失败')
  }
}

async function doNewFile() {
  const p = newFilePath.value.trim()
  if (!p) return
  try {
    await projectsApi.writeFile(currentProjectId.value, p, '')
    ElMessage.success(`已创建 ${p}`)
    showNewFile.value = false
    newFilePath.value = ''
    await loadTree()
    currentFile.value = p
    fileContent.value = ''
    fileInfo.value = null
    isBinary.value = false
  } catch { ElMessage.error('创建失败') }
}

async function doNewFolder() {
  const p = newFolderPath.value.trim()
  if (!p) return
  // Create a .gitkeep to materialise the directory
  try {
    await projectsApi.writeFile(currentProjectId.value, p + '/.gitkeep', '')
    ElMessage.success(`已创建文件夹 ${p}`)
    showNewFolder.value = false
    newFolderPath.value = ''
    loadTree()
  } catch { ElMessage.error('创建失败') }
}

// ── Project CRUD ──────────────────────────────────────────────────────────
function resetForm() {
  createForm.value = { id: '', name: '', description: '', tagsStr: '' }
}

async function doCreate() {
  const { id, name, description, tagsStr } = createForm.value
  if (!id || !name) { ElMessage.warning('请填写 ID 和名称'); return }
  const tags = tagsStr ? tagsStr.split(',').map(t => t.trim()).filter(Boolean) : []
  try {
    await projectsApi.create({ id, name, description, tags })
    ElMessage.success('项目已创建')
    showCreate.value = false
    resetForm()
    await loadProjects()
    selectProject(id)
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '创建失败')
  }
}

function openEdit(p: ProjectInfo) {
  editForm.value = {
    id: p.id, name: p.name,
    description: p.description || '',
    tagsStr: (p.tags || []).join(', '),
  }
  showEdit.value = true
}

async function doEdit() {
  const { id, name, description, tagsStr } = editForm.value
  const tags = tagsStr ? tagsStr.split(',').map(t => t.trim()).filter(Boolean) : []
  try {
    await projectsApi.update(id, { name, description, tags })
    ElMessage.success('已更新')
    showEdit.value = false
    loadProjects()
  } catch { ElMessage.error('更新失败') }
}

async function confirmDelete(p: ProjectInfo) {
  try {
    await ElMessageBox.confirm(
      `删除项目「${p.name}」将删除其所有文件，不可恢复。`,
      '删除项目',
      { confirmButtonText: '确认删除', cancelButtonText: '取消', type: 'warning', confirmButtonClass: 'el-button--danger' }
    )
    await projectsApi.delete(p.id)
    ElMessage.success('已删除')
    if (currentProjectId.value === p.id) {
      currentProjectId.value = ''
      treeData.value = []
      currentFile.value = ''
    }
    loadProjects()
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error('删除失败')
  }
}

// ── Helpers ───────────────────────────────────────────────────────────────
function fileColor(name: string): string {
  const ext = name.split('.').pop()?.toLowerCase() || ''
  if (['md', 'txt', 'rst'].includes(ext)) return '#409eff'
  if (['json', 'yaml', 'yml', 'toml'].includes(ext)) return '#67c23a'
  if (['go', 'py', 'js', 'ts', 'sh', 'vue', 'rs', 'java', 'c', 'cpp'].includes(ext)) return '#e6a23c'
  if (['jpg', 'jpeg', 'png', 'gif', 'svg', 'webp'].includes(ext)) return '#f56c6c'
  if (name.startsWith('.')) return '#c0c4cc'
  return '#909399'
}

function fileExt(path: string): string {
  const name = path.split('/').pop() || path
  const dot = name.lastIndexOf('.')
  return dot > 0 ? name.slice(dot) : 'file'
}

function fmtSize(bytes: number): string {
  if (!bytes) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

function fmtTime(t: string | undefined): string {
  if (!t) return ''
  return new Date(t).toLocaleString('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}
</script>

<style scoped>
.projects-layout {
  display: flex;
  height: calc(100vh - 60px);
  overflow: hidden;
}

/* 左栏 */
.projects-sidebar {
  width: 220px;
  flex-shrink: 0;
  border-right: 1px solid #e4e7ed;
  background: #fafafa;
  display: flex;
  flex-direction: column;
}
.sidebar-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 14px 8px;
  border-bottom: 1px solid #e4e7ed;
}
.sidebar-title {
  font-weight: 600;
  font-size: 14px;
  color: #303133;
}
.project-list {
  flex: 1;
  overflow-y: auto;
  padding: 6px 0;
}
.project-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  cursor: pointer;
  border-radius: 6px;
  margin: 2px 6px;
  transition: background 0.15s;
  position: relative;
}
.project-item:hover { background: #f0f2f5; }
.project-item.active { background: #ecf5ff; }
.project-icon { color: #e6a23c; font-size: 16px; flex-shrink: 0; }
.project-item-body { flex: 1; min-width: 0; }
.project-name { font-size: 13px; font-weight: 500; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.project-desc { font-size: 11px; color: #909399; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.project-menu-btn { color: #c0c4cc; font-size: 14px; cursor: pointer; }
.project-menu-btn:hover { color: #606266; }
.empty-projects {
  display: flex; flex-direction: column; align-items: center;
  justify-content: center; gap: 8px; padding: 40px 16px;
  color: #909399; font-size: 13px;
}

/* 右侧 */
.projects-main {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  padding: 12px;
  gap: 10px;
}
.projects-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
}
.main-header {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.main-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 15px;
  font-weight: 600;
  color: #303133;
}

/* 文件面板 */
.file-panel {
  height: calc(100vh - 130px);
  display: flex;
  flex-direction: column;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  background: #fff;
  overflow: hidden;
}
.file-panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 10px;
  border-bottom: 1px solid #f0f0f0;
  font-size: 13px;
  font-weight: 500;
}

/* 编辑器面板 */
.editor-panel {
  height: calc(100vh - 130px);
  display: flex;
  flex-direction: column;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  background: #fff;
  overflow: hidden;
}
.editor-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-bottom: 1px solid #f0f0f0;
  min-height: 38px;
}
.editor-path {
  font-family: monospace;
  font-size: 12px;
  color: #606266;
}
.editor-footer {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  border-top: 1px solid #f0f0f0;
}
.editor-empty, .binary-hint {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: #c0c4cc;
  font-size: 13px;
  gap: 8px;
}

/* 树节点 */
.tree-node {
  display: flex;
  align-items: center;
  gap: 5px;
  line-height: 1.8;
  width: 100%;
}
.tree-label {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tree-size {
  font-size: 11px;
  color: #c0c4cc;
  flex-shrink: 0;
}
</style>
