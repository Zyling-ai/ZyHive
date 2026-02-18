<template>
  <div class="channels-page">
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px">
      <h2 style="margin: 0">📡 消息通道</h2>
      <el-button type="primary" @click="openAdd">
        <el-icon><Plus /></el-icon> 添加通道
      </el-button>
    </div>

    <el-card shadow="hover">
      <el-table :data="list" stripe>
        <el-table-column label="类型" width="120">
          <template #default="{ row }">
            <el-tag size="small">{{ row.type }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="name" label="名称" min-width="160" />
        <el-table-column label="配置" min-width="200">
          <template #default="{ row }">
            <el-text type="info" size="small">
              <span v-for="(v, k) in row.config" :key="k" style="margin-right: 8px">
                {{ k }}: {{ v }}
              </span>
            </el-text>
          </template>
        </el-table-column>
        <el-table-column label="启用" width="80">
          <template #default="{ row }">
            <el-switch v-model="row.enabled" @change="toggleEnabled(row)" size="small" />
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="row.status === 'ok' ? 'success' : row.status === 'error' ? 'danger' : 'info'" size="small">
              {{ row.status === 'ok' ? '✓' : row.status === 'error' ? '✗' : '?' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="180">
          <template #default="{ row }">
            <el-button size="small" @click="testChannel(row)">测试</el-button>
            <el-button size="small" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="deleteChannel(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- Add/Edit Dialog -->
    <el-dialog v-model="dialogVisible" :title="editingId ? '编辑通道' : '添加通道'" width="520px">
      <el-form :model="form" label-width="120px">
        <el-form-item label="类型" required>
          <el-select v-model="form.type" style="width: 100%">
            <el-option label="Telegram" value="telegram" />
            <el-option label="iMessage" value="imessage" />
            <el-option label="WhatsApp" value="whatsapp" />
          </el-select>
        </el-form-item>
        <el-form-item label="名称" required>
          <el-input v-model="form.name" placeholder="如 Telegram Bot" />
        </el-form-item>
        <el-form-item label="ID">
          <el-input v-model="form.id" placeholder="唯一标识" />
        </el-form-item>

        <!-- Telegram-specific fields -->
        <template v-if="form.type === 'telegram'">
          <el-form-item label="Bot Token" required>
            <el-input v-model="form.config.botToken" type="password" show-password />
          </el-form-item>
          <el-form-item label="默认 Agent">
            <el-input v-model="form.config.defaultAgent" placeholder="main" />
          </el-form-item>
          <el-form-item label="允许的发送者">
            <el-input v-model="form.config.allowedFrom" placeholder="逗号分隔的 Telegram 用户 ID" />
          </el-form-item>
        </template>

        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveChannel" :loading="saving">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { channels as channelsApi, type ChannelEntry } from '../api'

const list = ref<ChannelEntry[]>([])
const dialogVisible = ref(false)
const editingId = ref('')
const saving = ref(false)

const form = reactive({
  id: '', name: '', type: 'telegram', enabled: true,
  config: { botToken: '', defaultAgent: 'main', allowedFrom: '' } as Record<string, string>,
})

onMounted(loadList)

async function loadList() {
  try {
    const res = await channelsApi.list()
    list.value = res.data
  } catch {}
}

function openAdd() {
  editingId.value = ''
  Object.assign(form, {
    id: '', name: '', type: 'telegram', enabled: true,
    config: { botToken: '', defaultAgent: 'main', allowedFrom: '' },
  })
  dialogVisible.value = true
}

function openEdit(row: ChannelEntry) {
  editingId.value = row.id
  Object.assign(form, { ...row, config: { ...row.config } })
  dialogVisible.value = true
}

async function saveChannel() {
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
      await channelsApi.update(editingId.value, { ...form } as any)
    } else {
      await channelsApi.create({ ...form, status: 'untested' } as any)
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

async function toggleEnabled(row: ChannelEntry) {
  try {
    await channelsApi.update(row.id, { enabled: row.enabled } as any)
  } catch {
    ElMessage.error('更新失败')
  }
}

async function testChannel(row: ChannelEntry) {
  try {
    await channelsApi.test(row.id)
    ElMessage.success('测试成功')
    loadList()
  } catch {
    ElMessage.error('测试失败')
  }
}

async function deleteChannel(row: ChannelEntry) {
  try {
    await ElMessageBox.confirm(`确定删除通道 "${row.name}"？`, '确认删除', { type: 'warning' })
    await channelsApi.delete(row.id)
    ElMessage.success('已删除')
    loadList()
  } catch {}
}
</script>
