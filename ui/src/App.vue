<template>
  <div v-if="isLoginPage || isPublicPage">
    <router-view />
  </div>
  <el-container v-else class="app-layout">
    <!-- Top header -->
    <el-header class="app-header" height="44px">
      <div class="header-left">
        <!-- Hamburger: mobile only -->
        <button class="hamburger-btn" @click="toggleMobileDrawer" aria-label="菜单">
          <span class="hamburger-line" :class="{ open: mobileDrawerOpen }"></span>
          <span class="hamburger-line" :class="{ open: mobileDrawerOpen }"></span>
          <span class="hamburger-line" :class="{ open: mobileDrawerOpen }"></span>
        </button>
        <span class="header-title">引巢 · ZyHive</span>
        <span v-if="appVersion" class="header-version">{{ appVersion }}</span>
      </div>
      <div class="header-right">
        <a href="https://zyling.ai" target="_blank" class="header-link header-website-btn header-hide-xs" title="官网">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor" style="vertical-align:-2px">
            <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v1.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z"/>
          </svg>
          官网
        </a>
        <a href="https://github.com/Zyling-ai/zyhive" target="_blank" class="header-link header-hide-sm" title="GitHub">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" style="vertical-align:-2px">
            <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0 0 24 12c0-6.63-5.37-12-12-12z"/>
          </svg>
          GitHub
        </a>
        <span class="header-star-btn">
          ★<template v-if="starCount !== null"> {{ starCount.toLocaleString() }}</template>
          <span class="header-hide-xs"> Star</span>
        </span>
        <!-- Update available badge → 一键升级 -->
        <span
          v-if="updateInfo && !showUpgradeBanner"
          class="header-update-btn"
          style="cursor:pointer"
          :title="`一键升级到 ${updateInfo.latest}`"
          @click="handleTopUpdateClick"
        >
          <span class="update-dot"></span>
          <span class="header-hide-xs">升级到 {{ updateInfo.latest }}</span>
          <span class="header-xs-only" style="display:none">↑</span>
        </span>
        <!-- Upgrade in progress/done hint (clicks → detailed progress) -->
        <span
          v-else-if="showUpgradeBanner"
          class="header-update-btn header-update-running"
          style="cursor:pointer"
          :title="`升级${updateStageLabel}`"
          @click="router.push('/settings')"
        >
          <span class="update-dot update-dot-running"></span>
          <span class="header-hide-xs">
            {{ updater.restartDetected.value ? '重启完成' : updateStageLabel }}
            {{ updater.updateStatus.value?.progress != null ? updater.updateStatus.value.progress + '%' : '' }}
          </span>
        </span>
        <el-divider direction="vertical" style="margin:0 4px;border-color:rgba(255,255,255,0.2)" />
        <span class="header-link" style="cursor:pointer" @click="logout" title="退出登录">
          退出
        </span>
      </div>
    </el-header>

    <!-- ══ 全局升级进度横幅 ════════════════════════════════════════════════════ -->
    <!-- 顶部一键升级 / Settings 页手动升级, 共用同一份状态机 (useUpdater) -->
    <transition name="fade">
      <div v-if="showUpgradeBanner" class="upgrade-banner" :class="{
        'is-done': updater.updateStatus.value?.stage === 'done' && updater.restartDetected.value,
        'is-failed': updater.updateStatus.value?.stage === 'failed',
        'is-rolledback': updater.updateStatus.value?.stage === 'rolledback',
      }">
        <div class="upgrade-banner-inner">
          <!-- 左：状态文本 -->
          <div class="upgrade-banner-text">
            <template v-if="updater.updateStatus.value?.stage === 'done' && updater.restartDetected.value">
              ✅ 服务已重启，新版本 {{ updater.currentVersion.value }} 运行中
            </template>
            <template v-else-if="updater.updateStatus.value?.stage === 'done'">
              ⏳ 升级成功，等待服务重启中…
            </template>
            <template v-else-if="updater.updateStatus.value?.stage === 'failed'">
              ❌ 升级失败：{{ updater.updateStatus.value?.message }}
            </template>
            <template v-else-if="updater.updateStatus.value?.stage === 'rolledback'">
              ↩️ 已回滚：{{ updater.updateStatus.value?.message }}
            </template>
            <template v-else>
              <span class="banner-spinner"></span>
              {{ updateStageLabel }} —— {{ updater.updateStatus.value?.message || '正在处理…' }}
            </template>
          </div>
          <!-- 中：进度条 -->
          <div v-if="!updater.restartDetected.value && updater.updateStatus.value?.stage !== 'failed' && updater.updateStatus.value?.stage !== 'rolledback'"
            class="upgrade-banner-progress">
            <div class="upgrade-banner-progress-bar"
              :style="{ width: (updater.updateStatus.value?.progress ?? 0) + '%' }"></div>
          </div>
          <!-- 右：动作按钮 -->
          <div class="upgrade-banner-actions">
            <button v-if="updater.restartDetected.value"
              class="banner-btn banner-btn-primary" @click="reloadAfterUpgrade">
              刷新页面
            </button>
            <button v-else-if="['failed','rolledback'].includes(updater.updateStatus.value?.stage ?? '')"
              class="banner-btn" @click="router.push('/settings')">
              查看详情
            </button>
            <span v-else class="upgrade-banner-pct">{{ updater.updateStatus.value?.progress ?? 0 }}%</span>
          </div>
        </div>
      </div>
    </transition>

    <el-container class="app-body">
      <!-- Mobile overlay backdrop -->
      <transition name="fade">
        <div v-if="mobileDrawerOpen" class="mobile-overlay" @click="mobileDrawerOpen = false"></div>
      </transition>

      <!-- Sidebar: desktop = persistent; mobile = fixed overlay drawer -->
      <el-aside
        :width="sidebarWidth"
        class="app-sidebar"
        :class="{ 'mobile-drawer': true, 'mobile-drawer-open': mobileDrawerOpen }"
      >
        <div class="sidebar-logo" @click="onLogoClick">
          <span class="logo-icon">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
              <path d="M12 2L21.5 7.5V16.5L12 22L2.5 16.5V7.5L12 2Z" fill="#409EFF"/>
              <text x="12" y="16" text-anchor="middle" fill="white" font-size="10" font-weight="800" font-family="sans-serif">Z</text>
            </svg>
          </span>
          <span v-if="!collapsed" class="logo-text">ZyHive</span>
        </div>

        <el-menu
          :default-active="activeMenu"
          :collapse="collapsed && !isMobile"
          :collapse-transition="false"
          router
          class="sidebar-menu"
          @select="onMenuSelect"
        >
          <!-- 聊天（默认首页） -->
          <el-menu-item index="/">
            <el-icon>
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>
              </svg>
            </el-icon>
            <template #title>聊天</template>
          </el-menu-item>

          <!-- 仪表盘 -->
          <el-menu-item index="/dashboard">
            <el-icon><HomeFilled /></el-icon>
            <template #title>仪表盘</template>
          </el-menu-item>

          <el-divider style="margin: 6px 0" />

          <!-- 团队 -->
          <el-menu-item index="/agents">
            <el-icon><User /></el-icon>
            <template #title>成员</template>
          </el-menu-item>

          <el-menu-item index="/team">
            <el-icon><Share /></el-icon>
            <template #title>通讯录</template>
          </el-menu-item>

          <el-menu-item index="/goals">
            <el-icon><Flag /></el-icon>
            <template #title>规划</template>
          </el-menu-item>

          <el-menu-item index="/projects">
            <el-icon><Folder /></el-icon>
            <template #title>项目</template>
          </el-menu-item>

          <el-divider style="margin: 6px 0" />

          <!-- 工作 -->
          <el-menu-item index="/chats">
            <el-icon><ChatLineRound /></el-icon>
            <template #title>对话管理</template>
          </el-menu-item>

          <el-menu-item index="/skills">
            <el-icon><MagicStick /></el-icon>
            <template #title>技能管理</template>
          </el-menu-item>

          <el-menu-item index="/cron">
            <el-icon><Timer /></el-icon>
            <template #title>定时任务</template>
          </el-menu-item>

          <el-menu-item index="/tasks">
            <el-icon><Operation /></el-icon>
            <template #title>后台任务</template>
          </el-menu-item>

          <el-divider style="margin: 6px 0" />

          <!-- 系统 -->
          <el-menu-item index="/config/models">
            <el-icon><Cpu /></el-icon>
            <template #title>模型配置</template>
          </el-menu-item>

          <el-menu-item index="/config/tools">
            <el-icon><SetUp /></el-icon>
            <template #title>密钥管理</template>
          </el-menu-item>

          <el-menu-item index="/logs">
            <el-icon><Document /></el-icon>
            <template #title>日志</template>
          </el-menu-item>

          <el-menu-item index="/usage">
            <el-icon><TrendCharts /></el-icon>
            <template #title>用量统计</template>
          </el-menu-item>

          <el-menu-item index="/settings">
            <el-icon><Tools /></el-icon>
            <template #title>系统设置</template>
          </el-menu-item>
        </el-menu>

        <!-- Sidebar footer -->
        <div class="sidebar-footer">
          <span v-if="!collapsed || isMobile" class="sidebar-copyright">© 2026 引巢 · ZyHive</span>
          <span v-else class="sidebar-copyright-mini">© 26</span>
        </div>
      </el-aside>

      <!-- Main content -->
      <el-container class="app-right-container">
        <el-main class="app-main" :class="{ 'is-chat-page': isChatPage }">
          <router-view @toggle-sidebar="onToggleSidebar" />
        </el-main>
      </el-container>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from './api'
