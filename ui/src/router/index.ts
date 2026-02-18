import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('../views/LoginView.vue')
    },
    {
      path: '/',
      name: 'dashboard',
      component: () => import('../views/DashboardView.vue'),
      meta: { requiresAuth: true }
    },
    {
      path: '/agents',
      name: 'agents',
      component: () => import('../views/AgentsView.vue'),
      meta: { requiresAuth: true }
    },
    {
      path: '/agents/new',
      name: 'agent-create',
      component: () => import('../views/AgentCreateView.vue'),
      meta: { requiresAuth: true }
    },
    {
      path: '/agents/:id',
      name: 'agent-detail',
      component: () => import('../views/AgentDetailView.vue'),
      meta: { requiresAuth: true }
    },
    {
      path: '/config/models',
      name: 'models',
      component: () => import('../views/ModelsView.vue'),
      meta: { requiresAuth: true }
    },
    {
      path: '/config/channels',
      name: 'channels',
      component: () => import('../views/ChannelsView.vue'),
      meta: { requiresAuth: true }
    },
    {
      path: '/config/tools',
      name: 'tools',
      component: () => import('../views/ToolsView.vue'),
      meta: { requiresAuth: true }
    },
    {
      path: '/config/skills',
      name: 'skills',
      component: () => import('../views/SkillsView.vue'),
      meta: { requiresAuth: true }
    },
    {
      path: '/chats',
      name: 'chats',
      component: () => import('../views/ChatsView.vue'),
      meta: { requiresAuth: true }
    },
    {
      path: '/cron',
      name: 'cron',
      component: () => import('../views/CronView.vue'),
      meta: { requiresAuth: true }
    },
    {
      path: '/logs',
      name: 'logs',
      component: () => import('../views/LogsView.vue'),
      meta: { requiresAuth: true }
    },
    {
      path: '/settings',
      name: 'settings',
      component: () => import('../views/SettingsView.vue'),
      meta: { requiresAuth: true }
    },
    // Legacy redirect
    {
      path: '/config',
      redirect: '/config/models'
    }
  ]
})

router.beforeEach((to) => {
  if (to.meta.requiresAuth) {
    const token = localStorage.getItem('aipanel_token')
    if (!token && to.name !== 'login') {
      return true
    }
  }
  return true
})

export default router
