<template>
  <div class="tools-page">
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px">
      <h2 style="margin: 0"><el-icon style="vertical-align:-2px;margin-right:6px"><SetUp /></el-icon>能力配置 <span style="font-size:12px;color:#64748b;font-weight:400;">（全局，所有成员共享）</span></h2>
      <el-button type="primary" @click="openAdd">
        <el-icon><Plus /></el-icon> 添加能力
      </el-button>
    </div>
    <p style="margin: 0 0 16px; color: #64748b; font-size: 13px;">
      配置<strong>所有 AI 成员</strong>都能使用的工具 API Key，例如 Brave Search、ElevenLabs 等。<br>
      如需配置<strong>某个成员专属</strong>的 API Key 或 Token，请进入该成员的「环境变量」Tab。
    </p>

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

    <!-- ── 全局工具权限策略 ── -->
    <el-card shadow="hover" style="margin-top: 28px;">
      <template #header>
        <span style="font-weight: 600; font-size: 14px;">🔒 全局工具权限策略</span>
        <span style="font-size: 12px; color: #64748b; margin-left: 8px;">所有成员默认继承此策略，可在成员详情页单独覆盖</span>
      </template>

      <el-form label-width="90px" size="default" style="max-width: 680px;">
        <el-form-item label="Profile">
          <el-select v-model="globalPolicy.profile" placeholder="不限制（full）" style="width: 260px;" clearable>
            <el-option label="full — 不限制（默认）" value="full" />
            <el-option label="coding — 文件+命令+Agent+记忆" value="coding" />
            <el-option label="messaging — 仅消息+Sessions" value="messaging" />
            <el-option label="minimal — 仅 send_message + 记忆" value="minimal" />
          </el-select>
        </el-form-item>

        <el-form-item label="全局 Allow">
          <div style="width: 100%">
            <el-tag
              v-for="(item, idx) in globalPolicy.allow"
              :key="idx"
              closable
              size="small"
              style="margin: 2px 4px 2px 0;"
              @close="globalPolicy.allow.splice(idx, 1)"
            >{{ item }}</el-tag>
            <el-input
              v-model="globalPolicyAllowInput"
              size="small"
              placeholder="工具名或 group:xx，回车添加"
              style="width: 260px; margin-top: 4px;"
              @keyup.enter="addGlobalTag('allow')"
            >
              <template #append><el-button @click="addGlobalTag('allow')">添加</el-button></template>
            </el-input>
          </div>
        </el-form-item>

        <el-form-item label="全局 Deny">
          <div style="width: 100%">
            <el-tag
              v-for="(item, idx) in globalPolicy.deny"
              :key="idx"
              closable
              type="danger"
              size="small"
              style="margin: 2px 4px 2px 0;"
              @close="globalPolicy.deny.splice(idx, 1)"
            >{{ item }}</el-tag>
            <el-input
              v-model="globalPolicyDenyInput"
              size="small"
              placeholder="工具名或 group:xx，回车拒绝"
              style="width: 260px; margin-top: 4px;"
              @keyup.enter="addGlobalTag('deny')"
            >
              <template #append><el-button @click="addGlobalTag('deny')">添加</el-button></template>
            </el-input>
          </div>
        </el-form-item>

        <el-form-item label="">
          <el-button type="primary" :loading="globalPolicySaving" @click="saveGlobalPolicy">
            保存全局策略
          </el-button>
          <el-button plain @click="loadGlobalPolicy">重置</el-button>
          <span v-if="globalPolicySaved" style="margin-left:10px;color:#67c23a;font-size:13px;">✓ 已保存</span>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { tools as toolsApi, config as configApi, type ToolEntry } from '../api'

const list = ref<ToolEntry[]>([])
const dialogVisible = ref(false)
const editingId = ref('')
const saving = ref(false)

const form = reactive({
  id: '', name: '', type: 'brave_search', apiKey: '', baseUrl: '', enabled: true,
})

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

// ── 全局工具权限策略 ──────────────────────────────────────────────────────────
const globalPolicy = reactive<{ profile: string; allow: string[]; deny: string[] }>({
  profile: '',
  allow: [],
  deny: [],
})
const globalPolicyAllowInput = ref('')
const globalPolicyDenyInput = ref('')
const globalPolicySaving = ref(false)
const globalPolicySaved = ref(false)

async function loadGlobalPolicy() {
  try {
    const res = await configApi.get()
    const p = res.data?.toolPolicy
    globalPolicy.profile = p?.profile || ''
    globalPolicy.allow = p?.allow ? [...p.allow] : []
    globalPolicy.deny = p?.deny ? [...p.deny] : []
  } catch {}
}

function addGlobalTag(type: 'allow' | 'deny') {
  const input = type === 'allow' ? globalPolicyAllowInput : globalPolicyDenyInput
  const val = input.value.trim()
  if (!val) return
  if (!globalPolicy[type].includes(val)) globalPolicy[type].push(val)
  input.value = ''
}

async function saveGlobalPolicy() {
  globalPolicySaving.value = true
  try {
    const policy: any = {}
    if (globalPolicy.profile) policy.profile = globalPolicy.profile
    if (globalPolicy.allow.length) policy.allow = globalPolicy.allow
    if (globalPolicy.deny.length) policy.deny = globalPolicy.deny
    await configApi.patch({ toolPolicy: Object.keys(policy).length ? policy : null })
    globalPolicySaved.value = true
    setTimeout(() => { globalPolicySaved.value = false }, 2000)
    ElMessage.success('全局工具权限已保存，重启后生效')
  } catch {
    ElMessage.error('保存失败')
  } finally {
    globalPolicySaving.value = false
  }
}

onMounted(() => { loadList(); loadGlobalPolicy() })
</script>
