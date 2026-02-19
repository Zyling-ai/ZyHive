<template>
  <div v-if="isLoginPage">
    <router-view />
  </div>
  <el-container v-else class="app-layout">
    <!-- Sidebar -->
    <el-aside :width="collapsed ? '64px' : '200px'" class="app-sidebar">
      <div class="sidebar-logo" @click="collapsed = !collapsed">
        <span class="logo-icon">🤖</span>
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
          <el-menu-item index="/config/channels">
            <el-icon><ChatDotRound /></el-icon>
            <template #title>消息通道</template>
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

        <!-- Collapsed: show config sub-items flat (/, /agents, /chats already shown above) -->
        <template v-if="collapsed">
          <el-menu-item index="/config/models">
            <el-icon><Cpu /></el-icon>
            <template #title>模型</template>
          </el-menu-item>
          <el-menu-item index="/config/channels">
            <el-icon><ChatDotRound /></el-icon>
            <template #title>消息通道</template>
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
  font-size: 24px;
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
.app-main {
  background: #f5f7fa;
  min-height: 100vh;
  padding: 20px 24px;
}
</style>
