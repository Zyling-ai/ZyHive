<template>
  <div class="login-wrapper">
    <el-card class="login-card" shadow="always">
      <template #header>
        <div class="login-header">
          <svg width="40" height="40" viewBox="0 0 24 24" fill="none">
            <path d="M12 2L21.5 7.5V16.5L12 22L2.5 16.5V7.5L12 2Z" fill="#409EFF"/>
            <text x="12" y="16.5" text-anchor="middle" fill="white" font-size="10" font-weight="800" font-family="sans-serif">Z</text>
          </svg>
          <h2>引巢 · ZyHive</h2>
          <p class="subtitle">zyling AI 团队操作系统</p>
        </div>
      </template>
      <el-form @submit.prevent="handleLogin">
        <el-form-item label="访问令牌">
          <el-input
            v-model="token"
            type="password"
            placeholder="输入访问令牌"
            show-password
            size="large"
          />
        </el-form-item>

        <!-- 简单算术验证码 -->
        <el-form-item label="验证码">
          <div class="captcha-row">
            <span class="captcha-question">{{ captchaA }} + {{ captchaB }} = ?</span>
            <el-input
              v-model="captchaInput"
              placeholder="结果"
              size="large"
              style="width:100px;margin:0 8px"
              @keyup.enter="handleLogin"
            />
            <el-button size="small" text type="primary" @click="refreshCaptcha">换一题</el-button>
          </div>
        </el-form-item>

        <el-form-item>
          <el-button type="primary" native-type="submit" :loading="loading" size="large" style="width: 100%">
            登录
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>
    <p class="login-copyright">© 2026 引巢 · ZyHive · zyling</p>
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

// 验证码
const captchaA = ref(0)
const captchaB = ref(0)
const captchaInput = ref('')

function refreshCaptcha() {
  captchaA.value = Math.floor(Math.random() * 9) + 1
  captchaB.value = Math.floor(Math.random() * 9) + 1
  captchaInput.value = ''
}
refreshCaptcha()

async function handleLogin() {
  const answer = parseInt(captchaInput.value.trim(), 10)
  if (isNaN(answer) || answer !== captchaA.value + captchaB.value) {
    ElMessage.error('验证码错误，请重新计算')
    refreshCaptcha()
    return
  }
  loading.value = true
  try {
    localStorage.setItem('aipanel_token', token.value)
    await api.get('/health')
    ElMessage.success('登录成功')
    router.push('/')
  } catch {
    ElMessage.error('令牌无效')
    localStorage.removeItem('aipanel_token')
    refreshCaptcha()
  } finally {
    loading.value = false
  }
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
  background: linear-gradient(135deg, #1a1b2e 0%, #16213e 50%, #0f3460 100%);
}
.login-copyright {
  font-size: 12px;
  color: rgba(255,255,255,0.3);
  margin: 0;
}
.login-card { width: 420px; }
.login-header { text-align: center; }
.login-header h2 { margin: 8px 0 4px; font-size: 22px; }
.subtitle { color: #909399; margin: 0; font-size: 13px; }
.captcha-row {
  display: flex;
  align-items: center;
  width: 100%;
}
.captcha-question {
  font-size: 17px;
  font-weight: 700;
  color: #303133;
  background: #f0f2f5;
  padding: 8px 14px;
  border-radius: 6px;
  letter-spacing: 3px;
  font-family: monospace;
  flex-shrink: 0;
  white-space: nowrap;
}
</style>