import { useUpdater } from './composables/useUpdater'

const route = useRoute()
const router = useRouter()
const collapsed = ref(false)
const starCount = ref<number | null>(null)
const appVersion = ref('')
const updateInfo = ref<{ latest: string; releaseUrl: string } | null>(null)
const isMobile = ref(false)
const mobileDrawerOpen = ref(false)

// 全局升级器：顶栏按钮点击直接弹确认 → 一键升级，进度条显示在 header 下方 banner
const updater = useUpdater()
const headerUpdateBusy = ref(false)  // 防重入（防止用户双击）

// UI 辅助：把 stage 翻成人话
const updateStageLabel = computed(() => {
  const s = updater.updateStatus.value?.stage
  switch (s) {
    case 'downloading': return '下载中'
    case 'verifying':   return '验证中'
    case 'applying':    return '替换文件'
    case 'done':        return '升级完成'
    case 'failed':      return '升级失败'
    case 'rolledback':  return '已回滚'
    default: return ''
  }
})

// 什么时候显示顶部 upgrade banner：
//   1. 有 updateStatus 且 stage 非 idle → 在进行中或刚结束
//   2. 或者 updateRunning=true（启动但还没来得及返回第一次 status）
const showUpgradeBanner = computed(() => {
  if (updater.updateRunning.value) return true
  const st = updater.updateStatus.value
  return !!st && st.stage !== 'idle'
})

