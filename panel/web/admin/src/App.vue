<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'

type Screen = 'loading' | 'setup' | 'login' | 'app'
type Section = 'overview' | 'customers' | 'customer-detail' | 'plans' | 'payments' | 'tickets' | 'resellers' | 'nodes' | 'system'

interface SetupStatus { ok: boolean; needs_setup: boolean; setup_key_required: boolean }
interface AuthResponse { ok: boolean; authenticated?: boolean; username?: string; role?: string; credit?: number }
interface ApiError extends Error { status?: number }

interface Customer { id: number; username: string; display_name: string; status: string; plan_id?: number | null; plan: string; credit: number; created_at: string }
interface DeletedCustomer extends Customer { deleted_at: string }
interface RadiusCheck { id: number; username: string; attribute: string; op: string; value: string }
interface WalletTransaction { id: number; username: string; amount: number; type: string; description: string; actor: string; created_at: string }
interface SubscriptionHistory { id: number; username: string; plan: string; status: string; started_at: string; expires_at: string; paid_amount: number; discount_code: string }
interface CustomerDetail extends Customer { notes: string; sub_token: string; radius_checks: RadiusCheck[]; radius_replies: RadiusCheck[]; subscription?: Record<string, unknown>; subscriptions: SubscriptionHistory[]; wallet_transactions: WalletTransaction[] }
interface Plan { id: number; name: string; data_gb: number; speed_mbps: number; duration_days: number; price: number; is_active: boolean; sort_order: number; created_at: string }
interface Payment { id: number; username: string; amount: number; method: string; status: string; intent_type: string; intent_id?: number; intent_label: string; created_at: string; updated_at: string }
interface PaymentMethod { id: number; name: string; type: string; instructions: string; is_active: boolean; sort_order: number; created_at: string }
interface Ticket { id: number; customer_id?: number; username: string; subject: string; status: string; priority: string; created_at: string; updated_at: string; closed_at: string }
interface TicketMessage { id: number; ticket_id: number; sender_type: string; sender_name: string; message: string; created_at: string }
interface TicketDetail extends Ticket { messages: TicketMessage[] }
interface NodeStatus { cpu_percent: number; ram_percent: number; disk_percent: number; rx_bps: number; tx_bps: number; openvpn_status: string; l2tp_status: string; ikev2_status: string; updated_at: string }
interface NodeService { service: string; status: string; updated_at: string }
interface NodeItem { id: number; name: string; public_ip: string; domain: string; status: string; last_seen_at: string; created_at: string; status_metrics: NodeStatus; services: NodeService[]; history?: any[] }
interface NodeTask { id: number; node_id: number; node_name: string; action: string; payload_json: Record<string, unknown>; status: string; error: string; created_at: string; completed_at: string }
interface VPNSettings { id: number; openvpn_port: number; openvpn_protocol: string; openvpn_network: string; l2tp_network: string; ikev2_network: string; ipsec_psk: string; dns_1: string; dns_2: string; updated_at: string; openvpn_service_status: string; ca_file: string; ca_exists: boolean; tls_crypt_file: string; tls_crypt_exists: boolean; remote_host: string; active_node: string }
interface UsageSession { id: number; username: string; start_time: string; update_time: string; stop_time: string; session_seconds: number; input_bytes: number; output_bytes: number; total_bytes: number; framed_ip: string; calling_station_id: string; terminate_cause: string; online: boolean }
interface UsageSummary { online: boolean; active_sessions: number; total_input_bytes: number; total_output_bytes: number; total_usage_bytes: number; max_data_bytes: number; remaining_bytes?: number; last_connected_at: string; last_disconnected_at: string; sessions: UsageSession[] }
interface Stats { ok: boolean; customers: number; active_customers: number; plans: number; nodes: number; open_tickets: number; pending_payments: number; approved_payments: number; total_rx_bps?: number; total_tx_bps?: number }
interface AuditLog { id: number; actor: string; action: string; entity_type: string; entity_id: string; before_json: string; after_json: string; ip: string; created_at: string }
type BlankNumber = number | ''

const screen = ref<Screen>('loading')
const section = ref<Section>('overview')
const setupStatus = ref<SetupStatus>({ ok: true, needs_setup: false, setup_key_required: false })
const user = ref({ username: '', role: '', credit: 0 })
const health = ref<{ ok?: boolean; version?: string; time?: string } | null>(null)
const stats = ref<Stats>({ ok: true, customers: 0, active_customers: 0, plans: 0, nodes: 0, open_tickets: 0, pending_payments: 0, approved_payments: 0 })
const customers = ref<Customer[]>([])
const deletedCustomers = ref<DeletedCustomer[]>([])
const plans = ref<Plan[]>([])
const payments = ref<Payment[]>([])
const paymentMethods = ref<PaymentMethod[]>([])
const methodForm = ref({ name: '', type: 'manual', instructions: '', is_active: true, sort_order: 0 })
const editingMethodId = ref<number | null>(null)
const tickets = ref<Ticket[]>([])
const selectedTicket = ref<TicketDetail | null>(null)
const ticketReply = ref('')
const adminTicketForm = ref({ username: '', subject: '', priority: 'normal', message: '' })
const nodes = ref<NodeItem[]>([])
const nodeTasks = ref<NodeTask[]>([])
const vpnSettings = ref<VPNSettings | null>(null)
const selectedCustomer = ref<CustomerDetail | null>(null)
const detailTab = ref<'profile' | 'usage' | 'history'>('profile')
const systemTab = ref<'audit' | 'backups' | 'diagnostics'>('diagnostics')
const infraTab = ref<'nodes' | 'vpn'>('nodes')
const customerView = ref<'active' | 'archived'>('active')
const selectedUsage = ref<UsageSummary | null>(null)
const search = ref('')
const busy = ref(false)
const appLoading = ref(false)
const detailLoading = ref(false)
const error = ref('')
const notice = ref('')
const auditLogs = ref<any[]>([])
const auditLoading = ref(false)
const auditOffset = ref(0)
const auditLimit = ref(100)

const setupForm = ref({ setup_key: '', username: 'owner', password: '' })
const loginForm = ref({ username: '', password: '' })
const createForm = ref<{ username: string; password: string; display_name: string; plan_id: number; data_gb: BlankNumber; speed_mbps: BlankNumber; days: BlankNumber }>({ username: '', password: '', display_name: '', plan_id: 0, data_gb: '', speed_mbps: '', days: '' })
const detailForm = ref({ display_name: '', status: 'active', plan_id: 0, notes: '', data_gb: 0, speed_mbps: 0, days: 0 })
const passwordForm = ref({ password: '' })
const planForm = ref({ name: '', data_gb: 0, speed_mbps: 0, duration_days: 30, price: 0, is_active: true, sort_order: 0 })
const paymentForm = ref({ username: '', amount: 0, method: 'manual', description: '' })
const nodeForm = ref({ name: '', public_ip: '', domain: '' })
const vpnForm = ref({ openvpn_port: 1194, openvpn_protocol: 'udp', openvpn_network: '10.8.0.0/24', l2tp_network: '10.9.0.0/24', ikev2_network: '10.10.0.0/24', ipsec_psk: '', dns_1: '1.1.1.1', dns_2: '8.8.8.8' })
const nodeToken = ref('')
const walletForm = ref({ username: '', amount: 0, description: 'Manual adjustment' })
const walletSetForm = ref({ username: '', balance: 0, description: 'Manual balance set' })
const renewForm = ref({ plan_id: 0 })
const editingPlanId = ref<number | null>(null)
const planModalOpen = ref(false)
const nodeModalOpen = ref(false)
const customerModalOpen = ref(false)
const realtimeConnected = ref(false)
const liveSessions = ref<any[]>([])
let realtimeSocket: WebSocket | null = null
let realtimeRetry: ReturnType<typeof setTimeout> | null = null

const activePlans = computed(() => plans.value.filter((plan) => plan.is_active))
const payAsGoPlan = computed(() => activePlans.value.find((plan) => plan.name.toLowerCase() === 'pay as you go'))
const selectedRenewPlan = computed(() => plans.value.find((plan) => plan.id === Number(renewForm.value.plan_id)))
const panelOrigin = computed(() => window.location.origin)
const nodeInstallCommand = computed(() => `cd koris-next && sudo PANEL_URL=${shQuote(panelOrigin.value)} NODE_TOKEN=${shQuote(nodeToken.value)} NODE_NAME=${shQuote(nodeForm.value.name || 'node1')} bash scripts/install-node.sh`)
const activePercent = computed(() => stats.value.customers ? Math.round((stats.value.active_customers / stats.value.customers) * 100) : 0)
const filteredCustomers = computed(() => {
  const q = search.value.trim().toLowerCase()
  const list = customerView.value === 'active' ? customers.value : deletedCustomers.value
  if (!q) return list
  return list.filter((customer) => `${customer.username} ${customer.display_name} ${customer.status} ${customer.plan}`.toLowerCase().includes(q))
})
const initials = computed(() => (user.value.username || 'K').slice(0, 2).toUpperCase())
const systemScore = computed(() => Math.min(100, (health.value?.ok ? 62 : 24) + (stats.value.customers ? 16 : 0) + (stats.value.plans ? 10 : 0) + (stats.value.nodes ? 12 : 0)))
const statusSummary = computed(() => {
  const summary: Record<string, number> = { active: 0, disabled: 0, expired: 0, limited: 0 }
  for (const customer of customers.value) summary[customer.status] = (summary[customer.status] || 0) + 1
  return summary
})

async function api<T>(url: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers || {})
  if (options.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  const response = await fetch(url, { credentials: 'same-origin', ...options, headers })
  const data = await response.json().catch(() => ({ ok: false, error: response.statusText }))
  if (!response.ok || data.ok === false) {
    const err = new Error(data.error || `HTTP ${response.status}`) as ApiError
    err.status = response.status
    throw err
  }
  return data as T
}

function connectRealtime() {
  if (realtimeSocket || screen.value !== 'app') return
  const scheme = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  realtimeSocket = new WebSocket(`${scheme}//${window.location.host}/api/realtime`)
  realtimeSocket.onopen = () => { realtimeConnected.value = true }
  realtimeSocket.onmessage = (event) => {
    try {
      const message = JSON.parse(event.data)
      if (message.type === 'stats' && message.data) stats.value = message.data as Stats
      if (message.type === 'sessions' && message.data) liveSessions.value = message.data
    } catch { /* ignore malformed realtime frame */ }
  }
  realtimeSocket.onclose = () => {
    realtimeSocket = null
    realtimeConnected.value = false
    if (screen.value === 'app') realtimeRetry = setTimeout(connectRealtime, 3000)
  }
  realtimeSocket.onerror = () => realtimeSocket?.close()
}

