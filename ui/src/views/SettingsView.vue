<template>
  <div class="settings-page">
    <h2 style="margin: 0 0 20px"><el-icon style="vertical-align:-2px;margin-right:6px"><Setting /></el-icon>设置</h2>

    <el-card shadow="hover" style="max-width: 600px">
      <el-form label-width="120px">
        <el-form-item label="面板端口">
          <el-input-number v-model="port" :min="1024" :max="65535" />
        </el-form-item>
        <el-form-item label="访问令牌">
          <el-input v-model="token" type="password" show-password placeholder="留空使用默认" style="max-width: 300px" />
        </el-form-item>
        <el-form-item label="语言">
          <el-select v-model="lang" style="width: 200px">
            <el-option label="中文" value="zh" />
            <el-option label="English" value="en" disabled />
          </el-select>
        </el-form-item>
        <el-form-item label="主题">
          <el-select v-model="theme" style="width: 200px">
            <el-option label="浅色" value="light" />
            <el-option label="深色" value="dark" disabled />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="save" :loading="saving">保存设置</el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { config as configApi } from '../api'

const port = ref(8080)
const token = ref('')
const lang = ref('zh')
const theme = ref('light')
const saving = ref(false)

onMounted(async () => {
  try {
    const res = await configApi.get()
    port.value = res.data.gateway?.port || 8080
  } catch {}
})

async function save() {
  saving.value = true
  try {
    const patch: any = { gateway: { port: port.value } }
    if (token.value) patch.auth = { mode: 'token', token: token.value }
    await configApi.patch(patch)
    ElMessage.success('设置已保存')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}
</script>