// 顶栏点「新版本 XXX」按钮
async function handleTopUpdateClick() {
  if (headerUpdateBusy.value) return
  if (!updateInfo.value) return
  // 已经在升级或等重启中 → 不重复触发，跳转到 settings 让用户看详细进度
  if (updater.updateRunning.value || showUpgradeBanner.value) {
    router.push('/settings')
    return
  }
  headerUpdateBusy.value = true
  try {
    await ElMessageBox.confirm(
      `确认将 ZyHive 从 ${appVersion.value} 升级到 ${updateInfo.value.latest}？\n\n升级过程中服务将短暂重启（约 10-30 秒），成员数据和配置文件不受影响。`,
      '确认升级',
      { confirmButtonText: '立即升级', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    headerUpdateBusy.value = false
    return  // 用户取消
  }
  try {
    await updater.startUpgrade(updateInfo.value.latest)
    ElMessage.success('升级已启动，进度见顶部横幅')
    // 升级期间清掉 localStorage 缓存，避免 restart 后还显示"有新版本"
    // (同时清旧 key 以防本机有遗留)
    localStorage.removeItem('zyhive_update_info_v2')
    localStorage.removeItem('zyhive_update_exp_v2')
    localStorage.removeItem('zyhive_update_info')
    localStorage.removeItem('zyhive_update_exp')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '启动升级失败')
  } finally {
    headerUpdateBusy.value = false
  }
}

// 升级完成后一键刷新
function reloadAfterUpgrade() {
  updater.reloadPage()
}

// 当升级成功（restartDetected）且用户还没手动刷 → 清本地缓存 + 给个 toast
watch(() => updater.restartDetected.value, (v) => {
  if (v) {
    localStorage.removeItem('zyhive_update_info_v2')
    localStorage.removeItem('zyhive_update_exp_v2')
    localStorage.removeItem('zyhive_update_info')
    localStorage.removeItem('zyhive_update_exp')
    updateInfo.value = null
    appVersion.value = updater.currentVersion.value
  }
})

const MOBILE_BREAKPOINT = 768

function checkMobile() {
  isMobile.value = window.innerWidth <= MOBILE_BREAKPOINT
  if (!isMobile.value) mobileDrawerOpen.value = false
}

const isLoginPage = computed(() => route.path === '/login')
const isPublicPage = computed(() => !!route.meta.public)

const sidebarWidth = computed(() => {
  if (isMobile.value) return '220px'
  return collapsed.value ? '64px' : '200px'
})

const activeMenu = computed(() => {
  const path = route.path
  if (path.startsWith('/agents/')) return '/agents'
  return path
})

// 聊天页：撑满高度，不需要 padding
const isChatPage = computed(() => route.path === '/')

function onLogoClick() {
  if (isMobile.value) {
    mobileDrawerOpen.value = false
  } else {
    collapsed.value = !collapsed.value
  }
}

function onToggleSidebar() {
  if (isMobile.value) {
    mobileDrawerOpen.value = !mobileDrawerOpen.value
  } else {
    collapsed.value = !collapsed.value
  }
}

function toggleMobileDrawer() {
  mobileDrawerOpen.value = !mobileDrawerOpen.value
}

function onMenuSelect() {
  if (isMobile.value) mobileDrawerOpen.value = false
}

// Close drawer on route change
watch(() => route.path, () => {
  if (isMobile.value) mobileDrawerOpen.value = false
})

function logout() {
  localStorage.removeItem('aipanel_token')
  router.push('/login')
}

// Fetch real-time GitHub star count (cached 10min in localStorage)
onMounted(async () => {
  checkMobile()
  window.addEventListener('resize', checkMobile)

  // Fetch current version
  try {
    const vRes = await api.get('/version')
    appVersion.value = vRes.data.version
  } catch { /* ignore */ }

  // 初始化全局升级器 — 刷新页面时若后端已有进行中任务，顶部 banner 会自动出现
  if (localStorage.getItem('aipanel_token')) {
    updater.initFromBackend().catch(() => {/* non-critical */})
  }

  // Check for updates (delayed 2s, cached 1h in localStorage)
  setTimeout(async () => {
    // Skip update check if not logged in — avoids 401 redirect loop on fresh installs
    if (!localStorage.getItem('aipanel_token')) return
    // 26.4.23v7: bump cache key (v2) so the old-parser-era cache is invalidated
    // on first load after this deploy. Old keys ('zyhive_update_info') become
    // orphan and expire naturally.
    const uCacheKey = 'zyhive_update_info_v2'
    const uCacheExp = 'zyhive_update_exp_v2'
    const now = Date.now()
    const cached = localStorage.getItem(uCacheKey)
    const exp = parseInt(localStorage.getItem(uCacheExp) || '0')
    // semver compare: a > b (e.g. v0.9.26 > v0.9.24)
    // 对齐 internal/api/update.go::semverGt —— 支持两种格式:
    //   1. 语义版本 v0.9.26         → [0, 9, 26, 0]
    //   2. 日期版本 26.4.23v6       → [26, 4, 23, 6]  (YY.M.D + vN 修订号)
    // 原版 parse 对 "26.4.23v6" 会把最后一段 Number('23v6') → NaN,
    // 导致 "26.4.23v6 > 26.4.23v5" 判成 false, 顶栏 updateInfo 永远为空.
    const semverGt = (a: string, b: string) => {
      const parse = (s: string): [number, number, number, number] => {
        s = s.replace(/^v/, '')
        // 剥离末尾的 vN 修订号
        let revision = 0
        const m = s.match(/^(.+?)[vV](\d+)$/)
        if (m && m[1] && m[2]) {
          s = m[1]
          revision = parseInt(m[2], 10) || 0
        }
        const p = s.split('.').map(x => parseInt(x, 10) || 0)
        return [p[0] ?? 0, p[1] ?? 0, p[2] ?? 0, revision]
      }
      const av = parse(a), bv = parse(b)
      for (let i = 0; i < 4; i++) {
        if ((av[i] ?? 0) > (bv[i] ?? 0)) return true
        if ((av[i] ?? 0) < (bv[i] ?? 0)) return false
      }
      return false
    }
    const current = appVersion.value
    if (cached && now < exp) {
      const parsed = JSON.parse(cached)
      // 验证缓存的 latest 确实大于当前运行版本，否则丢弃缓存
      if (parsed?.hasUpdate && parsed.latest && current && semverGt(parsed.latest, current)) {
        updateInfo.value = { latest: parsed.latest, releaseUrl: parsed.releaseUrl }
        return
      }
      // 缓存无效（版本已更新），清掉并重新检查
      localStorage.removeItem(uCacheKey)
      localStorage.removeItem(uCacheExp)
    }
    try {
      const res = await api.get('/update/check')
      const d = res.data
      localStorage.setItem(uCacheKey, JSON.stringify(d))
      localStorage.setItem(uCacheExp, String(now + 60 * 60 * 1000)) // 1h
      if (d?.hasUpdate && semverGt(d.latest, current)) updateInfo.value = { latest: d.latest, releaseUrl: d.releaseUrl }
    } catch { /* ignore, non-critical */ }
  }, 2000)

  const cacheKey = 'zyhive_gh_stars'
  const cacheExp = 'zyhive_gh_stars_exp'
  const now = Date.now()
  const cached = localStorage.getItem(cacheKey)
  const exp = parseInt(localStorage.getItem(cacheExp) || '0')
  if (cached && now < exp) {
    starCount.value = parseInt(cached)
    return
  }
  try {
    const res = await fetch('https://api.github.com/repos/Zyling-ai/ZyHive')
    if (res.ok) {
      const data = await res.json()
      starCount.value = data.stargazers_count ?? null
      localStorage.setItem(cacheKey, String(starCount.value))
      localStorage.setItem(cacheExp, String(now + 10 * 60 * 1000))
    }
  } catch { /* ignore */ }
})

onUnmounted(() => {
  window.removeEventListener('resize', checkMobile)
})
</script>

<style>
/* ─── Reset ──────────────────────────────────────────────────────────────── */
body {
  margin: 0;
  font-family: -apple-system, BlinkMacSystemFont, 'Inter', 'PingFang SC', 'Segoe UI', Roboto, sans-serif;
  background: #fafafa;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  color: #222;
}
#app { min-height: 100vh; }

/* ─── 全局细滚动条（WebKit / Blink）──────────────────────────────────────
   目的: 替换 Element Plus 默认的粗白色滚动条, 参考 Cursor 风格极简 */
::-webkit-scrollbar {
  width: 6px;
  height: 6px;
  background: transparent;
}
::-webkit-scrollbar-thumb {
  background: rgba(0, 0, 0, 0.08);
  border-radius: 3px;
  transition: background 0.15s;
}
::-webkit-scrollbar-thumb:hover { background: rgba(0, 0, 0, 0.25); }
::-webkit-scrollbar-track { background: transparent; }
::-webkit-scrollbar-corner { background: transparent; }
/* Firefox */
* {
  scrollbar-width: thin;
  scrollbar-color: rgba(0, 0, 0, 0.1) transparent;
}
/* 深色容器（sidebar 等）的滚动条用白色半透明 */
.app-sidebar ::-webkit-scrollbar-thumb,
.sidebar-menu::-webkit-scrollbar-thumb { background: rgba(255,255,255,0.08); }
.app-sidebar ::-webkit-scrollbar-thumb:hover,
.sidebar-menu::-webkit-scrollbar-thumb:hover { background: rgba(255,255,255,0.2); }
.app-sidebar, .sidebar-menu {
  scrollbar-color: rgba(255,255,255,0.1) transparent;
}

/* Element Plus 组件内滚动条继承 */
.el-scrollbar__bar { opacity: 0.4 !important; }
.el-scrollbar__bar:hover { opacity: 0.9 !important; }
.el-scrollbar__thumb { background: rgba(0,0,0,0.15) !important; }

/* ─── Layout ─────────────────────────────────────────────────────────────── */
.app-layout {
  height: 100vh;         /* 固定视口高度，flex 子元素 flex:1 才能生效 */
  max-height: 100vh;
  overflow: hidden;
  flex-direction: column !important;
}
.app-header {
  background: #1a1b2e;
  border-bottom: 1px solid rgba(255,255,255,0.08);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 16px;
  flex-shrink: 0;
  z-index: 100;
}
.app-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;   /* 防止子元素撑破 */
  position: relative;
}