function disconnectRealtime() {
  if (realtimeRetry) clearTimeout(realtimeRetry)
  realtimeRetry = null
  realtimeConnected.value = false
  if (realtimeSocket) {
    realtimeSocket.onclose = null
    realtimeSocket.close()
  }
  realtimeSocket = null
}

async function boot() {
  error.value = ''
  try {
    setupStatus.value = await api<SetupStatus>('/api/setup/status')
    if (setupStatus.value.needs_setup) { screen.value = 'setup'; return }
    const me = await api<AuthResponse>('/api/auth/me')
    if (me.authenticated) {
      user.value = { username: me.username || 'admin', role: me.role || 'admin', credit: me.credit || 0 }
      screen.value = 'app'
      await loadDashboard()
      return
    }
    screen.value = 'login'
  } catch (err) { error.value = friendlyError(err); screen.value = 'login' }
}

async function submitSetup() {
  busy.value = true; error.value = ''
  try {
    const res = await api<AuthResponse>('/api/setup/owner', { method: 'POST', body: JSON.stringify(setupForm.value) })
    user.value = { username: res.username || setupForm.value.username, role: res.role || 'owner', credit: 0 }
    notice.value = 'Owner account created. Welcome to KorisPanel.'
    screen.value = 'app'
    await loadDashboard()
  } catch (err) { error.value = friendlyError(err) } finally { busy.value = false }
}

async function submitLogin() {
  busy.value = true; error.value = ''
  try {
    const res = await api<AuthResponse>('/api/auth/admin', { method: 'POST', body: JSON.stringify(loginForm.value) })
    user.value = { username: res.username || loginForm.value.username, role: res.role || 'admin', credit: res.credit || 0 }
    screen.value = 'app'
    await loadDashboard()
  } catch (err) { error.value = friendlyError(err) } finally { busy.value = false }
}

async function logout() {
  disconnectRealtime()
  await api<{ ok: boolean }>('/api/auth/logout', { method: 'POST' }).catch(() => null)
  user.value = { username: '', role: '', credit: 0 }
  screen.value = 'login'
}

async function loadDashboard() {
  appLoading.value = true; error.value = ''
  try {
    const [healthRes, statsRes, customersRes, deletedRes, plansRes, paymentsRes, paymentMethodsRes, ticketsRes, nodesRes, nodeTasksRes, vpnRes] = await Promise.all([
      api<{ ok: boolean; version: string; time: string }>('/api/health'),
      api<Stats>('/api/dashboard/stats'),
      api<{ ok: boolean; customers: Customer[] }>(`/api/customers?q=${encodeURIComponent(search.value.trim())}`),
      api<{ ok: boolean; customers: DeletedCustomer[] }>('/api/deleted/customers'),
      api<{ ok: boolean; plans: Plan[] }>('/api/plans'),
      api<{ ok: boolean; payments: Payment[] }>('/api/payments'),
      api<{ ok: boolean; methods: PaymentMethod[] }>('/api/payment-methods'),
      api<{ ok: boolean; tickets: Ticket[] }>('/api/tickets'),
      api<{ ok: boolean; nodes: NodeItem[] }>('/api/nodes'),
      api<{ ok: boolean; tasks: NodeTask[] }>('/api/node/tasks'),
      api<{ ok: boolean; settings: VPNSettings }>('/api/vpn/settings')
    ])
    health.value = healthRes; stats.value = statsRes; customers.value = customersRes.customers || []; deletedCustomers.value = deletedRes.customers || []; plans.value = plansRes.plans || []; payments.value = paymentsRes.payments || []; paymentMethods.value = paymentMethodsRes.methods || []; tickets.value = ticketsRes.tickets || []; nodes.value = nodesRes.nodes || []; nodeTasks.value = nodeTasksRes.tasks || []; vpnSettings.value = vpnRes.settings; vpnForm.value = { openvpn_port: vpnRes.settings.openvpn_port, openvpn_protocol: vpnRes.settings.openvpn_protocol, openvpn_network: vpnRes.settings.openvpn_network, l2tp_network: vpnRes.settings.l2tp_network, ikev2_network: vpnRes.settings.ikev2_network, ipsec_psk: vpnRes.settings.ipsec_psk || '', dns_1: vpnRes.settings.dns_1, dns_2: vpnRes.settings.dns_2 }
    defaultCreatePlanIfNeeded()
    connectRealtime()
    if (user.value.role === 'reseller') {
      await loadResellerPayments()
    }
  } catch (err) {
    const apiErr = err as ApiError
    if (apiErr.status === 401) screen.value = 'login'
    error.value = friendlyError(err)
  } finally { appLoading.value = false }
}

async function loadAuditLogs() {
  auditLoading.value = true; error.value = ''
  try {
    const res = await api<{ ok: boolean; logs: AuditLog[]; limit: number; offset: number }>(`/api/audit-logs?limit=${auditLimit.value}&offset=${auditOffset.value}`)
    auditLogs.value = res.logs || []
  } catch (err) { error.value = friendlyError(err) }
  finally { auditLoading.value = false }
}

const diagnosticsData = ref<any>(null)
const diagnosticsLoading = ref(false)
async function loadDiagnostics() {
  diagnosticsLoading.value = true; error.value = ''
  try {
    const res = await api<any>('/api/diagnostics')
    diagnosticsData.value = res
  } catch (err) { error.value = friendlyError(err) }
  finally { diagnosticsLoading.value = false }
}

const resellersList = ref<any[]>([])
const resellerForm = ref({ username: '', password: '' })
const resellerCreditForm = ref<Record<number, number>>({})
const resellerTxs = ref<any[]>([])

async function loadResellerTxs() {
  try {
    const res = await api<any>('/api/resellers/transactions')
    resellerTxs.value = res.transactions || []
  } catch (err) { error.value = friendlyError(err) }
}

async function loadResellers() {
  error.value = ''
  try {
    const res = await api<any>('/api/resellers')
    resellersList.value = res.resellers || []
    await loadResellerTxs()
  } catch (err) { error.value = friendlyError(err) }
}

async function createReseller() {
  busy.value = true; error.value = ''
  try {
    await api<any>('/api/resellers', {
      method: 'POST',
      body: JSON.stringify(resellerForm.value)
    })
    resellerForm.value = { username: '', password: '' }
    notice.value = 'Reseller created successfully.'
    await loadResellers()
  } catch (err) { error.value = friendlyError(err) }
  finally { busy.value = false }
}

async function adjustResellerCredit(id: number, add: boolean) {
  busy.value = true; error.value = ''
  let amt = resellerCreditForm.value[id] || 0
  if (!add) amt = -amt
  try {
    await api<any>(`/api/resellers/${id}/credit`, {
      method: 'POST',
      body: JSON.stringify({ amount: amt })
    })
    resellerCreditForm.value[id] = 0
    notice.value = 'Reseller credit adjusted successfully.'
    await loadResellers()
  } catch (err) { error.value = friendlyError(err) }
  finally { busy.value = false }
}

async function deleteReseller(id: number) {
  if (!confirm('Are you sure you want to delete this reseller?')) return
  busy.value = true; error.value = ''
  try {
    await api<any>(`/api/resellers/${id}`, { method: 'DELETE' })
    notice.value = 'Reseller deleted.'
    await loadResellers()
  } catch (err) { error.value = friendlyError(err) }
  finally { busy.value = false }
}

async function killSession(id: number) {
  if (!confirm('Are you sure you want to terminate this active VPN connection?')) return
  error.value = ''
  try {
    await api<any>('/api/sessions/kill', {
      method: 'POST',
      body: JSON.stringify({ id })
    })
    notice.value = 'VPN session terminated.'
    liveSessions.value = liveSessions.value.filter(s => s.id !== id)
  } catch (err) { error.value = friendlyError(err) }
}

const rxHistory = ref<number[]>(Array(20).fill(0))
const txHistory = ref<number[]>(Array(20).fill(0))
const resellerTopupAmount = ref(50000)

watch(() => stats.value, (newStats: any) => {
  if (newStats) {
    rxHistory.value.push(newStats.total_rx_bps || 0)
    rxHistory.value.shift()
    txHistory.value.push(newStats.total_tx_bps || 0)
    txHistory.value.shift()
  }
}, { deep: true })

const maxBps = computed(() => {
  const maxVal = Math.max(...rxHistory.value, ...txHistory.value, 1024)
  return maxVal
})

const rxPoints = computed(() => {
  const max = maxBps.value
  return rxHistory.value.map((val, idx) => `${idx * 18},${60 - (val / max) * 50}`).join(' ')
})

const txPoints = computed(() => {
  const max = maxBps.value
  return txHistory.value.map((val, idx) => `${idx * 18},${60 - (val / max) * 50}`).join(' ')
})

async function checkoutResellerCredit() {
  busy.value = true; error.value = ''
  try {
    await api<any>('/api/resellers/checkout', {
      method: 'POST',
      body: JSON.stringify({ amount: resellerTopupAmount.value })
    })
    notice.value = 'Self-checkout completed. Reseller wallet credited.'
    user.value.credit += resellerTopupAmount.value
    resellerTopupAmount.value = 50000
    await loadDashboard()
  } catch (err) { error.value = friendlyError(err) }
  finally { busy.value = false }
}

function nodeHistoryPoints(history: any[]) {
  if (!history || !history.length) return '0,40 150,40'
  const maxRx = Math.max(...history.map(h => Number(h.rx_bytes || 0)), 1024)
  const reversed = [...history].reverse()
  return reversed.map((h, idx) => {
    const x = (idx / (reversed.length - 1 || 1)) * 150
    const y = 35 - (Number(h.rx_bytes || 0) / maxRx) * 30
    return `${x},${y}`
  }).join(' ')
}

function copyToClipboard(text: string) {
  if (navigator.clipboard && window.isSecureContext) {
    navigator.clipboard.writeText(text).then(() => {
      notice.value = 'Copied to clipboard!'
    })
  } else {
    const textArea = document.createElement('textarea')
    textArea.value = text
    textArea.style.top = '0'
    textArea.style.left = '0'
    textArea.style.position = 'fixed'
    document.body.appendChild(textArea)
    textArea.focus()
    textArea.select()
    try {
      const successful = document.execCommand('copy')
      if (successful) {
        notice.value = 'Copied to clipboard!'
      } else {
        notice.value = 'Press Ctrl+C to copy'
      }
    } catch (err) {
      notice.value = 'Failed to copy, please copy manually'
    }
    document.body.removeChild(textArea)
  }
}

