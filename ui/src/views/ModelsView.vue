<template>
  <div class="models-page">
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px">
      <h2 style="margin: 0">🤖 模型配置</h2>
      <el-button type="primary" @click="openAdd">
        <el-icon><Plus /></el-icon> 添加模型
      </el-button>
    </div>

    <el-card shadow="hover">
      <el-table :data="list" stripe>
        <el-table-column label="提供商" width="120">
          <template #default="{ row }">
            <el-tag size="small">{{ row.provider }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="name" label="名称" min-width="160" />
        <el-table-column label="模型" min-width="180">
          <template #default="{ row }">
            <el-text type="info" size="small">{{ row.model }}</el-text>
          </template>
        </el-table-column>
        <el-table-column label="API Key" min-width="180">
          <template #default="{ row }">
            <code style="font-size: 12px; color: #909399">{{ row.apiKey }}</code>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="row.status === 'ok' ? 'success' : row.status === 'error' ? 'danger' : 'info'" size="small">
              {{ row.status === 'ok' ? '✓ 有效' : row.status === 'error' ? '✗ 无效' : '? 未测试' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="默认" width="70">
          <template #default="{ row }">
            <el-tag v-if="row.isDefault" type="warning" size="small">默认</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="180">
          <template #default="{ row }">
            <el-button size="small" @click="testModel(row)" :loading="testing === row.id">测试</el-button>
            <el-button size="small" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="deleteModel(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- Add/Edit Dialog -->
    <el-dialog v-model="dialogVisible" :title="editingId ? '编辑模型' : '添加模型'" width="520px">
      <el-form :model="form" label-width="100px">
        <el-form-item label="提供商" required>
          <el-select v-model="form.provider" @change="onProviderChange" style="width: 100%">
            <el-option label="Anthropic" value="anthropic" />
            <el-option label="OpenAI" value="openai" />
            <el-option label="DeepSeek" value="deepseek" />
            <el-option label="自定义" value="custom" />
          </el-select>
        </el-form-item>
        <el-form-item label="模型" required>
          <el-select v-if="form.provider !== 'custom'" v-model="form.model" style="width: 100%">
            <el-option v-for="m in providerModels" :key="m.value" :label="m.label" :value="m.value" />
          </el-select>
          <el-input v-else v-model="form.model" placeholder="模型名称" />
        </el-form-item>
        <el-form-item label="显示名称">
          <el-input v-model="form.name" placeholder="如 Claude Sonnet 4" />
        </el-form-item>
        <el-form-item label="ID">
          <el-input v-model="form.id" placeholder="唯一标识" />
        </el-form-item>
        <el-form-item label="API Key" required>
          <el-input v-model="form.apiKey" type="password" show-password placeholder="sk-..." />
        </el-form-item>
        <el-form-item label="设为默认">
          <el-switch v-model="form.isDefault" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveModel" :loading="saving">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { models as modelsApi, type ModelEntry } from '../api'

const list = ref<ModelEntry[]>([])
const dialogVisible = ref(false)
const editingId = ref('')
const saving = ref(false)
const testing = ref('')

const form = reactive({
  id: '', name: '', provider: 'anthropic', model: 'claude-sonnet-4-6',
  apiKey: '', isDefault: false,
})

const modelOptions: Record<string, { label: string; value: string }[]> = {
  anthropic: [
    { label: 'Claude Sonnet 4', value: 'claude-sonnet-4-6' },
    { label: 'Claude Opus 4', value: 'claude-opus-4-0' },
    { label: 'Claude Haiku 3.5', value: 'claude-3-5-haiku-20241022' },
  ],
  openai: [
    { label: 'GPT-4o', value: 'gpt-4o' },
    { label: 'GPT-4o Mini', value: 'gpt-4o-mini' },
    { label: 'o1', value: 'o1' },
  ],
  deepseek: [
    { label: 'DeepSeek V3', value: 'deepseek-chat' },
    { label: 'DeepSeek R1', value: 'deepseek-reasoner' },
  ],
}

const providerModels = computed(() => modelOptions[form.provider] || [])

function onProviderChange() {
  const opts = providerModels.value
  if (opts.length && opts[0]) {
    form.model = opts[0]!.value
    form.name = opts[0]!.label
    form.id = form.provider + '-' + form.model.replace(/[^a-z0-9]/g, '-')
  }
}

onMounted(loadList)

async function loadList() {
  try {
    const res = await modelsApi.list()
    list.value = res.data
  } catch {}
}

function openAdd() {
  editingId.value = ''
  Object.assign(form, { id: '', name: '', provider: 'anthropic', model: 'claude-sonnet-4-6', apiKey: '', isDefault: false })
  onProviderChange()
  dialogVisible.value = true
}

function openEdit(row: ModelEntry) {
  editingId.value = row.id
  Object.assign(form, { ...row })
  dialogVisible.value = true
}

async function saveModel() {
  if (!form.id || !form.provider || !form.model) {
    ElMessage.warning('请填写必要字段')
    return
  }
  saving.value = true
  try {
    if (editingId.value) {
      await modelsApi.update(editingId.value, { ...form } as any)
    } else {
      await modelsApi.create({ ...form, status: 'untested' } as any)
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

async function testModel(row: ModelEntry) {
  testing.value = row.id
  try {
    const res = await modelsApi.test(row.id)
    if (res.data.valid) {
      ElMessage.success('连接成功！')
    } else {
      ElMessage.error('连接失败: ' + (res.data.error || ''))
    }
    loadList()
  } catch {
    ElMessage.error('测试请求失败')
  } finally {
    testing.value = ''
  }
}

async function deleteModel(row: ModelEntry) {
  try {
    await ElMessageBox.confirm(`确定删除模型 "${row.name}"？`, '确认删除', { type: 'warning' })
    await modelsApi.delete(row.id)
    ElMessage.success('已删除')
    loadList()
  } catch {}
}
</script>