/* ─── Header ─────────────────────────────────────────────────────────────── */
.header-left { display: flex; align-items: center; gap: 8px; }
.header-title { color: rgba(255,255,255,0.85); font-size: 14px; font-weight: 600; white-space: nowrap; }
.header-version { font-size: 11px; color: rgba(255,255,255,0.35); font-family: monospace; white-space: nowrap; }
.header-update-btn {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 3px 9px;
  border-radius: 12px;
  background: rgba(52, 211, 153, 0.15);
  border: 1px solid rgba(52, 211, 153, 0.4);
  color: #34d399;
  font-size: 12px;
  font-weight: 500;
  text-decoration: none;
  white-space: nowrap;
  transition: background 0.2s, border-color 0.2s;
  cursor: pointer;
}
.header-update-btn:hover {
  background: rgba(52, 211, 153, 0.25);
  border-color: rgba(52, 211, 153, 0.7);
  color: #6ee7b7;
}
.update-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #34d399;
  flex-shrink: 0;
  animation: update-pulse 2s ease-in-out infinite;
}
@keyframes update-pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.5; transform: scale(0.75); }
}

/* Running state: 蓝色 (区别于"可升级"的绿色) */
.header-update-btn.header-update-running {
  background: rgba(59, 130, 246, 0.15);
  border-color: rgba(59, 130, 246, 0.4);
  color: #60a5fa;
}
.header-update-btn.header-update-running:hover {
  background: rgba(59, 130, 246, 0.25);
  color: #93c5fd;
}
.update-dot.update-dot-running {
  background: #60a5fa;
  animation: update-spin 1s linear infinite;
}
@keyframes update-spin {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.3; }
}