const resellerPayments = ref<any[]>([])
const resellerManualPayForm = ref({ amount: 100000, description: '' })

async function loadResellerPayments() {
  try {
    const res = await api<any>('/api/resellers/payments')
    resellerPayments.value = res.payments || []
  } catch (err) { error.value = friendlyError(err) }
}

async function submitManualResellerPayment() {
  busy.value = true; error.value = ''
  try {
    await api<any>('/api/resellers/payments', {
      method: 'POST',
      body: JSON.stringify(resellerManualPayForm.value)
    })
    resellerManualPayForm.value = { amount: 100000, description: '' }
    notice.value = 'Manual top-up request submitted for admin review.'
    await loadResellerPayments()
  } catch (err) { error.value = friendlyError(err) }
  finally { busy.value = false }
}

function exportCSV(name: string) {
  window.open(`/api/export/${name}.csv`, '_blank')
}

function defaultCreatePlanIfNeeded() {
  if (!createForm.value.plan_id && payAsGoPlan.value) {
    createForm.value.plan_id = payAsGoPlan.value.id
    applyCreatePlan()
  }
}
function applyCreatePlan() {
  const plan = plans.value.find((item) => item.id === Number(createForm.value.plan_id))
  if (!plan) {
    createForm.value.data_gb = ''
    createForm.value.speed_mbps = ''
    createForm.value.days = ''
    return
  }
  createForm.value.data_gb = plan.data_gb || ''
  createForm.value.speed_mbps = plan.speed_mbps || ''
  createForm.value.days = plan.duration_days || ''
}
function applyDetailPlan() {
  const plan = plans.value.find((item) => item.id === Number(detailForm.value.plan_id))
  if (!plan) return
  detailForm.value.data_gb = plan.data_gb
  detailForm.value.speed_mbps = plan.speed_mbps
  detailForm.value.days = plan.duration_days
}
function resetCreateForm() {
  createForm.value = { username: '', password: '', display_name: '', plan_id: 0, data_gb: '', speed_mbps: '', days: '' }
  defaultCreatePlanIfNeeded()
}
function cleanNumber(value: unknown) { const n = Number(value); return Number.isFinite(n) && n > 0 ? n : 0 }

async function createCustomer() {
  busy.value = true; error.value = ''; notice.value = ''
  try {
    const payload = { ...createForm.value, data_gb: cleanNumber(createForm.value.data_gb), speed_mbps: cleanNumber(createForm.value.speed_mbps), days: Math.trunc(cleanNumber(createForm.value.days)) }
    await api<{ ok: boolean; id: number }>('/api/customers', { method: 'POST', body: JSON.stringify(payload) })
    notice.value = `Customer ${createForm.value.username} created.`
    customerModalOpen.value = false
    resetCreateForm()
    await loadDashboard()
  } catch (err) { error.value = friendlyError(err) } finally { busy.value = false }
}

async function openCustomer(customer: Customer) { section.value = 'customer-detail'; await loadCustomer(customer.id) }
async function loadCustomer(id: number) {
  detailLoading.value = true; error.value = ''; selectedCustomer.value = null; selectedUsage.value = null
  try {
    const [res, usageRes] = await Promise.all([
      api<{ ok: boolean; customer: CustomerDetail }>(`/api/customers/${id}`),
      api<{ ok: boolean; usage: UsageSummary }>(`/api/customers/${id}/usage`)
    ])
    selectedCustomer.value = res.customer
    selectedUsage.value = usageRes.usage
    detailForm.value = { display_name: res.customer.display_name || '', status: res.customer.status || 'active', plan_id: Number(res.customer.plan_id || 0), notes: res.customer.notes || '', data_gb: maxDataGB(res.customer.radius_checks || []), speed_mbps: speedMbps(res.customer.radius_replies || []), days: 0 }
    walletForm.value.username = res.customer.username
    walletSetForm.value.username = res.customer.username
    walletSetForm.value.balance = Number(res.customer.credit || 0)
    renewForm.value.plan_id = Number(res.customer.plan_id || payAsGoPlan.value?.id || 0)
    paymentForm.value.username = res.customer.username
  } catch (err) { error.value = friendlyError(err) } finally { detailLoading.value = false }
}

async function saveCustomerDetail() {
  if (!selectedCustomer.value) return
  busy.value = true; error.value = ''; notice.value = ''
  try {
    const payload = { ...detailForm.value, data_gb: cleanNumber(detailForm.value.data_gb), speed_mbps: cleanNumber(detailForm.value.speed_mbps), days: Math.trunc(cleanNumber(detailForm.value.days)) }
    await api<{ ok: boolean }>(`/api/customers/${selectedCustomer.value.id}`, { method: 'PATCH', body: JSON.stringify(payload) })
    notice.value = 'Customer details saved.'
    await loadCustomer(selectedCustomer.value.id); await loadDashboard()
  } catch (err) { error.value = friendlyError(err) } finally { busy.value = false }
}

async function setSelectedCustomerStatus(status: 'active' | 'disabled') {
  if (!selectedCustomer.value) return
  busy.value = true; error.value = ''
  try {
    await api<{ ok: boolean }>(`/api/customers/${selectedCustomer.value.id}/${status === 'active' ? 'enable' : 'disable'}`, { method: 'POST' })
    notice.value = status === 'active' ? 'Customer enabled.' : 'Customer disabled.'
    await loadCustomer(selectedCustomer.value.id); await loadDashboard()
  } catch (err) { error.value = friendlyError(err) } finally { busy.value = false }
}

async function resetCustomerPassword() {
  if (!selectedCustomer.value) return
  busy.value = true; error.value = ''; notice.value = ''
  try {
    await api<{ ok: boolean }>(`/api/customers/${selectedCustomer.value.id}/reset-password`, { method: 'POST', body: JSON.stringify(passwordForm.value) })
    notice.value = 'VPN password reset.'; passwordForm.value.password = ''; await loadCustomer(selectedCustomer.value.id)
  } catch (err) { error.value = friendlyError(err) } finally { busy.value = false }
}

async function renewCustomerPlan() {
  if (!selectedCustomer.value) return
  if (!renewForm.value.plan_id) { error.value = 'plan required'; return }
  busy.value = true; error.value = ''; notice.value = ''
  try {
    const res = await api<{ ok: boolean; wallet_deducted: number }>(`/api/customers/${selectedCustomer.value.id}/renew`, { method: 'POST', body: JSON.stringify(renewForm.value) })
    notice.value = res.wallet_deducted > 0 ? `Plan applied. Wallet deducted ${formatMoney(res.wallet_deducted)}.` : 'Plan applied.'
    await loadCustomer(selectedCustomer.value.id); await loadDashboard()
  } catch (err) { error.value = friendlyError(err) } finally { busy.value = false }
}

async function archiveSelectedCustomer() {
  if (!selectedCustomer.value) return
  if (!confirm(`Archive user ${selectedCustomer.value.username}? VPN access will be revoked until restored.`)) return
  busy.value = true; error.value = ''; notice.value = ''
  try {
    await api<{ ok: boolean }>(`/api/customers/${selectedCustomer.value.id}`, { method: 'DELETE' })
    notice.value = 'Customer archived.'
    selectedCustomer.value = null
    customerView.value = 'archived'
    section.value = 'customers'
    await loadDashboard()
  } catch (err) { error.value = friendlyError(err) } finally { busy.value = false }
}

async function restoreDeletedCustomer(customer: DeletedCustomer) {
  busy.value = true; error.value = ''; notice.value = ''
  try {
    await api<{ ok: boolean }>(`/api/customers/${customer.id}/restore`, { method: 'POST' })
    notice.value = `Customer ${customer.username} restored.`
    await loadDashboard()
  } catch (err) { error.value = friendlyError(err) } finally { busy.value = false }
}

function resetPlanForm() { editingPlanId.value = null; planForm.value = { name: '', data_gb: 0, speed_mbps: 0, duration_days: 30, price: 0, is_active: true, sort_order: 0 } }
function openNewPlan() { resetPlanForm(); planModalOpen.value = true }
function editPlan(plan: Plan) { editingPlanId.value = plan.id; planForm.value = { name: plan.name, data_gb: plan.data_gb, speed_mbps: plan.speed_mbps, duration_days: plan.duration_days, price: plan.price, is_active: plan.is_active, sort_order: plan.sort_order }; planModalOpen.value = true }
async function savePlan() {
  busy.value = true; error.value = ''; notice.value = ''
  try {
    const payload = { ...planForm.value, data_gb: cleanNumber(planForm.value.data_gb), speed_mbps: cleanNumber(planForm.value.speed_mbps), duration_days: Math.trunc(cleanNumber(planForm.value.duration_days)), price: cleanNumber(planForm.value.price) }
    if (editingPlanId.value) { await api<{ ok: boolean }>(`/api/plans/${editingPlanId.value}`, { method: 'PATCH', body: JSON.stringify(payload) }); notice.value = 'Plan updated.' }
    else { await api<{ ok: boolean; id: number }>('/api/plans', { method: 'POST', body: JSON.stringify(payload) }); notice.value = 'Plan created.' }
    resetPlanForm(); planModalOpen.value = false; await loadDashboard()
  } catch (err) { error.value = friendlyError(err) } finally { busy.value = false }
}
async function archivePlan(plan: Plan) {
  if (!confirm(`Deactivate plan ${plan.name}? Existing customers keep their reference.`)) return
  busy.value = true; error.value = ''
  try { await api<{ ok: boolean }>(`/api/plans/${plan.id}`, { method: 'DELETE' }); notice.value = 'Plan deactivated.'; await loadDashboard() }
  catch (err) { error.value = friendlyError(err) } finally { busy.value = false }
}


async function saveVPNSettings(apply = false) {
  busy.value = true; error.value = ''; notice.value = ''
  try {
    const res = await api<{ ok: boolean; settings: VPNSettings; applied: boolean; apply_error: string }>('/api/vpn/settings', { method: 'PATCH', body: JSON.stringify({ ...vpnForm.value, apply }) })
    vpnSettings.value = res.settings
    if (apply && res.apply_error) notice.value = `Settings saved, but apply failed: ${res.apply_error}`
    else notice.value = apply ? 'VPN settings saved and OpenVPN restarted.' : 'VPN settings saved.'
    await loadDashboard()
  } catch (err) { error.value = friendlyError(err) } finally { busy.value = false }
}

