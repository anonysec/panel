import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory('/portal/'),
  routes: [
    {
      path: '/',
      component: () => import('@/layouts/PortalShell.vue'),
      meta: { requiresAuth: true },
      children: [
        { path: '', name: 'portal-dashboard', component: () => import('@/views/DashboardView.vue') },
        { path: 'billing', name: 'portal-billing', component: () => import('@/views/BillingView.vue') },
        { path: 'usage', name: 'portal-usage', component: () => import('@/views/UsageView.vue') },
        { path: 'support', name: 'portal-support', component: () => import('@/views/SupportView.vue') },
        { path: 'profile', name: 'portal-profile', component: () => import('@/views/ProfileView.vue') },
        { path: 'vpn-profiles', name: 'portal-vpn', component: () => import('@/views/VpnProfilesView.vue') },
      ]
    },
    { path: '/login', name: 'portal-login', component: () => import('@/views/LoginView.vue') },
    { path: '/:pathMatch(.*)*', redirect: '/' }
  ]
})

router.beforeEach(async (to) => {
  const { usePortalAuthStore } = await import('@/stores/auth')
  const auth = usePortalAuthStore()

  if (!auth.isAuthenticated && !auth.loading) {
    await auth.checkAuth()
  }

  if (to.meta.requiresAuth && !auth.isAuthenticated) {
    return { name: 'portal-login' }
  }

  if (to.name === 'portal-login' && auth.isAuthenticated) {
    return { name: 'portal-dashboard' }
  }
})

export default router
