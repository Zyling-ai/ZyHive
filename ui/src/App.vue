<template>
  <div v-if="isLoginPage || isPublicPage">
    <router-view />
  </div>
  <el-container v-else class="app-layout">
    <!-- Top header -->
    <el-header class="app-header" height="44px">
      <div class="header-left">
        <span class="header-title">引巢 · ZyHive</span>
      </div>
      <div class="header-right">
        <a href="https://github.com/sunhuihui6688-star/ai-panel" target="_blank" class="header-link" title="GitHub">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" style="vertical-align:-2px">
            <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0 0 24 12c0-6.63-5.37-12-12-12z"/>
          </svg>
          GitHub
        </a>
        <a href="https://github.com/sunhuihui6688-star/ai-panel" target="_blank" class="header-star-btn" title="Star on GitHub">
          ★ Star
        </a>
        <el-divider direction="vertical" style="margin:0 8px;border-color:rgba(255,255,255,0.2)" />
        <span class="header-link" style="cursor:pointer" @click="logout" title="退出登录">
          退出
        </span>
      </div>
    </el-header>

    <el-container class="app-body">
    <!-- Sidebar -->
    <el-aside :width="collapsed ? '64px' : '200px'" class="app-sidebar">
      <div class="sidebar-logo" @click="collapsed = !collapsed">
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
        :collapse="collapsed"
        :collapse-transition="false"
        router
        class="sidebar-menu"
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

        <el-menu-item index="/chats">
          <el-icon><ChatLineRound /></el-icon>
          <template #title>对话管理</template>
        </el-menu-item>

        <el-menu-item index="/skills">
          <el-icon><MagicStick /></el-icon>
          <template #title>技能</template>
        </el-menu-item>

        <el-sub-menu index="config" v-if="!collapsed">
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

        <!-- Collapsed: show config sub-items flat -->
        <template v-if="collapsed">
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
        <span v-if="!collapsed" class="sidebar-copyright">© 2026 引巢 · ZyHive</span>
        <span v-else class="sidebar-copyright-mini">© 26</span>
      </div>
    </el-aside>

    <!-- Main content -->
    <el-container>
      <el-main class="app-main">
        <router-view />
      </el-main>
    </el-container>
    </el-container><!-- /app-body -->
  </el-container>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const route = useRoute()
const router = useRouter()
const collapsed = ref(false)

const isLoginPage = computed(() => route.path === '/login')
const isPublicPage = computed(() => !!route.meta.public)

const activeMenu = computed(() => {
  const path = route.path
  if (path.startsWith('/agents/')) return '/agents'
  return path
})

function logout() {
  localStorage.removeItem('aipanel_token')
  router.push('/login')
}
</script>

<style>
body {
  margin: 0;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  background: #f5f7fa;
}
#app {
  min-height: 100vh;
}
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
}
.app-body {
  flex: 1;
  min-height: 0;
}
.header-left { display: flex; align-items: center; gap: 8px; }
.header-title { color: rgba(255,255,255,0.85); font-size: 14px; font-weight: 600; }
.header-right { display: flex; align-items: center; gap: 12px; }
.header-link {
  color: rgba(255,255,255,0.55);
  text-decoration: none;
  font-size: 13px;
  display: flex;
  align-items: center;
  gap: 4px;
  transition: color 0.15s;
}
.header-link:hover { color: #fff; }
.header-star-btn {
  background: rgba(255,215,0,0.12);
  color: #ffd700;
  border: 1px solid rgba(255,215,0,0.3);
  border-radius: 4px;
  padding: 2px 10px;
  font-size: 12px;
  font-weight: 600;
  text-decoration: none;
  cursor: pointer;
  transition: background 0.15s;
}
.header-star-btn:hover { background: rgba(255,215,0,0.22); }
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
}
.logo-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  flex-shrink: 0;
}
.logo-text {
  font-size: 18px;
  font-weight: 700;
  color: #fff;
  white-space: nowrap;
}
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
.sidebar-menu .el-sub-menu .el-menu {
  background: transparent !important;
}
.sidebar-menu .el-sub-menu .el-menu .el-menu-item {
  padding-left: 48px !important;
}
.sidebar-menu .el-divider {
  border-color: rgba(255,255,255,0.08);
}
.sidebar-footer {
  padding: 12px 16px;
  border-top: 1px solid rgba(255,255,255,0.08);
  margin-top: auto;
}
.sidebar-copyright {
  font-size: 11px;
  color: rgba(255,255,255,0.3);
  white-space: nowrap;
  display: block;
  text-align: center;
}
.sidebar-copyright-mini {
  font-size: 10px;
  color: rgba(255,255,255,0.25);
  display: block;
  text-align: center;
}
.app-main {
  background: #f5f7fa;
  min-height: 100vh;
  padding: 20px 24px;
}
</style>