function resetNodeForm() { nodeForm.value = { name: '', public_ip: '', domain: '' }; nodeToken.value = '' }
async function createNode() {
  busy.value = true; error.value = ''; notice.value = ''; nodeToken.value = ''
  try {
    const res = await api<{ ok: boolean; id: number; token: string }>('/api/nodes', { method: 'POST', body: JSON.stringify(nodeForm.value) })
    nodeToken.value = res.token
    notice.value = 'Node created. Copy the token now.'
    await loadDashboard()
  } catch (err) { error.value = friendlyError(err) } finally { busy.value = false }
}
async function rotateNodeToken(node: NodeItem) {
  if (!confirm(`Rotate token for ${node.name}? The old node token will stop working.`)) return
  busy.value = true; error.value = ''; notice.value = ''; nodeToken.value = ''
  try {
    const res = await api<{ ok: boolean; token: string }>(`/api/nodes/${node.id}/rotate-token`, { method: 'POST' })
    nodeToken.value = res.token
    nodeModalOpen.value = true
    notice.value = 'Node token rotated. Copy the new token now.'
    await loadDashboard()
  } catch (err) { error.value = friendlyError(err) } finally { busy.value = false }
}
async function setNodeEnabled(node: NodeItem, enabled: boolean) {
  busy.value = true; error.value = ''
  try {
    await api<{ ok: boolean }>(`/api/nodes/${node.id}/${enabled ? 'enable' : 'disable'}`, { method: 'POST' })
    notice.value = enabled ? 'Node enabled.' : 'Node disabled.'
    await loadDashboard()
  } catch (err) { error.value = friendlyError(err) } finally { busy.value = false }
}
function serviceLabel(node: NodeItem, key: string) {
  return node.services?.find((service) => service.service === key)?.status || node.status_metrics?.[`${key}_status` as keyof NodeStatus] || 'unknown'
}
function bps(value?: number) {
  const n = Number(value || 0)
  if (n > 1024 * 1024) return `${(n / 1024 / 1024).toFixed(2)} MB/s`
  if (n > 1024) return `${(n / 1024).toFixed(2)} KB/s`
  return `${Math.round(n)} B/s`
}


async function createNodeTask(node: NodeItem, action: string, payload: Record<string, unknown> = {}) {
  busy.value = true; error.value = ''; notice.value = ''
  try {
    await api<{ ok: boolean; id: number }>('/api/node/tasks', { method: 'POST', body: JSON.stringify({ node_id: node.id, action, payload_json: payload }) })
    notice.value = `Task queued for ${node.name}.`
    await loadDashboard()
  } catch (err) { error.value = friendlyError(err) } finally { busy.value = false }
}



function resetMethodForm() { editingMethodId.value = null; methodForm.value = { name: '', type: 'manual', instructions: '', is_active: true, sort_order: 0 } }
function editPaymentMethod(method: PaymentMethod) { editingMethodId.value = method.id; methodForm.value = { name: method.name, type: method.type, instructions: method.instructions || '', is_active: method.is_active, sort_order: method.sort_order } }
async function savePaymentMethod() {
  busy.value = true; error.value = ''; notice.value = ''
  try {
    if (editingMethodId.value) {
      await api<{ ok: boolean }>(`/api/payment-methods/${editingMethodId.value}`, { method: 'PATCH', body: JSON.stringify(methodForm.value) })
      notice.value = 'Payment method updated.'
    } else {
      await api<{ ok: boolean; id: number }>('/api/payment-methods', { method: 'POST', body: JSON.stringify(methodForm.value) })
      notice.value = 'Payment method created.'
    }
    resetMethodForm(); await loadDashboard()
  } catch (err) { error.value = friendlyError(err) } finally { busy.value = false }
}
async function deactivatePaymentMethod(method: PaymentMethod) {
  busy.value = true; error.value = ''; notice.value = ''
  try {
    await api<{ ok: boolean }>(`/api/payment-methods/${method.id}`, { method: 'DELETE' })
    notice.value = 'Payment method deactivated.'
    await loadDashboard()
  } catch (err) { error.value = friendlyError(err) } finally { busy.value = false }
}

async function loadTicket(id: number) {
  busy.value = true; error.value = ''
  try {
    const res = await api<{ ok: boolean; ticket: TicketDetail }>(`/api/tickets/${id}`)
    selectedTicket.value = res.ticket
    ticketReply.value = ''
  } catch (err) { error.value = friendlyError(err) } finally { busy.value = false }
}
async function replyTicket() {
  if (!selectedTicket.value || !ticketReply.value.trim()) return
  busy.value = true; error.value = ''; notice.value = ''
  try {
    await api<{ ok: boolean }>(`/api/tickets/${selectedTicket.value.id}/reply`, { method: 'POST', body: JSON.stringify({ message: ticketReply.value }) })
    notice.value = 'Reply sent.'
    await loadTicket(selectedTicket.value.id); await loadDashboard()
  } catch (err) { error.value = friendlyError(err) } finally { busy.value = false }
}
async function setTicketStatus(ticket: Ticket, status: 'open' | 'closed') {
  busy.value = true; error.value = ''; notice.value = ''
  try {
    await api<{ ok: boolean }>(`/api/tickets/${ticket.id}/${status === 'closed' ? 'close' : 'open'}`, { method: 'POST' })
    notice.value = status === 'closed' ? 'Ticket closed.' : 'Ticket reopened.'
    await loadTicket(ticket.id).catch(() => null); await loadDashboard()
  } catch (err) { error.value = friendlyError(err) } finally { busy.value = false }
}
async function createAdminTicket() {
  busy.value = true; error.value = ''; notice.value = ''
  try {
    const res = await api<{ ok: boolean; id: number }>('/api/tickets', { method: 'POST', body: JSON.stringify(adminTicketForm.value) })
    notice.value = 'Ticket created.'
    adminTicketForm.value = { username: '', subject: '', priority: 'normal', message: '' }
    await loadDashboard(); await loadTicket(res.id)
  } catch (err) { error.value = friendlyError(err) } finally { busy.value = false }
}

async function createManualPayment() {
  busy.value = true; error.value = ''; notice.value = ''
  try {
    await api<{ ok: boolean; id: number }>('/api/payments', { method: 'POST', body: JSON.stringify({ ...paymentForm.value, amount: cleanNumber(paymentForm.value.amount) }) })
    notice.value = 'Manual payment recorded and wallet topped up.'
    paymentForm.value = { username: '', amount: 0, method: 'manual', description: '' }
    await loadDashboard()
  } catch (err) { error.value = friendlyError(err) } finally { busy.value = false }
}
async function adjustWallet() {
  busy.value = true; error.value = ''; notice.value = ''
  try {
    await api<{ ok: boolean }>(`/api/wallets/${encodeURIComponent(walletForm.value.username)}/adjust`, { method: 'POST', body: JSON.stringify({ amount: Number(walletForm.value.amount), description: walletForm.value.description }) })
    notice.value = 'Wallet adjusted.'; walletForm.value.amount = 0; await loadDashboard(); if (selectedCustomer.value) await loadCustomer(selectedCustomer.value.id)
  } catch (err) { error.value = friendlyError(err) } finally { busy.value = false }
}
async function setWalletBalance() {
  busy.value = true; error.value = ''; notice.value = ''
  try {
    await api<{ ok: boolean }>(`/api/wallets/${encodeURIComponent(walletSetForm.value.username)}/set`, { method: 'POST', body: JSON.stringify({ balance: Number(walletSetForm.value.balance), description: walletSetForm.value.description }) })
    notice.value = 'Wallet balance saved.'; await loadDashboard(); if (selectedCustomer.value) await loadCustomer(selectedCustomer.value.id)
  } catch (err) { error.value = friendlyError(err) } finally { busy.value = false }
}
async function approvePayment(payment: Payment, status: 'approve' | 'reject') {
  busy.value = true; error.value = ''
  try { await api<{ ok: boolean }>(`/api/payments/${payment.id}/${status}`, { method: 'POST' }); notice.value = `Payment ${status}d.`; await loadDashboard() }
  catch (err) { error.value = friendlyError(err) } finally { busy.value = false }
}

