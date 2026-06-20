import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory('/portal/'),
  routes: [
    {
      path: '/',
      component: () => import('@/layouts/PortalShell.vue'),
      meta: { requiresAuth: true },
      children: [
        { path: '', name: 'portal-home', component: () => import('@/views/SinglePageView.vue') },
        { path: 'billing', name: 'portal-billing', component: () => import('@/views/BillingView.vue') },
        { path: 'profile', name: 'portal-profile', component: () => import('@/views/ProfileView.vue') },
        { path: 'wireguard', redirect: '/' },
      ]
    },
    { path: '/login', name: 'portal-login', component: () => import('@/views/LoginView.vue') },
    // Redirect old routes to home
    { path: '/usage', redirect: '/' },
    { path: '/support', redirect: '/' },
    { path: '/vpn-profiles', redirect: '/' },
    { path: '/:pathMatch(.*)*', redirect: '/' }
  ]
})

router.beforeEach(async (to) => {
  const { usePortalAuthStore } = await import('@/stores/auth')
  const auth = usePortalAuthStore()

  // If already authenticated (e.g. just logged in), skip the network check
  if (auth.isAuthenticated) {
    if (to.name === 'portal-login') {
      return { name: 'portal-home' }
    }
    return // allow navigation
  }

  // Not authenticated yet — try to verify session with the server
  if (!auth.loading) {
    await auth.checkAuth()
  }

  if (to.meta.requiresAuth && !auth.isAuthenticated) {
    return { name: 'portal-login' }
  }

  if (to.name === 'portal-login' && auth.isAuthenticated) {
    return { name: 'portal-home' }
  }
})

export default router
