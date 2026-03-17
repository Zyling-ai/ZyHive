<template>
  <div class="login-wrapper">
    <!-- 主卡片 -->
    <div class="login-card">
      <!-- 头部 Logo -->
      <div class="login-header">
        <div class="login-logo">
          <svg width="44" height="44" viewBox="0 0 24 24" fill="none">
            <path d="M12 2L21.5 7.5V16.5L12 22L2.5 16.5V7.5L12 2Z" fill="url(#zg)" />
            <defs>
              <linearGradient id="zg" x1="2.5" y1="2" x2="21.5" y2="22" gradientUnits="userSpaceOnUse">
                <stop offset="0%" stop-color="#6366f1"/>
                <stop offset="100%" stop-color="#0ea5e9"/>
              </linearGradient>
            </defs>
            <text x="12" y="16.5" text-anchor="middle" fill="white" font-size="9.5"
              font-weight="900" font-family="sans-serif">Z</text>
          </svg>
        </div>
        <h1 class="login-title">引巢 · ZyHive</h1>
        <p class="login-sub">zyling AI 团队操作系统</p>
      </div>

      <!-- 表单 -->
      <form class="login-form" @submit.prevent="handleLogin">
        <!-- 服务器地址 -->
        <div class="field-group">
          <label class="field-label">服务器地址</label>
          <input
            v-model="serverUrl"
            class="field-input"
            type="text"
            placeholder="http://localhost:8080"
            autocomplete="off"
          />
        </div>

        <!-- 访问令牌 -->
        <div class="field-group">
          <label class="field-label">访问令牌</label>
          <div class="secret-row">
            <input
              v-model="token"
              class="field-input"
              :type="showToken ? 'text' : 'password'"
              placeholder="粘贴访问令牌"
              autocomplete="off"
              spellcheck="false"
              @keydown.enter="handleLogin"
            />
            <button type="button" class="icon-btn" :title="showToken ? '隐藏' : '显示'" @click="showToken = !showToken">
              <svg v-if="showToken" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/>
              </svg>
              <svg v-else width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94"/>
                <path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19"/>
                <line x1="1" y1="1" x2="23" y2="23"/>
              </svg>
            </button>
          </div>
        </div>

        <!-- 错误提示 -->
        <div v-if="errorMsg" class="error-callout">
          <div class="error-callout__icon">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/>
            </svg>
          </div>
          <div>
            <div>{{ errorMsg }}</div>
            <div v-if="showTokenHint" class="error-hint">
              重新获取令牌：<code>zyhive token</code>
            </div>
          </div>
        </div>

        <button type="submit" class="connect-btn" :disabled="loading">
          <span v-if="loading" class="spin">⟳</span>
          <span>{{ loading ? '连接中...' : '连接' }}</span>
        </button>
      </form>

      <!-- 连接指引 -->
      <div class="guide-section">
        <div class="guide-title">如何连接</div>
        <ol class="guide-steps">
          <li>
            在服务器上启动 ZyHive：
            <code class="code-block">zyhive start</code>
          </li>
          <li>
            查看访问令牌：
            <code class="code-block">zyhive token</code>
          </li>
          <li>将令牌粘贴到上方输入框，点击连接</li>
        </ol>
        <div class="guide-tip">
          💡 安装时如果选择了自动配置，令牌会在安装结束时显示
        </div>
      </div>
    </div>

    <p class="login-footer">
      引巢 · ZyHive
      <span v-if="version" class="ver">{{ version }}</span>
      · © 2026 zyling
    </p>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import axios from 'axios'

const router = useRouter()
const token = ref('')
const loading = ref(false)
const version = ref('')
const showToken = ref(false)
const errorMsg = ref('')
const showTokenHint = ref(false)

// 默认服务器地址 = 当前页面地址
const serverUrl = ref(window.location.origin)

onMounted(async () => {
  // 尝试获取版本号（不需要鉴权）
  try {
    const base = serverUrl.value || window.location.origin
    const res = await axios.get(`${base}/api/version`, { timeout: 3000 })
    version.value = res.data.version || ''
  } catch {}

  // 如果已有 token，直接尝试静默连接
  const saved = localStorage.getItem('aipanel_token')
  const savedUrl = localStorage.getItem('aipanel_url')
  if (saved) {
    token.value = saved
    if (savedUrl) serverUrl.value = savedUrl
  }
})