/* ══ Global upgrade banner ════════════════════════════════════════════════ */
.upgrade-banner {
  background: #eff6ff;
  border-bottom: 1px solid #bfdbfe;
  color: #1e3a8a;
  padding: 10px 20px;
  font-size: 13px;
  position: relative;
  z-index: 5;
}
.upgrade-banner.is-done { background: #ecfdf5; border-color: #a7f3d0; color: #065f46; }
.upgrade-banner.is-failed { background: #fef2f2; border-color: #fecaca; color: #991b1b; }
.upgrade-banner.is-rolledback { background: #fffbeb; border-color: #fde68a; color: #92400e; }
.upgrade-banner-inner {
  display: flex;
  align-items: center;
  gap: 16px;
  max-width: 100%;
}
.upgrade-banner-text { flex-shrink: 0; font-weight: 500; }
.upgrade-banner-progress {
  flex: 1;
  height: 6px;
  background: rgba(30, 58, 138, 0.15);
  border-radius: 3px;
  overflow: hidden;
  min-width: 100px;
}
.is-done .upgrade-banner-progress { background: rgba(6, 95, 70, 0.15); }
.upgrade-banner-progress-bar {
  height: 100%;
  background: #3b82f6;
  border-radius: 3px;
  transition: width 0.35s ease-out;
}
.is-done .upgrade-banner-progress-bar { background: #10b981; }
.upgrade-banner-actions { flex-shrink: 0; display: flex; align-items: center; gap: 8px; }
.upgrade-banner-pct { font-size: 12px; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; min-width: 40px; text-align: right; }
.banner-btn {
  padding: 4px 12px;
  border-radius: 6px;
  border: 1px solid currentColor;
  background: transparent;
  color: inherit;
  font-size: 12px;
  cursor: pointer;
  transition: all 0.15s;
}
.banner-btn:hover { background: rgba(0,0,0,0.05); }
.banner-btn-primary {
  background: #10b981;
  border-color: #10b981;
  color: #fff;
}
.banner-btn-primary:hover { background: #059669; border-color: #059669; }
.banner-spinner {
  display: inline-block;
  width: 10px; height: 10px;
  border: 2px solid currentColor;
  border-top-color: transparent;
  border-radius: 50%;
  animation: banner-spin 0.7s linear infinite;
  margin-right: 6px;
  vertical-align: -1px;
}
@keyframes banner-spin { to { transform: rotate(360deg); } }

.fade-enter-active, .fade-leave-active { transition: opacity 0.3s, transform 0.3s; }
.fade-enter-from, .fade-leave-to { opacity: 0; transform: translateY(-8px); }

@media (max-width: 640px) {
  .upgrade-banner-progress { display: none; }
  .upgrade-banner-inner { flex-wrap: wrap; gap: 8px; }
}
.header-right { display: flex; align-items: center; gap: 10px; }
.header-link {
  color: rgba(255,255,255,0.55);
  text-decoration: none;
  font-size: 13px;
  display: flex;
  align-items: center;
  gap: 4px;
  transition: color 0.15s;
  white-space: nowrap;
}
.header-link:hover { color: #fff; }
.header-star-btn {
  background: transparent;
  color: rgba(255,255,255,0.45);
  border: 1px solid rgba(255,255,255,0.12);
  border-radius: 4px;
  padding: 2px 8px;
  font-size: 12px;
  font-weight: 400;
  white-space: nowrap;
  display: flex;
  align-items: center;
  gap: 3px;
  user-select: none;
  transition: color 0.15s, border-color 0.15s;
}
.header-star-btn:hover {
  color: rgba(255,255,255,0.75);
  border-color: rgba(255,255,255,0.25);
}
.header-website-btn {
  background: rgba(99,102,241,0.12);
  border: 1px solid rgba(99,102,241,0.3);
  border-radius: 4px;
  padding: 2px 8px;
  color: #a5b4fc !important;
}
.header-website-btn:hover { background: rgba(99,102,241,0.22); color: #fff !important; }

/* ─── Hamburger ──────────────────────────────────────────────────────────── */
.hamburger-btn {
  display: none; /* desktop: hidden */
  flex-direction: column;
  justify-content: center;
  gap: 5px;
  width: 32px;
  height: 32px;
  padding: 4px;
  background: none;
  border: none;
  cursor: pointer;
  flex-shrink: 0;
}
.hamburger-line {
  display: block;
  width: 22px;
  height: 2px;
  background: rgba(255,255,255,0.7);
  border-radius: 2px;
  transition: transform 0.22s, opacity 0.22s;
  transform-origin: center;
}
.hamburger-line:nth-child(1).open { transform: translateY(7px) rotate(45deg); }
.hamburger-line:nth-child(2).open { opacity: 0; transform: scaleX(0); }
.hamburger-line:nth-child(3).open { transform: translateY(-7px) rotate(-45deg); }

/* ─── Sidebar ────────────────────────────────────────────────────────────── */
.app-sidebar {
  background: #1d1e2c;
  transition: width 0.2s;
  overflow: hidden;
  display: flex !important;
  flex-direction: column;
}
.sidebar-logo {
  height: 52px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  cursor: pointer;
  border-bottom: 1px solid rgba(255,255,255,0.06);
  flex-shrink: 0;
}
.logo-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  flex-shrink: 0;
}
.logo-text { font-size: 18px; font-weight: 700; color: #fff; white-space: nowrap; }
.sidebar-menu {
  border-right: none !important;
  background: transparent !important;
  flex: 1;
  overflow-y: auto;
  padding: 4px 8px;
}
.sidebar-menu .el-menu-item,
.sidebar-menu .el-sub-menu__title {
  color: rgba(255,255,255,0.6) !important;
  height: 40px !important;
  line-height: 40px !important;
  border-radius: 6px;
  margin: 2px 0;
  position: relative;
  font-size: 13px !important;
  transition: background 0.15s, color 0.15s;
}
.sidebar-menu .el-menu-item:hover,
.sidebar-menu .el-sub-menu__title:hover {
  background: rgba(255,255,255,0.06) !important;
  color: rgba(255,255,255,0.95) !important;
}
/* Active: 左侧 2px 高亮条 + 柔和高亮背景, 不再是整块蓝色 */
.sidebar-menu .el-menu-item.is-active {
  background: rgba(99,102,241,0.12) !important;
  color: #fff !important;
  font-weight: 500;
}
.sidebar-menu .el-menu-item.is-active::before {
  content: '';
  position: absolute;
  left: 0;
  top: 20%;
  bottom: 20%;
  width: 2px;
  background: #818cf8;
  border-radius: 2px;
}
.sidebar-menu .el-menu-item .el-icon,
.sidebar-menu .el-sub-menu__title .el-icon {
  font-size: 15px !important;
  margin-right: 10px !important;
}
.sidebar-menu .el-sub-menu .el-menu { background: transparent !important; }
.sidebar-menu .el-sub-menu .el-menu .el-menu-item { padding-left: 42px !important; }
.sidebar-menu .el-divider { border-color: rgba(255,255,255,0.06); margin: 8px 6px !important; }
.sidebar-footer {
  padding: 12px 16px;
  border-top: 1px solid rgba(255,255,255,0.08);
  margin-top: auto;
  flex-shrink: 0;
}
.sidebar-copyright { font-size: 11px; color: rgba(255,255,255,0.3); white-space: nowrap; display: block; text-align: center; }
.sidebar-copyright-mini { font-size: 10px; color: rgba(255,255,255,0.25); display: block; text-align: center; }

/* ─── Mobile overlay ─────────────────────────────────────────────────────── */
.mobile-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0,0,0,0.5);
  z-index: 999;
  touch-action: none;
}
.fade-enter-active, .fade-leave-active { transition: opacity 0.2s; }
.fade-enter-from, .fade-leave-to { opacity: 0; }

/* ─── Main ───────────────────────────────────────────────────────────────── */
.app-main {
  background: #f5f7fa;
  min-height: calc(100vh - 44px);
  padding: 20px 24px;
  overflow-x: hidden;
}

/* 聊天首页：撑满剩余高度，无内边距，无滚动 */
/* app-right-container 是 app-body (row-flex) 的子元素，
   align-items:stretch 会自动给它 100% 高度，不能设 height:0 */
.app-right-container {
  flex: 1 !important;
  min-width: 0;
  min-height: 0;
  display: flex !important;
  flex-direction: column !important;
  overflow: hidden;
}
/* app-main 是 app-right-container (column-flex) 的子元素，
   flex:1 + min-height:0 撑满剩余高度 */
.app-main.is-chat-page {
  padding: 0 !important;
  margin: 0 !important;
  min-height: 0 !important;
  flex: 1 !important;
  overflow: hidden !important;
  display: flex !important;
  flex-direction: column !important;
}

/* ─── Global mobile table fix ────────────────────────────────────────────── */
.el-table {
  overflow-x: auto;
}

/* ═══════════════════════════════════════════════════════════════════════════
   MOBILE STYLES (≤768px) — does NOT affect desktop
   ═══════════════════════════════════════════════════════════════════════════ */
@media (max-width: 768px) {
  /* Hamburger visible on mobile */
  .hamburger-btn { display: flex; }

  /* Hide some header links on mobile */
  .header-hide-sm { display: none !important; }

  /* Sidebar: fixed overlay drawer on mobile */
  .app-sidebar {
    position: fixed !important;
    left: 0;
    top: 44px;
    height: calc(100vh - 44px);
    z-index: 1000;
    transform: translateX(-100%);
    transition: transform 0.25s ease, width 0.2s;
    width: 220px !important;
    box-shadow: 4px 0 24px rgba(0,0,0,0.35);
  }
  .app-sidebar.mobile-drawer-open {
    transform: translateX(0);
  }

  /* Main content: full width, no left margin */
  .app-body .el-container {
    width: 100% !important;
  }
  .app-main {
    padding: 12px 12px 80px;
    min-height: calc(100vh - 44px);
  }

  /* Shrink el-main padding */
  .el-main { padding: 12px !important; }

  /* Cards: remove fixed min-widths */
  .el-card { min-width: 0 !important; }

  /* Tables: horizontal scroll */
  .el-table-wrapper, .el-table { max-width: 100%; overflow-x: auto !important; }
  .el-table .el-table__body-wrapper { overflow-x: auto; }

  /* Dialogs: full width on mobile */
  .el-dialog { width: 95vw !important; max-width: none !important; margin: 5vh auto !important; }
  .el-dialog__body { padding: 16px !important; }

  /* Drawers: full width */
  .el-drawer { width: 100% !important; }

  /* Form items: stack label + input */
  .el-form-item { flex-direction: column; align-items: flex-start; }
  .el-form-item__label { width: auto !important; text-align: left !important; padding-bottom: 4px; }
  .el-form-item__content { margin-left: 0 !important; width: 100%; }

  /* Page titles */
  h2 { font-size: 18px !important; margin-bottom: 14px !important; }
  h3 { font-size: 15px !important; }

  /* Button row: wrap */
  .el-button + .el-button { margin-left: 6px; }
}

/* Extra small (≤480px) */
@media (max-width: 480px) {
  .header-hide-xs { display: none !important; }
  .app-main { padding: 10px 10px 80px; }
  .el-dialog { width: 98vw !important; }
}
</style>