function friendlyError(err: unknown) { return err instanceof Error ? err.message.replace(/_/g, ' ') : 'Unexpected error' }
function formatDate(value?: string) { return value ? new Intl.DateTimeFormat('en', { month: 'short', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(new Date(value)) : '—' }
function shQuote(value: string) { return `'${String(value).replace(/'/g, `'\\''`)}'` }
function formatMoney(value?: number) { return `${new Intl.NumberFormat('en', { maximumFractionDigits: 0 }).format(value || 0)} IRT` }
function signedMoney(value?: number) { const n = Number(value || 0); return `${n > 0 ? '+' : ''}${formatMoney(n)}` }
function formatBytes(value?: number) {
  const n = Number(value || 0)
  if (n >= 1024 ** 4) return `${(n / 1024 ** 4).toFixed(2)} TB`
  if (n >= 1024 ** 3) return `${(n / 1024 ** 3).toFixed(2)} GB`
  if (n >= 1024 ** 2) return `${(n / 1024 ** 2).toFixed(2)} MB`
  if (n >= 1024) return `${(n / 1024).toFixed(2)} KB`
  return `${Math.round(n)} B`
}
function formatDuration(seconds?: number) {
  const s = Math.max(0, Math.trunc(Number(seconds || 0)))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = s % 60
  if (h) return `${h}h ${m}m`
  if (m) return `${m}m ${sec}s`
  return `${sec}s`
}
function formatGB(value?: number) { return value && value > 0 ? `${new Intl.NumberFormat('en', { maximumFractionDigits: 2 }).format(value)} GB` : 'Unlimited' }
function formatSpeed(value?: number) { return value && value > 0 ? `${new Intl.NumberFormat('en', { maximumFractionDigits: 2 }).format(value)} Mbps` : 'Unlimited' }
function maxDataGB(checks: RadiusCheck[]) { const raw = Number(checks.find((check) => check.attribute === 'Max-Data')?.value || 0); return raw ? Math.round((raw / 1024 / 1024 / 1024) * 100) / 100 : 0 }
function speedMbps(replies: RadiusCheck[]) { const v = replies.find((reply) => reply.attribute === 'Mikrotik-Rate-Limit')?.value || ''; const m = v.match(/([0-9.]+)M/i); return m ? Number(m[1]) : 0 }
function subscriptionText(customer: CustomerDetail | null) { const sub = customer?.subscription; return sub ? `${sub.status || 'active'} · expires ${formatDate(String(sub.expires_at || ''))}` : 'No subscription yet' }

watch(notice, (message) => {
  if (!message) return
  window.setTimeout(() => {
    if (notice.value === message) notice.value = ''
  }, 4000)
})

watch(section, (newSec) => {
  window.location.hash = '/' + newSec
})

onMounted(() => {
  if (window.location.pathname !== '/dashboard/' && window.location.pathname !== '/dashboard') {
    window.history.replaceState(null, '', '/dashboard/' + window.location.hash)
  }
  const hash = window.location.hash.replace('#/', '').replace('#', '')
  if (hash && ['overview', 'customers', 'plans', 'payments', 'tickets', 'resellers', 'nodes', 'system', 'customer-detail'].includes(hash)) {
    section.value = hash as Section
  }
  boot()
})
</script>


<template>
  <!-- Loading -->
  <div v-if="screen==='loading'" class="loading-screen"><div class="spinner"></div></div>

  <!-- Auth -->
  <div v-else-if="screen==='setup'||screen==='login'" class="auth-screen">
    <div class="auth-left">
      <div class="brand"><div class="logo">K</div><div class="brand-text"><h1>KorisPanel</h1><span>Control Center</span></div></div>
      <h1>VPN Management<br>Made Simple</h1>
      <p>Multi-protocol, multi-node VPN panel with customer management, billing, and real-time monitoring.</p>
    </div>
    <div class="auth-right">
      <h2>{{ screen==='setup'?'Create Owner':'Sign In' }}</h2>
      <div class="sub">{{ screen==='setup'?'Initialize your panel':'Admin access required' }}</div>
      <form v-if="screen==='setup'" class="form-stack" @submit.prevent="submitSetup">
        <label v-if="setupStatus.setup_key_required">Setup Key<input v-model="setupForm.setup_key" required/></label>
        <label>Username<input v-model.trim="setupForm.username" required/></label>
        <label>Password<input v-model="setupForm.password" type="password" required/></label>
        <button class="btn-primary" :disabled="busy">{{ busy?'Creating...':'Create Owner' }}</button>
      </form>
      <form v-else class="form-stack" @submit.prevent="submitLogin">
        <label>Username<input v-model.trim="loginForm.username" required/></label>
        <label>Password<input v-model="loginForm.password" type="password" required/></label>
        <button class="btn-primary" :disabled="busy">{{ busy?'Signing in...':'Sign In' }}</button>
      </form>
      <p v-if="error" class="alert danger">{{ error }}</p>
    </div>
  </div>

  <!-- App Shell -->
  <template v-else>
    <aside class="sidebar">
      <div class="brand"><div class="logo">K</div><div class="brand-text"><h1>KorisPanel</h1><span>v{{ health?.version||'dev' }}</span></div></div>

      <div class="nav-group">Overview</div>
      <button class="nav-item" :class="{active:section==='overview'}" @click="section='overview'"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="7" height="9" rx="1"/><rect x="14" y="3" width="7" height="5" rx="1"/><rect x="14" y="12" width="7" height="9" rx="1"/><rect x="3" y="16" width="7" height="5" rx="1"/></svg>Dashboard</button>
      <button class="nav-item" :class="{active:section==='payments'}" @click="section='payments'"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 3v18h18"/><path d="M7 14l4-4 3 3 5-6"/></svg>Analytics</button>
      <button class="nav-item" :class="{active:section==='tickets'}" @click="section='tickets'"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 7h18M3 12h18M3 17h12"/></svg>Transactions<span v-if="stats.pending_payments" class="badge">{{ stats.pending_payments }}</span></button>

      <div class="nav-group">Manage</div>
      <button class="nav-item" :class="{active:section==='customers'||section==='customer-detail'||section==='resellers'}" @click="section='customers'"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="9" cy="8" r="3.5"/><path d="M2.5 20a6.5 6.5 0 0113 0"/><circle cx="17" cy="9" r="2.5"/><path d="M16 14.5A5 5 0 0121.5 20"/></svg>Users<span v-if="stats.customers" class="badge">{{ stats.customers }}</span></button>
      <button class="nav-item" :class="{active:section==='nodes'}" @click="section='nodes'"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="4" width="18" height="6" rx="1"/><rect x="3" y="14" width="18" height="6" rx="1"/><circle cx="7" cy="7" r="1" fill="currentColor"/><circle cx="7" cy="17" r="1" fill="currentColor"/></svg>Services</button>
      <button class="nav-item" :class="{active:section==='plans'}" @click="section='plans'"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="5" width="20" height="14" rx="2"/><path d="M2 10h20"/></svg>Billing</button>

      <div class="nav-group">System</div>
      <button class="nav-item" :class="{active:section==='system'}" @click="section='system';loadDiagnostics();loadAuditLogs()"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 00.3 1.9l.1.1a2 2 0 11-2.8 2.8l-.1-.1a1.7 1.7 0 00-1.9-.3 1.7 1.7 0 00-1 1.5V21a2 2 0 11-4 0v-.1a1.7 1.7 0 00-1.1-1.5 1.7 1.7 0 00-1.9.3l-.1.1a2 2 0 11-2.8-2.8l.1-.1a1.7 1.7 0 00.3-1.9 1.7 1.7 0 00-1.5-1H3a2 2 0 110-4h.1a1.7 1.7 0 001.5-1.1 1.7 1.7 0 00-.3-1.9l-.1-.1a2 2 0 112.8-2.8l.1.1a1.7 1.7 0 001.9.3H10a1.7 1.7 0 001-1.5V3a2 2 0 114 0v.1a1.7 1.7 0 001 1.5 1.7 1.7 0 001.9-.3l.1-.1a2 2 0 112.8 2.8l-.1.1a1.7 1.7 0 00-.3 1.9V10a1.7 1.7 0 001.5 1H21a2 2 0 110 4h-.1a1.7 1.7 0 00-1.5 1z"/></svg>Settings</button>

      <div class="sidebar-foot">
        <div class="avatar" :style="{background:`linear-gradient(135deg,var(--brand),var(--brand-2))`}">{{ initials }}</div>
        <div class="meta">{{ user.username }}<small>{{ user.role }}</small></div>
        <button class="icon-btn" style="width:28px;height:28px;margin-left:auto;border-radius:7px" @click="logout" title="Logout"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width:13px;height:13px"><path d="M9 21H5a2 2 0 01-2-2V5a2 2 0 012-2h4M16 17l5-5-5-5M21 12H9"/></svg></button>
      </div>
    </aside>

    <main class="main">
      <!-- Toast notification (temporary, auto-dismiss) -->
      <div v-if="notice" class="toast success" @click="notice=''">
        <svg class="toast-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 11.08V12a10 10 0 11-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>
        {{ notice }}
        <span class="toast-close" @click.stop="notice=''">✕</span>
      </div>

      <!-- Topbar -->
      <div class="topbar">
        <div class="topbar-left">
          <h2>{{ section==='overview'?'Dashboard':section==='customers'||section==='resellers'?'Users':section==='customer-detail'?'User Detail':section==='payments'?'Analytics':section==='plans'?'Billing':section==='nodes'?'Services':section==='tickets'?'Transactions':section==='system'?'Settings':'Panel' }}</h2>
          <p>{{ section==='overview'?`Welcome back, ${user.username}`:section==='customers'?'Manage accounts and access':section==='payments'?'Revenue and payment analytics':section==='plans'?'Subscription plans':section==='nodes'?'Servers and VPN configuration':section==='tickets'?'Payments and wallet operations':section==='system'?'Panel configuration':'Details' }}</p>
        </div>
        <div class="topbar-right">
          <div class="search-box"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="7"/><path d="M21 21l-4-4"/></svg><input v-model="search" @keyup.enter="loadDashboard" placeholder="Search..."></div>
          <div :class="['status-dot',{offline:!realtimeConnected}]" :title="realtimeConnected?'Connected':'Disconnected'"></div>
          <button class="icon-btn" title="Notifications"><span v-if="stats.pending_payments" class="notif-dot"></span><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width:16px;height:16px"><path d="M18 8a6 6 0 10-12 0c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.7 21a2 2 0 01-3.4 0"/></svg></button>
        </div>
      </div>
      <p v-if="error" class="alert danger">{{ error }}</p>

      <!-- ===== DASHBOARD ===== -->
      <div v-if="section==='overview'" class="page">
        <div class="grid g4">
          <div class="card stat-card"><div class="ic" style="background:rgba(91,157,255,.12);color:var(--brand)"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2v20M17 5H9.5a3.5 3.5 0 000 7h5a3.5 3.5 0 010 7H6"/></svg></div><div class="lbl">Revenue</div><h3>{{ formatMoney(stats.approved_payments) }}</h3><div class="trend"><b>{{ stats.pending_payments }}</b> pending</div></div>
          <div class="card stat-card"><div class="ic" style="background:rgba(124,92,255,.12);color:var(--brand-2)"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="9" cy="8" r="3.5"/><path d="M2.5 20a6.5 6.5 0 0113 0"/></svg></div><div class="lbl">Active Users</div><h3>{{ stats.active_customers }}</h3><div class="trend"><b>{{ activePercent }}%</b> of {{ stats.customers }}</div></div>
          <div class="card stat-card"><div class="ic" style="background:rgba(52,211,153,.12);color:var(--green)"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="4" width="18" height="6" rx="1"/><rect x="3" y="14" width="18" height="6" rx="1"/></svg></div><div class="lbl">Nodes Online</div><h3>{{ stats.nodes }}</h3><div class="trend"><b>{{ liveSessions.length }}</b> connections</div></div>
          <div class="card stat-card"><div class="ic" style="background:rgba(248,113,113,.12);color:var(--red)"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M10.3 3.9L1.8 18a2 2 0 001.7 3h17a2 2 0 001.7-3L13.7 3.9a2 2 0 00-3.4 0z"/></svg></div><div class="lbl">Open Tickets</div><h3>{{ stats.open_tickets }}</h3><div class="trend">{{ stats.open_tickets?'Needs attention':'All clear' }}</div></div>
        </div>

        <div class="grid g2" style="margin-top:16px">
          <div class="card"><div class="card-head"><div><h4>Usage Monitor</h4><div class="sub">Admin traffic usage over time</div></div></div><div class="chart-wrap"><svg viewBox="0 0 360 60" preserveAspectRatio="none" style="width:100%;height:100%"><polyline fill="none" stroke="var(--brand)" stroke-width="2" :points="rxPoints"/><polyline fill="none" stroke="var(--brand-2)" stroke-width="2" :points="txPoints"/></svg></div><div class="legend"><span><i style="background:var(--brand)"></i>↓ {{ formatBytes((stats.total_rx_bps||0)/8) }}/s</span><span><i style="background:var(--brand-2)"></i>↑ {{ formatBytes((stats.total_tx_bps||0)/8) }}/s</span></div></div>
          <div class="card"><div class="card-head"><div><h4>User Status</h4><div class="sub">Account distribution</div></div></div><div class="donut-wrap"><div class="donut"><svg width="150" height="150" viewBox="0 0 190 190" style="transform:rotate(-90deg)"><circle r="65" cx="95" cy="95" fill="none" stroke="var(--brand)" stroke-width="20" :stroke-dasharray="`${(statusSummary.active/Math.max(stats.customers,1))*408} 408`"/><circle r="65" cx="95" cy="95" fill="none" stroke="var(--amber)" stroke-width="20" :stroke-dasharray="`${((statusSummary.limited||0)/Math.max(stats.customers,1))*408} 408`" :stroke-dashoffset="`${-(statusSummary.active/Math.max(stats.customers,1))*408}`"/><circle r="65" cx="95" cy="95" fill="none" stroke="var(--red)" stroke-width="20" :stroke-dasharray="`${((statusSummary.expired||0)/Math.max(stats.customers,1))*408} 408`" :stroke-dashoffset="`${-((statusSummary.active+(statusSummary.limited||0))/Math.max(stats.customers,1))*408}`"/></svg><div class="center"><b>{{ stats.customers }}</b><small>Total</small></div></div><div class="dlist"><div class="row"><i style="background:var(--brand)"></i>Active<span class="v">{{ statusSummary.active }}</span></div><div class="row"><i style="background:var(--amber)"></i>Limited<span class="v">{{ statusSummary.limited||0 }}</span></div><div class="row"><i style="background:var(--red)"></i>Expired<span class="v">{{ statusSummary.expired||0 }}</span></div><div class="row"><i style="background:var(--muted)"></i>Disabled<span class="v">{{ statusSummary.disabled||0 }}</span></div></div></div></div>
        </div>

        <!-- Recent Users -->
        <div class="card" style="margin-top:16px"><div class="card-head"><div><h4>Recent Users</h4><div class="sub">Latest accounts</div></div><button class="btn-ghost btn-sm" @click="section='customers'">View All</button></div><div class="table-wrap"><table><thead><tr><th>User</th><th>Plan</th><th>Status</th><th>Balance</th><th>Joined</th></tr></thead><tbody><tr v-for="c in customers.slice(0,6)" :key="c.id" style="cursor:pointer" @click="openCustomer(c)"><td><div class="user-cell"><div class="av" :style="{background:`linear-gradient(135deg,hsl(${c.id*37%360},65%,50%),hsl(${c.id*67%360},65%,40%))`}">{{ c.username.slice(0,1).toUpperCase() }}</div><div><div class="name">{{ c.username }}</div><div class="email">{{ c.display_name||'—' }}</div></div></div></td><td>{{ c.plan||'Free' }}</td><td><span class="pill" :class="c.status==='active'?'ok':c.status==='disabled'?'bad':'warn'">{{ c.status }}</span></td><td>{{ formatMoney(c.credit) }}</td><td style="color:var(--muted)">{{ formatDate(c.created_at) }}</td></tr></tbody></table></div></div>
      </div>

      <!-- ===== USERS ===== -->
      <div v-else-if="section==='customers'||section==='resellers'" class="page">
        <div style="display:flex;align-items:center;gap:10px;margin-bottom:16px">
          <div class="tabs"><button :class="{on:section==='customers'&&customerView==='active'}" @click="section='customers';customerView='active'">Active</button><button :class="{on:section==='customers'&&customerView==='archived'}" @click="section='customers';customerView='archived'">Archived</button><button v-if="user.role==='owner'||user.role==='admin'" :class="{on:section==='resellers'}" @click="section='resellers';loadResellers()">Resellers</button></div>
          <button v-if="section==='customers'" class="btn-primary btn-sm" style="margin-left:auto" @click="customerModalOpen=true">+ New User</button>
        </div>
        <template v-if="section==='customers'">
          <div class="card"><div class="table-wrap"><table><thead><tr><th>User</th><th>Status</th><th>Plan</th><th>Balance</th><th>Created</th><th></th></tr></thead><tbody><tr v-for="c in filteredCustomers" :key="c.id"><td><div class="user-cell"><div class="av" :style="{background:`linear-gradient(135deg,hsl(${c.id*37%360},65%,50%),hsl(${c.id*67%360},65%,40%))`}">{{ c.username.slice(0,2).toUpperCase() }}</div><div><div class="name">{{ c.username }}</div><div class="email">{{ c.display_name||'—' }}</div></div></div></td><td><span class="pill" :class="c.status==='active'?'ok':c.status==='disabled'?'bad':'warn'">{{ c.status }}</span></td><td>{{ c.plan||'—' }}</td><td>{{ formatMoney(c.credit) }}</td><td style="color:var(--muted)">{{ formatDate(c.created_at) }}</td><td><button v-if="customerView==='active'" class="btn-ghost btn-sm" @click="openCustomer(c)">Detail</button><button v-else class="btn-primary btn-sm" @click="restoreDeletedCustomer(c as any)">Restore</button></td></tr><tr v-if="!filteredCustomers.length"><td colspan="6" class="empty-state"><p>No users found</p></td></tr></tbody></table></div></div>
        </template>
        <template v-else>
          <div class="grid" style="grid-template-columns:320px 1fr"><div class="card"><div class="card-head"><h4>New Reseller</h4></div><form class="form-stack" @submit.prevent="createReseller"><label>Username<input v-model.trim="resellerForm.username" required/></label><label>Password<input v-model="resellerForm.password" type="password" required/></label><button class="btn-primary" :disabled="busy">Create</button></form></div><div class="card"><div class="card-head"><h4>Resellers</h4></div><div class="table-wrap"><table><thead><tr><th>User</th><th>Credit</th><th>Status</th><th>Adjust</th><th></th></tr></thead><tbody><tr v-for="r in resellersList" :key="r.id"><td><b>{{ r.username }}</b></td><td>{{ formatMoney(r.credit) }}</td><td><span class="pill" :class="r.is_active?'ok':'bad'">{{ r.is_active?'Active':'Off' }}</span></td><td style="display:flex;gap:4px;align-items:center"><input v-model.number="resellerCreditForm[r.id]" type="number" style="width:70px;min-height:30px"/><button class="btn-ghost btn-sm" @click="adjustResellerCredit(r.id,true)">+</button><button class="btn-danger btn-sm" @click="adjustResellerCredit(r.id,false)">−</button></td><td><button class="btn-danger btn-sm" @click="deleteReseller(r.id)">Del</button></td></tr></tbody></table></div></div></div>
        </template>
      </div>

      <!-- ===== ANALYTICS (Payments) ===== -->
      <div v-else-if="section==='payments'" class="page">
        <div class="grid" style="grid-template-columns:340px 1fr"><div class="card"><div class="card-head"><h4>Record Payment</h4></div><form class="form-stack" @submit.prevent="createManualPayment"><label>Username<input v-model.trim="paymentForm.username" required/></label><label>Amount<input v-model.number="paymentForm.amount" type="number" min="0" required/></label><label>Method<select v-model="paymentForm.method"><option value="manual">Manual</option><option v-for="m in paymentMethods.filter(m=>m.is_active)" :key="m.id" :value="m.name">{{ m.name }}</option></select></label><label>Note<textarea v-model.trim="paymentForm.description" placeholder="Optional note"></textarea></label><button class="btn-primary" :disabled="busy">Record</button></form></div><div class="card"><div class="card-head"><div><h4>Payment History</h4><div class="sub">{{ payments.length }} records</div></div></div><div class="table-wrap"><table><thead><tr><th>User</th><th>Amount</th><th>Method</th><th>Status</th><th>Date</th><th></th></tr></thead><tbody><tr v-for="p in payments" :key="p.id"><td><div class="user-cell"><div class="av" :style="{background:`linear-gradient(135deg,hsl(${p.id*53%360},65%,50%),hsl(${p.id*89%360},65%,40%))`}">{{ (p.username||'?')[0].toUpperCase() }}</div>{{ p.username }}</div></td><td style="font-weight:600">{{ formatMoney(p.amount) }}</td><td>{{ p.method }}</td><td><span class="pill" :class="p.status==='approved'?'ok':p.status==='rejected'?'bad':'warn'">{{ p.status }}</span></td><td style="color:var(--muted)">{{ formatDate(p.created_at) }}</td><td><template v-if="p.status==='pending'"><button class="btn-primary btn-sm" style="margin-right:4px" @click="approvePayment(p,'approve')">✓</button><button class="btn-danger btn-sm" @click="approvePayment(p,'reject')">✗</button></template></td></tr></tbody></table></div></div></div>
      </div>

      <!-- ===== TRANSACTIONS (Tickets) ===== -->
      <div v-else-if="section==='tickets'" class="page">
        <div style="margin-bottom:16px"><button class="btn-primary btn-sm" @click="adminTicketForm={username:'',subject:'',priority:'normal',message:''};selectedTicket=null;customerModalOpen=false;nodeModalOpen=false;planModalOpen=true">+ New Ticket</button></div>
        <div class="card"><div class="card-head"><h4>Support Queue</h4></div><div class="table-wrap"><table><thead><tr><th>User</th><th>Subject</th><th>Priority</th><th>Status</th><th>Updated</th><th></th></tr></thead><tbody><tr v-for="t in tickets" :key="t.id"><td>{{ t.username }}</td><td>{{ t.subject }}</td><td><span class="pill warn">{{ t.priority }}</span></td><td><span class="pill" :class="t.status==='open'?'ok':'idle'">{{ t.status }}</span></td><td style="color:var(--muted)">{{ formatDate(t.updated_at) }}</td><td><button class="btn-ghost btn-sm" @click="loadTicket(t.id)">Open</button></td></tr><tr v-if="!tickets.length"><td colspan="6" class="empty-state"><p>No tickets</p></td></tr></tbody></table></div></div>
      </div>

      <!-- ===== BILLING (Plans) ===== -->
      <div v-else-if="section==='plans'" class="page">
        <div style="margin-bottom:16px"><button class="btn-primary btn-sm" @click="openNewPlan">+ New Plan</button></div>
        <div class="grid" style="grid-template-columns:repeat(auto-fill,minmax(280px,1fr))"><div v-for="plan in plans" :key="plan.id" class="card" :style="!plan.is_active?'opacity:.5':''"><div class="card-head"><div><h4>{{ plan.name }}</h4><span class="pill" :class="plan.is_active?'ok':'bad'" style="margin-top:4px">{{ plan.is_active?'Active':'Off' }}</span></div></div><div style="display:grid;grid-template-columns:repeat(4,1fr);gap:6px;margin:10px 0"><div style="text-align:center"><b style="font-size:15px">{{ plan.data_gb||'∞' }}</b><br><small style="color:var(--muted);font-size:10px">GB</small></div><div style="text-align:center"><b style="font-size:15px">{{ plan.speed_mbps||'∞' }}</b><br><small style="color:var(--muted);font-size:10px">Mbps</small></div><div style="text-align:center"><b style="font-size:15px">{{ plan.duration_days }}</b><br><small style="color:var(--muted);font-size:10px">Days</small></div><div style="text-align:center"><b style="font-size:15px">{{ formatMoney(plan.price) }}</b><br><small style="color:var(--muted);font-size:10px">Price</small></div></div><div class="action-row"><button class="btn-ghost btn-sm" @click="editPlan(plan)">Edit</button><button class="btn-danger btn-sm" :disabled="!plan.is_active" @click="archivePlan(plan)">Deactivate</button></div></div></div>
      </div>

      <!-- ===== SERVICES (Nodes) ===== -->
      <div v-else-if="section==='nodes'" class="page">
        <div class="tabs" style="margin-bottom:16px"><button :class="{on:infraTab==='nodes'}" @click="infraTab='nodes'">Nodes</button><button :class="{on:infraTab==='vpn'}" @click="infraTab='vpn'">VPN Config</button></div>
        <template v-if="infraTab==='nodes'">
          <div style="margin-bottom:16px"><button class="btn-primary btn-sm" @click="nodeModalOpen=true;nodeForm={name:'',public_ip:'',domain:''};nodeToken=''">+ New Node</button></div>
          <div class="node-grid"><div v-for="node in nodes" :key="node.id" class="node-card"><div style="display:flex;align-items:center;gap:8px;margin-bottom:10px"><span class="pill" :class="node.status==='online'?'ok':node.status==='disabled'?'bad':'warn'">{{ node.status }}</span><b style="font-size:14px">{{ node.name }}</b><small style="color:var(--muted);margin-left:auto;font-size:11px">{{ node.public_ip }}</small></div><div class="node-metrics"><span><b>{{ Math.round(node.status_metrics?.cpu_percent||0) }}%</b><small>CPU</small></span><span><b>{{ Math.round(node.status_metrics?.ram_percent||0) }}%</b><small>RAM</small></span><span><b>{{ Math.round(node.status_metrics?.disk_percent||0) }}%</b><small>Disk</small></span><span><b>{{ formatBytes(node.status_metrics?.rx_bps||0) }}/s</b><small>RX</small></span><span><b>{{ formatBytes(node.status_metrics?.tx_bps||0) }}/s</b><small>TX</small></span></div><div class="action-row"><button class="btn-ghost btn-sm" @click="createNodeTask(node,'service.restart',{service:'openvpn'})">Restart</button><button class="btn-ghost btn-sm" @click="rotateNodeToken(node)">Token</button><button v-if="node.status!=='disabled'" class="btn-danger btn-sm" @click="setNodeEnabled(node,false)">Disable</button><button v-else class="btn-ghost btn-sm" @click="setNodeEnabled(node,true)">Enable</button></div></div></div>
        </template>
        <template v-else>
          <div class="card" style="max-width:560px"><div class="card-head"><h4>VPN Configuration</h4></div><form class="form-stack" @submit.prevent="saveVPNSettings(false)"><div class="two-col"><label>Port<input v-model.number="vpnForm.openvpn_port" type="number"/></label><label>Protocol<select v-model="vpnForm.openvpn_protocol"><option value="udp">UDP</option><option value="tcp">TCP</option></select></label></div><label>Network<input v-model.trim="vpnForm.openvpn_network"/></label><div class="two-col"><label>DNS 1<input v-model.trim="vpnForm.dns_1"/></label><label>DNS 2<input v-model.trim="vpnForm.dns_2"/></label></div><label>IPSec PSK<input v-model.trim="vpnForm.ipsec_psk" type="password" placeholder="Shared secret"/></label><div class="action-row"><button class="btn-primary" :disabled="busy">Save</button><button class="btn-danger" type="button" :disabled="busy" @click="saveVPNSettings(true)">Save & Apply</button></div></form></div>
        </template>
      </div>

      <!-- ===== SETTINGS ===== -->
      <div v-else-if="section==='system'" class="page">
        <div class="settings-layout">
          <div class="settings-nav card" style="padding:12px">
            <button :class="{on:systemTab==='diagnostics'}" @click="systemTab='diagnostics'">Panel Status</button>
            <button :class="{on:systemTab==='audit'}" @click="systemTab='audit'">Audit Logs</button>
            <button :class="{on:systemTab==='backups'}" @click="systemTab='backups'">Backup & Export</button>
          </div>
          <div v-if="systemTab==='diagnostics'" class="card"><div class="card-head"><div><h4>Panel Status</h4><div class="sub">System health</div></div><button class="btn-ghost btn-sm" :disabled="diagnosticsLoading" @click="loadDiagnostics">{{ diagnosticsLoading?'...':'Refresh' }}</button></div><div v-if="diagnosticsData"><div style="display:grid;grid-template-columns:1fr 1fr;gap:10px;margin-bottom:14px"><div style="padding:10px;border:1px solid var(--border);border-radius:8px"><div style="color:var(--muted);font-size:10px;text-transform:uppercase">Disk</div><b style="font-size:14px">{{ diagnosticsData.disk }}</b></div><div style="padding:10px;border:1px solid var(--border);border-radius:8px"><div style="color:var(--muted);font-size:10px;text-transform:uppercase">Memory</div><b style="font-size:14px">{{ diagnosticsData.mem }}</b></div></div><div class="table-wrap"><table><thead><tr><th>Service</th><th>Status</th></tr></thead><tbody><tr v-for="c in diagnosticsData.checks" :key="c.name"><td>{{ c.name }}</td><td><span class="pill" :class="c.ok?'ok':'bad'">{{ c.ok?'OK':'Issue' }}</span></td></tr></tbody></table></div></div><div v-else class="empty-state"><p>Click Refresh to check</p></div></div>
          <div v-else-if="systemTab==='audit'" class="card"><div class="card-head"><h4>Audit Logs</h4><div style="display:flex;gap:4px"><button class="btn-ghost btn-sm" @click="auditOffset=Math.max(0,auditOffset-auditLimit);loadAuditLogs()">←</button><button class="btn-ghost btn-sm" @click="auditOffset+=auditLimit;loadAuditLogs()">→</button></div></div><div class="table-wrap"><table><thead><tr><th>Actor</th><th>Action</th><th>Entity</th><th>IP</th><th>Date</th></tr></thead><tbody><tr v-for="log in auditLogs" :key="log.id"><td>{{ log.actor }}</td><td><span class="pill warn">{{ log.action }}</span></td><td>{{ log.entity_type }}#{{ log.entity_id }}</td><td style="color:var(--muted)">{{ log.ip }}</td><td style="color:var(--muted)">{{ formatDate(log.created_at) }}</td></tr></tbody></table></div></div>
          <div v-else class="card"><div class="card-head"><h4>Backup & Export</h4></div><p style="color:var(--muted);margin-bottom:12px;font-size:12.5px">Download CSV snapshots. Automated backups run daily at 2 AM.</p><div class="action-row"><button class="btn-primary btn-sm" @click="exportCSV('customers')">Users</button><button class="btn-primary btn-sm" @click="exportCSV('payments')">Payments</button><button class="btn-primary btn-sm" @click="exportCSV('radacct')">Sessions</button><button class="btn-primary btn-sm" @click="exportCSV('wallet-transactions')">Wallet</button></div></div>
        </div>
      </div>
    </main>

    <!-- ===== MODALS ===== -->

    <!-- User Detail Modal -->
    <div v-if="section==='customer-detail'&&selectedCustomer" class="modal-backdrop" @click.self="section='customers';selectedCustomer=null">
      <div class="modal modal-lg">
        <div class="modal-head"><h3>{{ selectedCustomer.username }}</h3><button class="modal-close" @click="section='customers';selectedCustomer=null">✕</button></div>
        <div style="display:flex;align-items:center;gap:12px;margin-bottom:16px;padding-bottom:16px;border-bottom:1px solid var(--border)"><div class="avatar" :style="{background:`linear-gradient(135deg,hsl(${selectedCustomer.id*37%360},65%,50%),hsl(${selectedCustomer.id*67%360},65%,40%))`,width:'48px',height:'48px',fontSize:'16px'}">{{ selectedCustomer.username.slice(0,2).toUpperCase() }}</div><div style="flex:1"><div style="font-weight:600">{{ selectedCustomer.display_name||selectedCustomer.username }}</div><div style="color:var(--muted);font-size:12px">{{ selectedCustomer.plan||'No plan' }} · <span class="pill" :class="selectedCustomer.status==='active'?'ok':'warn'">{{ selectedCustomer.status }}</span></div></div><div style="text-align:right"><div style="color:var(--muted);font-size:11px">Balance</div><div style="font-size:20px;font-weight:700">{{ formatMoney(selectedCustomer.credit) }}</div></div></div>
        <div class="tabs"><button :class="{on:detailTab==='profile'}" @click="detailTab='profile'">Profile</button><button :class="{on:detailTab==='usage'}" @click="detailTab='usage'">Usage</button><button :class="{on:detailTab==='history'}" @click="detailTab='history'">History</button></div>
        <div v-if="detailTab==='profile'"><div class="grid" style="grid-template-columns:1fr 1fr;gap:14px"><div><form class="form-stack" @submit.prevent="saveCustomerDetail"><label>Display Name<input v-model.trim="detailForm.display_name"/></label><div class="two-col"><label>Status<select v-model="detailForm.status"><option value="active">Active</option><option value="limited">Limited</option><option value="expired">Expired</option><option value="disabled">Disabled</option></select></label><label>Plan<select v-model.number="detailForm.plan_id" @change="applyDetailPlan"><option :value="0">None</option><option v-for="p in plans" :key="p.id" :value="p.id">{{ p.name }}</option></select></label></div><div class="two-col"><label>Data GB<input v-model.number="detailForm.data_gb" type="number" min="0"/></label><label>Speed Mbps<input v-model.number="detailForm.speed_mbps" type="number" min="0"/></label></div><label>Add Days<input v-model.number="detailForm.days" type="number" min="0"/></label><label>Notes<textarea v-model.trim="detailForm.notes"></textarea></label><button class="btn-primary" :disabled="busy">Save</button></form></div><div><form class="form-stack" @submit.prevent="resetCustomerPassword"><label>New Password<input v-model="passwordForm.password"/></label><button class="btn-primary" :disabled="busy">Reset Password</button></form><hr style="border:none;border-top:1px solid var(--border);margin:14px 0"><form class="form-stack" @submit.prevent="renewCustomerPlan"><label>Apply Plan<select v-model.number="renewForm.plan_id"><option :value="0">Select</option><option v-for="p in activePlans" :key="p.id" :value="p.id">{{ p.name }} · {{ formatMoney(p.price) }}</option></select></label><button class="btn-primary" :disabled="busy||!renewForm.plan_id">Activate</button></form><hr style="border:none;border-top:1px solid var(--border);margin:14px 0"><form class="form-stack" @submit.prevent="adjustWallet"><label>Wallet ±<input v-model.number="walletForm.amount" type="number"/></label><button class="btn-ghost" :disabled="busy">Adjust</button></form><div class="action-row" style="margin-top:14px"><button class="btn-ghost btn-sm" :disabled="busy" @click="setSelectedCustomerStatus('active')">Enable</button><button class="btn-danger btn-sm" :disabled="busy" @click="setSelectedCustomerStatus('disabled')">Disable</button><button class="btn-danger btn-sm" :disabled="busy" @click="archiveSelectedCustomer">Archive</button></div></div></div></div>
        <div v-else-if="detailTab==='usage'&&selectedUsage"><div class="grid g4" style="margin-bottom:14px;grid-template-columns:repeat(5,1fr)"><div class="card" style="padding:10px;text-align:center"><div style="color:var(--muted);font-size:10px;text-transform:uppercase">Total</div><b>{{ formatBytes(selectedUsage.total_usage_bytes) }}</b></div><div class="card" style="padding:10px;text-align:center"><div style="color:var(--muted);font-size:10px;text-transform:uppercase">Down</div><b>{{ formatBytes(selectedUsage.total_input_bytes) }}</b></div><div class="card" style="padding:10px;text-align:center"><div style="color:var(--muted);font-size:10px;text-transform:uppercase">Up</div><b>{{ formatBytes(selectedUsage.total_output_bytes) }}</b></div><div class="card" style="padding:10px;text-align:center"><div style="color:var(--muted);font-size:10px;text-transform:uppercase">Left</div><b>{{ selectedUsage.remaining_bytes===undefined?'∞':formatBytes(selectedUsage.remaining_bytes) }}</b></div><div class="card" style="padding:10px;text-align:center"><div style="color:var(--muted);font-size:10px;text-transform:uppercase">Sessions</div><b>{{ selectedUsage.active_sessions }}</b></div></div><div class="table-wrap"><table><thead><tr><th>Status</th><th>IP</th><th>Duration</th><th>↓</th><th>↑</th><th>Started</th></tr></thead><tbody><tr v-for="s in selectedUsage.sessions" :key="s.id"><td><span class="pill" :class="s.online?'ok':'idle'">{{ s.online?'on':'off' }}</span></td><td>{{ s.framed_ip||'—' }}</td><td>{{ formatDuration(s.session_seconds) }}</td><td>{{ formatBytes(s.input_bytes) }}</td><td>{{ formatBytes(s.output_bytes) }}</td><td style="color:var(--muted)">{{ formatDate(s.start_time) }}</td></tr></tbody></table></div></div>
        <div v-else-if="detailTab==='history'"><div class="table-wrap" style="margin-bottom:14px"><h4 style="margin-bottom:10px">Wallet</h4><table><thead><tr><th>Amount</th><th>Type</th><th>Note</th><th>Date</th></tr></thead><tbody><tr v-for="tx in (selectedCustomer.wallet_transactions||[])" :key="tx.id"><td :style="{color:tx.amount>=0?'var(--green)':'var(--red)',fontWeight:600}">{{ tx.amount>=0?'+':'' }}{{ formatMoney(tx.amount) }}</td><td><span class="pill warn">{{ tx.type }}</span></td><td style="color:var(--muted)">{{ tx.description||'—' }}</td><td style="color:var(--muted)">{{ formatDate(tx.created_at) }}</td></tr></tbody></table></div><div class="table-wrap"><h4 style="margin-bottom:10px">Subscriptions</h4><table><thead><tr><th>Plan</th><th>Status</th><th>Paid</th><th>Expires</th></tr></thead><tbody><tr v-for="sub in (selectedCustomer.subscriptions||[])" :key="sub.id"><td>{{ sub.plan||'—' }}</td><td><span class="pill" :class="sub.status==='active'?'ok':'bad'">{{ sub.status }}</span></td><td>{{ formatMoney(sub.paid_amount) }}</td><td style="color:var(--muted)">{{ formatDate(sub.expires_at) }}</td></tr></tbody></table></div></div>
      </div>
    </div>

    <!-- Ticket Detail Modal -->
    <div v-if="selectedTicket" class="modal-backdrop" @click.self="selectedTicket=null">
      <div class="modal">
        <div class="modal-head"><h3>#{{ selectedTicket.id }}: {{ selectedTicket.subject }}</h3><button class="modal-close" @click="selectedTicket=null">✕</button></div>
        <div style="display:flex;gap:8px;align-items:center;margin-bottom:14px"><span class="pill" :class="selectedTicket.status==='open'?'ok':'idle'">{{ selectedTicket.status }}</span><span style="color:var(--muted);font-size:12px">{{ selectedTicket.username }} · {{ selectedTicket.priority }}</span><button v-if="selectedTicket.status==='open'" class="btn-danger btn-sm" style="margin-left:auto" @click="setTicketStatus(selectedTicket,'closed')">Close</button><button v-else class="btn-ghost btn-sm" style="margin-left:auto" @click="setTicketStatus(selectedTicket,'open')">Reopen</button></div>
        <div style="display:flex;flex-direction:column;gap:8px;max-height:400px;overflow-y:auto;margin-bottom:14px;padding:4px"><div v-for="msg in selectedTicket.messages" :key="msg.id" style="border:1px solid var(--border);border-radius:10px;padding:12px" :style="msg.sender_type==='admin'?'border-color:rgba(91,157,255,.2);background:rgba(91,157,255,.03);margin-left:24px':'margin-right:24px;background:var(--surface-2)'"><div style="display:flex;justify-content:space-between;margin-bottom:6px"><b style="font-size:12px">{{ msg.sender_name }}</b><small style="color:var(--muted);font-size:11px">{{ formatDate(msg.created_at) }}</small></div><p style="white-space:pre-wrap;font-size:13.5px;line-height:1.5">{{ msg.message }}</p></div></div>
        <form class="form-stack" style="border-top:1px solid var(--border);padding-top:12px" @submit.prevent="replyTicket"><label>Reply<textarea v-model.trim="ticketReply" placeholder="Type your reply..."></textarea></label><button class="btn-primary" :disabled="busy||!ticketReply.trim()">Send</button></form>
      </div>
    </div>

    <!-- Create User Modal -->
    <div v-if="customerModalOpen" class="modal-backdrop" @click.self="customerModalOpen=false">
      <div class="modal"><div class="modal-head"><h3>New User</h3><button class="modal-close" @click="customerModalOpen=false">✕</button></div><form class="form-stack" @submit.prevent="createCustomer"><div class="two-col"><label>Username<input v-model.trim="createForm.username" required/></label><label>Password<input v-model="createForm.password" required/></label></div><label>Display Name<input v-model.trim="createForm.display_name"/></label><label>Plan<select v-model.number="createForm.plan_id" @change="applyCreatePlan"><option :value="0">No plan</option><option v-for="p in activePlans" :key="p.id" :value="p.id">{{ p.name }}</option></select></label><div class="two-col"><label>Data GB (0=∞)<input v-model.number="createForm.data_gb" type="number" min="0"/></label><label>Speed Mbps (0=∞)<input v-model.number="createForm.speed_mbps" type="number" min="0"/></label></div><label>Duration Days<input v-model.number="createForm.days" type="number" min="0"/></label><div class="action-row"><button class="btn-primary" :disabled="busy">{{ busy?'Creating...':'Create' }}</button><button type="button" class="btn-ghost" @click="customerModalOpen=false">Cancel</button></div></form></div>
    </div>

    <!-- Create Node Modal -->
    <div v-if="nodeModalOpen" class="modal-backdrop" @click.self="nodeModalOpen=false">
      <div class="modal"><div class="modal-head"><h3>New Node</h3><button class="modal-close" @click="nodeModalOpen=false">✕</button></div><form class="form-stack" @submit.prevent="createNode"><div class="two-col"><label>Name<input v-model.trim="nodeForm.name" required/></label><label>Public IP<input v-model.trim="nodeForm.public_ip" required/></label></div><label>Domain<input v-model.trim="nodeForm.domain" placeholder="Optional"/></label><button class="btn-primary" :disabled="busy">Create</button></form><div v-if="nodeToken" style="margin-top:14px;border:1px solid rgba(91,157,255,.2);border-radius:10px;padding:12px;background:rgba(91,157,255,.03)"><small style="color:var(--brand);font-weight:600">Token (copy now — shown once):</small><code style="display:block;margin-top:6px;word-break:break-all;background:var(--surface-2);padding:8px;border-radius:6px;font-size:12px">{{ nodeToken }}</code><button class="btn-ghost btn-sm" style="margin-top:8px" @click="copyToClipboard(nodeToken)">Copy</button></div></div>
    </div>

    <!-- Create/Edit Plan Modal -->
    <div v-if="planModalOpen" class="modal-backdrop" @click.self="planModalOpen=false">
      <div class="modal"><div class="modal-head"><h3>{{ editingPlanId?'Edit Plan':'New Plan' }}</h3><button class="modal-close" @click="planModalOpen=false">✕</button></div><form class="form-stack" @submit.prevent="savePlan"><label>Name<input v-model.trim="planForm.name" required/></label><div class="two-col"><label>Data GB<input v-model.number="planForm.data_gb" type="number" min="0"/></label><label>Speed Mbps<input v-model.number="planForm.speed_mbps" type="number" min="0"/></label></div><div class="two-col"><label>Days<input v-model.number="planForm.duration_days" type="number" min="0"/></label><label>Price<input v-model.number="planForm.price" type="number" min="0"/></label></div><label style="display:flex;align-items:center;gap:8px;flex-direction:row"><input v-model="planForm.is_active" type="checkbox" style="width:16px;min-height:16px"/>Active</label><div class="action-row"><button class="btn-primary" :disabled="busy">{{ editingPlanId?'Update':'Create' }}</button><button type="button" class="btn-ghost" @click="planModalOpen=false">Cancel</button></div></form></div>
    </div>

    <!-- New Ticket Modal (reusing planModalOpen flag temporarily - needs separate flag) -->

  </template>
</template>
