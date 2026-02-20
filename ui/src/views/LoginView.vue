<template>
  <div class="login-wrapper">
    <el-card class="login-card" shadow="always">
      <template #header>
        <div class="login-header">
          <el-icon :size="32" color="#409EFF"><Monitor /></el-icon>
          <h2>引巢 · ZyHive</h2>
          <p class="subtitle">zyling AI 团队操作系统</p>
        </div>
      </template>
      <el-form @submit.prevent="handleLogin">
        <el-form-item label="访问令牌">
          <el-input
            v-model="token"
            type="password"
            placeholder="输入 Token（默认 changeme 可跳过）"
            show-password
            size="large"
          />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" native-type="submit" :loading="loading" size="large" style="width: 100%">
            登录
          </el-button>
        </el-form-item>
        <el-form-item>
          <el-button type="default" @click="skipLogin" size="large" style="width: 100%">
            跳过（无密码模式）
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>
    <p class="login-copyright">© 2025 引巢 · ZyHive · zyling</p>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import api from '../api'

const router = useRouter()
const token = ref('')
const loading = ref(false)

async function handleLogin() {
  loading.value = true
  try {
    localStorage.setItem('aipanel_token', token.value)
    await api.get('/health')
    ElMessage.success('登录成功')
    router.push('/')
  } catch {
    ElMessage.error('令牌无效')
    localStorage.removeItem('aipanel_token')
  } finally {
    loading.value = false
  }
}

function skipLogin() {
  localStorage.removeItem('aipanel_token')
  router.push('/')
}
</script>

<style scoped>
.login-wrapper {
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  min-height: 100vh;
  gap: 16px;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
}
.login-copyright {
  font-size: 12px;
  color: rgba(255,255,255,0.45);
  margin: 0;
  letter-spacing: 0.3px;
}
.login-card {
  width: 420px;
}
.login-header {
  text-align: center;
}
.login-header h2 {
  margin: 8px 0 4px;
  font-size: 24px;
}
.subtitle {
  color: #909399;
  margin: 0;
  font-size: 14px;
}
</style>
