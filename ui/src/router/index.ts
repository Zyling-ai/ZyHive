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
      path: '/agents/:id',
      name: 'agent-detail',
      component: () => import('../views/AgentDetailView.vue'),
      meta: { requiresAuth: true }
    },
    {
      path: '/config',
      name: 'config',
      component: () => import('../views/ConfigView.vue'),
      meta: { requiresAuth: true }
    }
  ]
})

router.beforeEach((to) => {
  if (to.meta.requiresAuth) {
    const token = localStorage.getItem('aipanel_token')
    // Allow access if no token is required (default "changeme" mode)
    // The backend allows all requests when token is "changeme"
    if (!token && to.name !== 'login') {
      // Check if auth is needed by trying a request
      return true // Allow by default, login page is optional
    }
  }
  return true
})

export default router
