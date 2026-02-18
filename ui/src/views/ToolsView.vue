<template>
  <div class="tools-page">
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px">
      <h2 style="margin: 0">🛠️ 能力配置</h2>
      <el-button type="primary" @click="openAdd">
        <el-icon><Plus /></el-icon> 添加能力
      </el-button>
    </div>

    <el-card shadow="hover">
      <el-table :data="list" stripe>
        <el-table-column prop="name" label="名称" min-width="160" />
        <el-table-column label="类型" width="140">
          <template #default="{ row }">
            <el-tag size="small">{{ row.type }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="API Key" min-width="180">
          <template #default="{ row }">
            <code style="font-size: 12px; color: #909399">{{ row.apiKey }}</code>
          </template>
        </el-table-column>
        <el-table-column label="启用" width="80">
          <template #default="{ row }">
            <el-switch v-model="row.enabled" @change="toggleEnabled(row)" size="small" />
          </template>
        </el-table-column>
        <el-table-column label="状态" width="80">
          <template #default="{ row }">
            <el-tag :type="row.status === 'ok' ? 'success' : 'info'" size="small">
              {{ row.status === 'ok' ? '✓' : '?' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="180">
          <template #default="{ row }">
            <el-button size="small" @click="testTool(row)">测试</el-button>
            <el-button size="small" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="deleteTool(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" :title="editingId ? '编辑能力' : '添加能力'" width="520px">
      <el-form :model="form" label-width="100px">
        <el-form-item label="类型" required>
          <el-select v-model="form.type" style="width: 100%">
            <el-option label="Brave Search" value="brave_search" />
            <el-option label="ElevenLabs" value="elevenlabs" />
            <el-option label="自定义" value="custom" />
          </el-select>
        </el-form-item>
        <el-form-item label="名称" required>
          <el-input v-model="form.name" placeholder="如 Brave Search" />
        </el-form-item>
        <el-form-item label="ID">
          <el-input v-model="form.id" placeholder="唯一标识" />
        </el-form-item>
        <el-form-item label="API Key" required>
          <el-input v-model="form.apiKey" type="password" show-password />
        </el-form-item>
        <el-form-item v-if="form.type === 'custom'" label="Base URL">
          <el-input v-model="form.baseUrl" placeholder="https://..." />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveTool" :loading="saving">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { tools as toolsApi, type ToolEntry } from '../api'

const list = ref<ToolEntry[]>([])
const dialogVisible = ref(false)
const editingId = ref('')
const saving = ref(false)

const form = reactive({
  id: '', name: '', type: 'brave_search', apiKey: '', baseUrl: '', enabled: true,
})

onMounted(loadList)

async function loadList() {
  try {
    const res = await toolsApi.list()
    list.value = res.data
  } catch {}
}

function openAdd() {
  editingId.value = ''
  Object.assign(form, { id: '', name: '', type: 'brave_search', apiKey: '', baseUrl: '', enabled: true })
  dialogVisible.value = true
}

function openEdit(row: ToolEntry) {
  editingId.value = row.id
  Object.assign(form, { ...row })
  dialogVisible.value = true
}

async function saveTool() {
  if (!form.name || !form.type) {
    ElMessage.warning('请填写必要字段')
    return
  }
  if (!form.id) {
    form.id = form.type + '-' + Date.now().toString(36)
  }
  saving.value = true
  try {
    if (editingId.value) {
      await toolsApi.update(editingId.value, { ...form } as any)
    } else {
      await toolsApi.create({ ...form, status: 'untested' } as any)
    }
    ElMessage.success('保存成功')
    dialogVisible.value = false
    loadList()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}

async function toggleEnabled(row: ToolEntry) {
  try {
    await toolsApi.update(row.id, { enabled: row.enabled } as any)
  } catch {
    ElMessage.error('更新失败')
  }
}

async function testTool(row: ToolEntry) {
  try {
    await toolsApi.test(row.id)
    ElMessage.success('测试成功')
    loadList()
  } catch {
    ElMessage.error('测试失败')
  }
}

async function deleteTool(row: ToolEntry) {
  try {
    await ElMessageBox.confirm(`确定删除 "${row.name}"？`, '确认删除', { type: 'warning' })
    await toolsApi.delete(row.id)
    ElMessage.success('已删除')
    loadList()
  } catch {}
}
</script>
