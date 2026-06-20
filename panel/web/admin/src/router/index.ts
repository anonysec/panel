import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

/** Routes that resellers are NOT allowed to access */
const adminOnlyRoutes = new Set([
  'overview',
  'nodes',
  'settings',
  'tickets',
  'ticket-detail',
  'payments',
  'templates',
  'notifications',
])

const router = createRouter({
  history: createWebHistory('/dashboard/'),
  routes: [
    {
      path: '/',
      component: () => import('@/layouts/AppShell.vue'),
      meta: { requiresAuth: true },
      children: [
        { path: '', name: 'overview', component: () => import('@/views/DashboardView.vue') },
        { path: 'users', name: 'users', component: () => import('@/views/CustomersView.vue') },
        { path: 'users/:id', name: 'user-detail', component: () => import('@/views/CustomerDetailView.vue'), props: true },
        { path: 'plans', name: 'plans', component: () => import('@/views/PlansView.vue') },
        { path: 'payments', name: 'payments', component: () => import('@/views/PaymentsView.vue') },
        { path: 'tickets', name: 'tickets', component: () => import('@/views/TicketsView.vue') },
        { path: 'tickets/:id', name: 'ticket-detail', component: () => import('@/views/TicketDetailView.vue'), props: true },
        { path: 'nodes', name: 'nodes', component: () => import('@/views/NodesView.vue') },
        { path: 'templates', name: 'templates', component: () => import('@/views/TemplatesView.vue') },
        { path: 'settings/:tab?', name: 'settings', component: () => import('@/views/SettingsView.vue'), props: true },
        { path: 'backups', redirect: '/dashboard/settings/backup' },
        { path: 'wireguard', redirect: '/dashboard/nodes' },
        { path: 'notifications', name: 'notifications', component: () => import('@/views/NotificationsView.vue') },
        // Redirects from old paths
        { path: 'customers', redirect: '/dashboard/users' },
        { path: 'customers/:id', redirect: (to: any) => `/dashboard/users/${to.params.id}` },
        { path: 'resellers', redirect: '/dashboard/users' },
      ]
    },
    { path: '/login', name: 'login', component: () => import('@/views/LoginView.vue') },
    { path: '/setup', name: 'setup', component: () => import('@/views/SetupView.vue') },
    { path: '/:pathMatch(.*)*', redirect: '/' }
  ]
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()

  if (!auth.initialized) {
    await auth.checkAuth()
  }

  if (auth.setupRequired && to.name !== 'setup') {
    return { name: 'setup' }
  }

  if (to.meta.requiresAuth && !auth.isAuthenticated) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }

  if ((to.name === 'login' || to.name === 'setup') && auth.isAuthenticated) {
    return { name: 'overview' }
  }

  // Role-based access: resellers can only access allowed routes
  if (auth.user?.role === 'reseller' && to.name && adminOnlyRoutes.has(to.name as string)) {
    return { name: 'users' }
  }

  // Legacy meta-based role check
  if (to.meta.roles && auth.user) {
    const roles = to.meta.roles as string[]
    if (!roles.includes(auth.user.role)) {
      return { name: 'overview' }
    }
  }
})

export default router