async function handleLogin() {
  errorMsg.value = ''
  showTokenHint.value = false

  if (!token.value.trim()) {
    errorMsg.value = '请输入访问令牌'
    return
  }

  loading.value = true
  try {
    const base = (serverUrl.value || window.location.origin).replace(/\/$/, '')
    // 更新 axios 基础 URL
    axios.defaults.baseURL = base
    // 测试连接
    await axios.get(`${base}/api/health`, {
      headers: { Authorization: `Bearer ${token.value.trim()}` },
      timeout: 5000,
    })
    localStorage.setItem('aipanel_token', token.value.trim())
    localStorage.setItem('aipanel_url', base)
    // 跳转主页
    router.push('/')
  } catch (e: any) {
    const status = e?.response?.status
    if (status === 401 || status === 403) {
      errorMsg.value = '令牌无效或已过期，请重新获取'
      showTokenHint.value = true
    } else if (!e?.response) {
      errorMsg.value = `无法连接到服务器：${serverUrl.value}`
      showTokenHint.value = false
    } else {
      errorMsg.value = `连接失败（${status}）`
      showTokenHint.value = false
    }
    localStorage.removeItem('aipanel_token')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-wrapper {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  gap: 20px;
  background: linear-gradient(135deg, #0f0f1a 0%, #131428 50%, #0d1b35 100%);
  padding: 24px;
}

/* 卡片 */
.login-card {
  width: 100%;
  max-width: 440px;
  background: #1c1d2e;
  border: 1px solid rgba(255,255,255,0.08);
  border-radius: 16px;
  padding: 36px 32px 28px;
  box-shadow: 0 24px 64px rgba(0,0,0,0.5);
}

/* 头部 */
.login-header {
  text-align: center;
  margin-bottom: 28px;
}
.login-logo {
  display: flex;
  justify-content: center;
  margin-bottom: 12px;
}
.login-title {
  font-size: 22px;
  font-weight: 700;
  color: #f0f0ff;
  margin: 0 0 6px;
  letter-spacing: 0.3px;
}
.login-sub {
  font-size: 13px;
  color: rgba(255,255,255,0.35);
  margin: 0;
}

/* 表单 */
.login-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
  margin-bottom: 24px;
}

.field-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.field-label {
  font-size: 13px;
  font-weight: 500;
  color: rgba(255,255,255,0.6);
}
.field-input {
  width: 100%;
  background: rgba(255,255,255,0.05);
  border: 1px solid rgba(255,255,255,0.1);
  border-radius: 8px;
  padding: 10px 14px;
  font-size: 14px;
  color: #e8e8f0;
  outline: none;
  transition: border-color 0.2s;
  box-sizing: border-box;
}
.field-input:focus {
  border-color: #6366f1;
}
.field-input::placeholder {
  color: rgba(255,255,255,0.2);
}

/* 密码行 */
.secret-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.secret-row .field-input {
  flex: 1;
}
.icon-btn {
  flex-shrink: 0;
  width: 38px;
  height: 38px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(255,255,255,0.05);
  border: 1px solid rgba(255,255,255,0.1);
  border-radius: 8px;
  color: rgba(255,255,255,0.4);
  cursor: pointer;
  transition: all 0.2s;
}
.icon-btn:hover {
  background: rgba(255,255,255,0.1);
  color: rgba(255,255,255,0.7);
}

/* 错误提示 */
.error-callout {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  background: rgba(239,68,68,0.1);
  border: 1px solid rgba(239,68,68,0.25);
  border-radius: 8px;
  padding: 12px 14px;
  font-size: 13px;
  color: #fca5a5;
}
.error-callout__icon {
  flex-shrink: 0;
  margin-top: 1px;
  color: #f87171;
}
.error-hint {
  margin-top: 6px;
  color: rgba(252,165,165,0.7);
}
.error-hint code {
  background: rgba(255,255,255,0.08);
  padding: 2px 6px;
  border-radius: 4px;
  font-family: monospace;
  font-size: 12px;
  color: #e2e8f0;
}

/* 连接按钮 */
.connect-btn {
  width: 100%;
  padding: 12px;
  background: linear-gradient(135deg, #6366f1, #4f46e5);
  border: none;
  border-radius: 8px;
  color: white;
  font-size: 15px;
  font-weight: 600;
  cursor: pointer;
  transition: opacity 0.2s, transform 0.1s;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
}
.connect-btn:hover:not(:disabled) {
  opacity: 0.9;
}
.connect-btn:active:not(:disabled) {
  transform: scale(0.99);
}
.connect-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.spin {
  display: inline-block;
  animation: spin 0.8s linear infinite;
}
@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

/* 连接指引 */
.guide-section {
  border-top: 1px solid rgba(255,255,255,0.07);
  padding-top: 20px;
}
.guide-title {
  font-size: 12px;
  font-weight: 600;
  color: rgba(255,255,255,0.3);
  text-transform: uppercase;
  letter-spacing: 0.8px;
  margin-bottom: 12px;
}
.guide-steps {
  margin: 0 0 12px;
  padding-left: 18px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.guide-steps li {
  font-size: 13px;
  color: rgba(255,255,255,0.45);
  line-height: 1.5;
}
.code-block {
  display: block;
  margin-top: 5px;
  background: rgba(255,255,255,0.05);
  border: 1px solid rgba(255,255,255,0.07);
  border-radius: 6px;
  padding: 7px 12px;
  font-family: 'SF Mono', 'Consolas', monospace;
  font-size: 13px;
  color: #a5f3fc;
  letter-spacing: 0.3px;
  user-select: all;
}
.guide-tip {
  font-size: 12px;
  color: rgba(255,255,255,0.25);
  padding: 8px 12px;
  background: rgba(255,255,255,0.03);
  border-radius: 6px;
  line-height: 1.5;
}

/* 页脚 */
.login-footer {
  font-size: 12px;
  color: rgba(255,255,255,0.2);
  margin: 0;
  text-align: center;
}
.ver {
  font-family: monospace;
  background: rgba(255,255,255,0.06);
  padding: 1px 6px;
  border-radius: 3px;
  margin: 0 2px;
}
</style>
