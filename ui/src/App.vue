<template>
  <div v-if="isLoginPage || isPublicPage">
    <router-view />
  </div>
  <el-container v-else class="app-layout">
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
          <el-menu-item index="/config/skills">
            <el-icon><MagicStick /></el-icon>
            <template #title>Skills</template>
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
          <el-menu-item index="/config/skills">
            <el-icon><MagicStick /></el-icon>
            <template #title>Skills</template>
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
        <span v-if="!collapsed" class="sidebar-copyright">© 2025 引巢 · ZyHive</span>
        <span v-else class="sidebar-copyright-mini">© 25</span>
      </div>
    </el-aside>

    <!-- Main content -->
    <el-container>
      <el-main class="app-main">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const collapsed = ref(false)

const isLoginPage = computed(() => route.path === '/login')
const isPublicPage = computed(() => !!route.meta.public)

const activeMenu = computed(() => {
  const path = route.path
  // For agent detail, highlight /agents
  if (path.startsWith('/agents/')) return '/agents'
  return path
})
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
}
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
