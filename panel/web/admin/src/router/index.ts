import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

/** Routes that resellers are NOT allowed to access */
const adminOnlyRoutes = new Set([
  'overview',
  'nodes',
  'node-detail',
  'node-compare',
  'cores',
  'metrics',
  'landing-editor',
  'settings',
  'tickets',
  'ticket-detail',
  'payments',
  'billing',
  'templates',
  'notifications',
  'plans',
  'canned-responses',
  'sla-config',
  'knowledge-base',
  'user-tags',
  'filter-presets',
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
        { path: 'billing', name: 'billing', component: () => import('@/views/BillingView.vue') },
        { path: 'tickets', name: 'tickets', component: () => import('@/views/TicketsView.vue') },
        { path: 'tickets/:id', name: 'ticket-detail', component: () => import('@/views/TicketDetailView.vue'), props: true },
        { path: 'nodes', name: 'nodes', component: () => import('@/views/NodesView.vue') },
        { path: 'nodes/compare', name: 'node-compare', component: () => import('@/views/NodeCompareView.vue') },
        { path: 'nodes/:id/:tab?', name: 'node-detail', component: () => import('@/views/NodeDetailView.vue'), props: true },
        { path: 'cores', name: 'cores', component: () => import('@/views/CoresView.vue') },
        { path: 'metrics', name: 'metrics', component: () => import('@/views/MetricsDashboardView.vue') },
        { path: 'landing-editor', name: 'landing-editor', component: () => import('@/views/LandingPageEditorView.vue') },
        { path: 'templates', name: 'templates', component: () => import('@/views/TemplatesView.vue') },
        { path: 'settings/:tab?', name: 'settings', component: () => import('@/views/SettingsView.vue'), props: true },
        { path: 'backups', redirect: '/dashboard/settings/backup' },
        { path: 'wireguard', redirect: '/dashboard/nodes' },
        { path: 'notifications', name: 'notifications', component: () => import('@/views/NotificationsView.vue') },
        { path: 'telegram-proxies', redirect: '/dashboard/nodes' },
        { path: 'xray', redirect: '/dashboard/nodes' },
        { path: 'mtproto', redirect: '/dashboard/nodes' },
        { path: 'canned-responses', name: 'canned-responses', component: () => import('@/views/CannedResponsesView.vue') },
        { path: 'sla-config', name: 'sla-config', component: () => import('@/views/SLAConfigView.vue') },
        { path: 'knowledge-base', name: 'knowledge-base', component: () => import('@/views/KnowledgeBaseView.vue') },
        { path: 'user-tags', name: 'user-tags', component: () => import('@/views/UserTagsView.vue') },
        { path: 'filter-presets', name: 'filter-presets', component: () => import('@/views/FilterPresetsView.vue') },
        // Redirects from old paths
        { path: 'customers', redirect: '/dashboard/users' },
        { path: 'customers/:id', redirect: (to: any) => `/dashboard/users/${to.params.id}` },
        { path: 'resellers', redirect: '/dashboard/users' },
        // Reseller-specific routes
        { path: 'reseller-dashboard', name: 'reseller-dashboard', component: () => import('@/views/ResellerDashboardView.vue') },
        { path: 'reseller-plans', name: 'reseller-plans', component: () => import('@/views/ResellerPlansView.vue') },
        { path: 'reseller-transactions', name: 'reseller-transactions', component: () => import('@/views/ResellerTransactionsView.vue') },
        { path: 'reseller-tickets', name: 'reseller-tickets', component: () => import('@/views/ResellerTicketsView.vue') },
        { path: 'reseller-tickets/:id', name: 'reseller-ticket-detail', component: () => import('@/views/ResellerTicketDetailView.vue'), props: true },
        { path: 'reseller-settings', name: 'reseller-settings', component: () => import('@/views/ResellerSettingsView.vue') },
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
    return { name: 'reseller-dashboard' }
  }

  // Reseller landing page: redirect root to reseller-dashboard
  if (auth.user?.role === 'reseller' && (to.name === 'overview' || to.path === '/' || to.path === '')) {
    return { name: 'reseller-dashboard' }
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
