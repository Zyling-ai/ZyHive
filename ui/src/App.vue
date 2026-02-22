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
        <a href="https://github.com/Zyling-ai/zyhive" target="_blank" class="header-star-btn" title="Star on GitHub">
          ★<template v-if="starCount !== null"> {{ starCount.toLocaleString() }}</template>
          <span class="header-hide-xs"> Star</span>
        </a>
        <el-divider direction="vertical" style="margin:0 4px;border-color:rgba(255,255,255,0.2)" />
        <span class="header-link" style="cursor:pointer" @click="logout" title="退出登录">
          退出
        </span>
      </div>
    </el-header>

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
          <el-menu-item index="/">
            <el-icon><HomeFilled /></el-icon>
            <template #title>仪表盘</template>
          </el-menu-item>

          <el-menu-item index="/agents">
            <el-icon><User /></el-icon>
            <template #title>AI 成员</template>
          </el-menu-item>

          <el-menu-item index="/team">
            <el-icon><Share /></el-icon>
            <template #title>团队</template>
          </el-menu-item>

          <el-menu-item index="/projects">
            <el-icon><Folder /></el-icon>
            <template #title>项目</template>
          </el-menu-item>

          <el-menu-item index="/chats">
            <el-icon><ChatLineRound /></el-icon>
            <template #title>对话管理</template>
          </el-menu-item>

          <el-menu-item index="/skills">
            <el-icon><MagicStick /></el-icon>
            <template #title>技能</template>
          </el-menu-item>

          <el-sub-menu index="config" v-if="!collapsed || isMobile">
            <template #title>
              <el-icon><Setting /></el-icon>
              <span>全局配置</span>
            </template>
            <el-menu-item index="/config/models">
              <el-icon><Cpu /></el-icon>
              <template #title>模型</template>
            </el-menu-item>
            <el-menu-item index="/config/tools">
              <el-icon><SetUp /></el-icon>
              <template #title>能力</template>
            </el-menu-item>
          </el-sub-menu>

          <!-- Collapsed desktop: show config sub-items flat -->
          <template v-if="collapsed && !isMobile">
            <el-menu-item index="/config/models">
              <el-icon><Cpu /></el-icon>
              <template #title>模型</template>
            </el-menu-item>
            <el-menu-item index="/config/tools">
              <el-icon><SetUp /></el-icon>
              <template #title>能力</template>
            </el-menu-item>
          </template>

          <el-divider style="margin: 8px 0" />

          <el-menu-item index="/cron">
            <el-icon><Timer /></el-icon>
            <template #title>定时任务</template>
          </el-menu-item>

          <el-menu-item index="/tasks">
            <el-icon><Operation /></el-icon>
            <template #title>后台任务</template>
          </el-menu-item>

          <el-menu-item index="/logs">
            <el-icon><Document /></el-icon>
            <template #title>日志</template>
          </el-menu-item>

          <el-menu-item index="/settings">
            <el-icon><Tools /></el-icon>
            <template #title>设置</template>
          </el-menu-item>
        </el-menu>

        <!-- Sidebar footer -->
        <div class="sidebar-footer">
          <span v-if="!collapsed || isMobile" class="sidebar-copyright">© 2026 引巢 · ZyHive</span>
          <span v-else class="sidebar-copyright-mini">© 26</span>
        </div>
      </el-aside>

      <!-- Main content -->
      <el-container>
        <el-main class="app-main">
          <router-view />
        </el-main>
      </el-container>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const route = useRoute()
const router = useRouter()
const collapsed = ref(false)
const starCount = ref<number | null>(null)
const isMobile = ref(false)
const mobileDrawerOpen = ref(false)

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

function onLogoClick() {
  if (isMobile.value) {
    mobileDrawerOpen.value = false
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
    const res = await fetch('https://api.github.com/repos/Zyling-ai/zyhive')
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
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  background: #f5f7fa;
}
#app { min-height: 100vh; }

/* ─── Layout ─────────────────────────────────────────────────────────────── */
.app-layout {
  min-height: 100vh;
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
  position: relative;
}

/* ─── Header ─────────────────────────────────────────────────────────────── */
.header-left { display: flex; align-items: center; gap: 8px; }
.header-title { color: rgba(255,255,255,0.85); font-size: 14px; font-weight: 600; white-space: nowrap; }
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
  background: rgba(255,215,0,0.12);
  color: #ffd700;
  border: 1px solid rgba(255,215,0,0.3);
  border-radius: 4px;
  padding: 2px 8px;
  font-size: 12px;
  font-weight: 600;
  text-decoration: none;
  cursor: pointer;
  transition: background 0.15s;
  white-space: nowrap;
  display: flex;
  align-items: center;
  gap: 3px;
}
.header-star-btn:hover { background: rgba(255,215,0,0.22); }
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
  height: 60px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  cursor: pointer;
  border-bottom: 1px solid rgba(255,255,255,0.08);
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
}
.sidebar-menu .el-menu-item,
.sidebar-menu .el-sub-menu__title {
  color: rgba(255,255,255,0.65) !important;
}
.sidebar-menu .el-menu-item:hover,
.sidebar-menu .el-sub-menu__title:hover {
  background: rgba(255,255,255,0.08) !important;
  color: #fff !important;
}
.sidebar-menu .el-menu-item.is-active {
  background: #409eff !important;
  color: #fff !important;
  border-radius: 4px;
  margin: 2px 8px;
  width: calc(100% - 16px);
}
.sidebar-menu .el-sub-menu .el-menu { background: transparent !important; }
.sidebar-menu .el-sub-menu .el-menu .el-menu-item { padding-left: 48px !important; }
.sidebar-menu .el-divider { border-color: rgba(255,255,255,0.08); }
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
