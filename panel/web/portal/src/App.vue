<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

type Screen = 'loading' | 'login' | 'portal'

interface PortalCustomer {
  id?: number
  username: string
  display_name?: string
  status?: string
  plan?: string
  credit?: number
  created_at?: string
  max_data_bytes?: string
  sub_token?: string
  subscription?: {
    plan?: string
    status?: string
    expires_at?: string
  }
}

interface Payment { id: number; username: string; amount: number; method: string; status: string; intent_type: string; intent_id?: number; intent_label: string; created_at: string; updated_at: string }
interface PaymentMethod { id: number; name: string; type: string; instructions: string; is_active: boolean; sort_order: number; created_at: string }
interface Ticket { id: number; username: string; subject: string; status: string; priority: string; created_at: string; updated_at: string; closed_at: string }
interface TicketMessage { id: number; ticket_id: number; sender_type: string; sender_name: string; message: string; created_at: string }
interface TicketDetail extends Ticket { messages: TicketMessage[] }
interface Plan { id: number; name: string; data_gb: number; speed_mbps: number; duration_days: number; price: number; is_active: boolean; sort_order: number; created_at: string }
interface VpnProfile { type: string; name: string; filename: string; available: boolean; remote: string; port: number; protocol: string; node: string; download: string }
interface UsageSession { id: number; start_time: string; stop_time: string; session_seconds: number; input_bytes: number; output_bytes: number; total_bytes: number; framed_ip: string; online: boolean }
interface UsageSummary { online: boolean; active_sessions: number; total_input_bytes: number; total_output_bytes: number; total_usage_bytes: number; max_data_bytes: number; remaining_bytes?: number; last_connected_at: string; last_disconnected_at: string; sessions: UsageSession[] }
interface ApiError extends Error { status?: number }

const screen = ref<Screen>('loading')
const loginForm = ref({ username: '', password: '' })
const customer = ref<PortalCustomer | null>(null)
const payments = ref<Payment[]>([])
const paymentMethods = ref<PaymentMethod[]>([])
const tickets = ref<Ticket[]>([])
const selectedTicket = ref<TicketDetail | null>(null)
const ticketForm = ref({ subject: '', priority: 'normal', message: '' })
const ticketReply = ref('')
const plans = ref<Plan[]>([])
const profiles = ref<VpnProfile[]>([])
const portalTab = ref<'overview' | 'billing' | 'support'>('overview')
const usage = ref<UsageSummary | null>(null)
const paymentForm = ref({ amount: 0, method: 'manual', receipt: '' })
const renewForm = ref({ plan_id: 0 })
const busy = ref(false)
const error = ref('')
const notice = ref('')

const titleName = computed(() => customer.value?.display_name || customer.value?.username || 'Customer')
const planName = computed(() => customer.value?.subscription?.plan || customer.value?.plan || 'Starter')
const status = computed(() => customer.value?.subscription?.status || customer.value?.status || 'active')
const dataLimit = computed(() => {
  const raw = Number(customer.value?.max_data_bytes || 0)
  if (!raw) return 'Unlimited'
  return `${Math.round((raw / 1024 / 1024 / 1024) * 10) / 10} GB`
})
const accountScore = computed(() => {
  let score = status.value === 'active' ? 70 : 38
  if (customer.value?.subscription?.expires_at) score += 15
  if ((customer.value?.credit || 0) > 0) score += 10
  if (customer.value?.max_data_bytes) score += 5
  return Math.min(100, score)
})
const selectedPlan = computed(() => plans.value.find((plan) => plan.id === Number(renewForm.value.plan_id)))
const openvpnProfile = computed(() => profiles.value.find((profile) => profile.type === 'openvpn'))
const l2tpProfile = computed(() => profiles.value.find((profile) => profile.type === 'l2tp'))
const ikev2Profile = computed(() => profiles.value.find((profile) => profile.type === 'ikev2'))
const windowOrigin = computed(() => window.location.origin)

function copyToClipboard() {
  const input = document.getElementById('sub-url-input') as HTMLInputElement
  if (input) {
    const text = input.value
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
}

const walletCredit = computed(() => Number(customer.value?.credit || 0))
const requiredTopup = computed(() => Math.max(0, Number(selectedPlan.value?.price || 0) - walletCredit.value))
const selectedPaymentMethod = computed(() => paymentMethods.value.find((method) => method.name === paymentForm.value.method))
const usagePercent = computed(() => {
  if (!usage.value?.max_data_bytes) return 0
  return Math.min(100, Math.round((usage.value.total_usage_bytes / usage.value.max_data_bytes) * 100))
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

async function boot() {
  error.value = ''
  try {
    const [res, paymentRes, methodRes, ticketRes, plansRes, profilesRes, usageRes] = await Promise.all([
      api<{ ok: boolean; customer: PortalCustomer }>('/api/portal/me'),
      api<{ ok: boolean; payments: Payment[] }>('/api/portal/payments'),
      api<{ ok: boolean; methods: PaymentMethod[] }>('/api/portal/payment-methods'),
      api<{ ok: boolean; tickets: Ticket[] }>('/api/portal/tickets'),
      api<{ ok: boolean; plans: Plan[] }>('/api/portal/plans'),
      api<{ ok: boolean; profiles: VpnProfile[] }>('/api/portal/profiles'),
      api<{ ok: boolean; usage: UsageSummary }>('/api/portal/usage')
    ])
    customer.value = res.customer
    payments.value = paymentRes.payments || []
    paymentMethods.value = methodRes.methods || []
    if (paymentMethods.value.length && (!paymentForm.value.method || paymentForm.value.method === 'manual')) paymentForm.value.method = paymentMethods.value[0].name
    tickets.value = ticketRes.tickets || []
    plans.value = plansRes.plans || []
    profiles.value = profilesRes.profiles || []
    usage.value = usageRes.usage
    if (!renewForm.value.plan_id && plans.value.length) renewForm.value.plan_id = plans.value[0].id
    screen.value = 'portal'
  } catch (err) {
    const apiErr = err as ApiError
    if (apiErr.status && apiErr.status !== 401) error.value = friendlyError(err)
    screen.value = 'login'
  }
}

async function login() {
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    await api<{ ok: boolean; username: string }>('/api/auth/customer', { method: 'POST', body: JSON.stringify(loginForm.value) })
    await boot()
    notice.value = 'Signed in successfully.'
  } catch (err) {
    error.value = friendlyError(err)
  } finally {
    busy.value = false
  }
}

async function logout() {
  await api<{ ok: boolean }>('/api/auth/customer/logout', { method: 'POST' }).catch(() => null)
  customer.value = null
  payments.value = []
  paymentMethods.value = []
  tickets.value = []
  selectedTicket.value = null
  plans.value = []
  profiles.value = []
  usage.value = null
  screen.value = 'login'
}

async function submitRenewal() {
  if (!renewForm.value.plan_id) return
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    const res = await api<{ ok: boolean; renewed: boolean; payment_required: boolean; required_amount?: number; payment_id?: number }>('/api/portal/renew', { method: 'POST', body: JSON.stringify(renewForm.value) })
    if (res.renewed) notice.value = 'Plan activated. Wallet was charged.'
    else if (res.payment_required) notice.value = `Payment request #${res.payment_id} created for ${formatMoney(res.required_amount)}.`
    await boot()
  } catch (err) {
    error.value = friendlyError(err)
  } finally {
    busy.value = false
  }
}

async function submitPaymentRequest() {
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    await api<{ ok: boolean; id: number }>('/api/portal/payments', { method: 'POST', body: JSON.stringify(paymentForm.value) })
    notice.value = 'Payment request submitted. Admin will review it.'
    paymentForm.value = { amount: 0, method: 'manual', receipt: '' }
    await boot()
  } catch (err) {
    error.value = friendlyError(err)
  } finally {
    busy.value = false
  }
}


async function createTicket() {
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    const res = await api<{ ok: boolean; id: number }>('/api/portal/tickets', { method: 'POST', body: JSON.stringify(ticketForm.value) })
    notice.value = 'Ticket created.'
    ticketForm.value = { subject: '', priority: 'normal', message: '' }
    await boot()
    await openTicket(res.id)
  } catch (err) {
    error.value = friendlyError(err)
  } finally {
    busy.value = false
  }
}

async function openTicket(id: number) {
  busy.value = true
  error.value = ''
  try {
    const res = await api<{ ok: boolean; ticket: TicketDetail }>(`/api/portal/tickets/${id}`)
    selectedTicket.value = res.ticket
    ticketReply.value = ''
  } catch (err) {
    error.value = friendlyError(err)
  } finally {
    busy.value = false
  }
}

async function replyTicket() {
  if (!selectedTicket.value || !ticketReply.value.trim()) return
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    await api<{ ok: boolean }>(`/api/portal/tickets/${selectedTicket.value.id}/reply`, { method: 'POST', body: JSON.stringify({ message: ticketReply.value }) })
    notice.value = 'Reply sent.'
    await openTicket(selectedTicket.value.id)
    await boot()
  } catch (err) {
    error.value = friendlyError(err)
  } finally {
    busy.value = false
  }
}

async function closeTicket() {
  if (!selectedTicket.value) return
  busy.value = true
  error.value = ''
  try {
    await api<{ ok: boolean }>(`/api/portal/tickets/${selectedTicket.value.id}/close`, { method: 'POST' })
    notice.value = 'Ticket closed.'
    await openTicket(selectedTicket.value.id)
    await boot()
  } catch (err) {
    error.value = friendlyError(err)
  } finally {
    busy.value = false
  }
}

function friendlyError(err: unknown) {
  if (err instanceof Error) return err.message.replace(/_/g, ' ')
  return 'Unexpected error'
}

function formatMoney(value?: number) {
  return `${new Intl.NumberFormat('en', { maximumFractionDigits: 0 }).format(value || 0)} IRT`
}
function formatGB(value?: number) { return value && value > 0 ? `${new Intl.NumberFormat('en', { maximumFractionDigits: 2 }).format(value)} GB` : 'Unlimited' }
function formatSpeed(value?: number) { return value && value > 0 ? `${new Intl.NumberFormat('en', { maximumFractionDigits: 2 }).format(value)} Mbps` : 'Unlimited' }
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
  if (h) return `${h}h ${m}m`
  if (m) return `${m}m`
  return `${s}s`
}

function formatDate(value?: string) {
  if (!value) return 'Not set'
  return new Intl.DateTimeFormat('en', { year: 'numeric', month: 'short', day: '2-digit' }).format(new Date(value))
}

onMounted(boot)
</script>


<template>
  <div v-if="screen==='loading'" class="loading"><div class="spinner"></div></div>

  <div v-else-if="screen==='login'" class="auth-screen">
    <div class="auth-hero">
      <div class="logo-row"><div class="logo">K</div><div><b style="font-size:17px">KorisPanel</b><small style="color:var(--muted);font-size:12px">Client Portal</small></div></div>
      <h1>Your VPN<br>Dashboard</h1>
      <p>Monitor your account, download connection profiles, manage subscription, and get support.</p>
      <div class="chips"><span>OpenVPN</span><span>L2TP/IPSec</span><span>IKEv2</span></div>
    </div>
    <div class="auth-card">
      <h2>Welcome back</h2>
      <div class="sub">Sign in with your account credentials</div>
      <form class="form-stack" @submit.prevent="login">
        <label>Username<input v-model.trim="loginForm.username" required placeholder="Your username"/></label>
        <label>Password<input v-model="loginForm.password" type="password" required placeholder="Your password"/></label>
        <button class="btn-primary" :disabled="busy">{{ busy?'Signing in...':'Sign In' }}</button>
      </form>
      <p v-if="error" class="alert danger">{{ error }}</p>
    </div>
  </div>

  <div v-else class="portal-shell">
    <div class="portal-topbar">
      <div class="logo-row"><div class="logo" style="width:34px;height:34px;border-radius:9px;font-size:14px">K</div><div><b>KorisPanel</b><small style="color:var(--muted);font-size:11px">Client Portal</small></div></div>
      <div style="display:flex;align-items:center;gap:10px"><span class="pill" :class="status==='active'?'ok':'warn'">{{ status }}</span><button class="btn-ghost" @click="logout">Logout</button></div>
    </div>

    <div class="welcome-card">
      <div><div class="eyebrow">{{ status }}</div><h1>Hello, {{ titleName }}</h1><p>Your VPN account is active and ready to connect.</p></div>
      <a v-if="openvpnProfile?.available" class="btn-primary" :href="openvpnProfile.download" download>Download Config</a>
      <button v-else class="btn-primary" disabled>No server available</button>
    </div>

    <p v-if="notice" class="alert success">{{ notice }}</p>
    <p v-if="error" class="alert danger">{{ error }}</p>

    <div class="tabs">
      <button :class="{on:portalTab==='overview'}" @click="portalTab='overview'">Dashboard</button>
      <button :class="{on:portalTab==='billing'}" @click="portalTab='billing'">Billing</button>
      <button :class="{on:portalTab==='support'}" @click="portalTab='support'">Support</button>
    </div>

    <!-- Dashboard -->
    <div v-if="portalTab==='overview'">
      <div class="stats-row">
        <div class="stat-card main"><div class="lbl">Plan</div><h3>{{ planName }}</h3><div class="sub">Expires: {{ formatDate(customer?.subscription?.expires_at) }}</div></div>
        <div class="stat-card"><div class="lbl">Data</div><h3>{{ dataLimit }}</h3><div class="sub">{{ usage?.online?`${usage.active_sessions} active`:'Offline' }}</div></div>
        <div class="stat-card"><div class="lbl">Wallet</div><h3>{{ formatMoney(customer?.credit) }}</h3><div class="sub">Balance</div></div>
      </div>
      <div v-if="usage" class="card"><div class="card-head"><div><h4>Usage</h4><div class="sub">{{ formatBytes(usage.total_usage_bytes) }} / {{ usage.max_data_bytes?formatBytes(usage.max_data_bytes):'Unlimited' }}</div></div><span class="pill" :class="usage.online?'ok':'idle'">{{ usage.online?'Online':'Offline' }}</span></div><div class="usage-bar"><i :style="{width:usagePercent+'%'}"></i></div><div style="display:flex;justify-content:space-between;font-size:12px;color:var(--muted)"><span>↓ {{ formatBytes(usage.total_input_bytes) }}</span><span>↑ {{ formatBytes(usage.total_output_bytes) }}</span><span>{{ usagePercent }}%</span></div></div>
      <div class="card"><div class="card-head"><h4>Profiles</h4></div><div class="profile-grid"><div v-for="p in profiles" :key="p.type" class="profile-card"><div class="info"><b>{{ p.name }}</b><span>{{ p.remote }}:{{ p.port }} · {{ p.protocol }}</span></div><a v-if="p.available" class="btn-primary" style="padding:6px 12px;font-size:12px" :href="p.download" download>Get</a><button v-else class="btn-ghost" style="padding:6px 12px;font-size:12px" disabled>N/A</button></div></div></div>
      <div v-if="usage?.sessions?.length" class="card"><div class="card-head"><h4>Sessions</h4></div><div style="overflow-x:auto"><table><thead><tr><th>Status</th><th>IP</th><th>Duration</th><th>↓</th><th>↑</th></tr></thead><tbody><tr v-for="s in usage.sessions.slice(0,8)" :key="s.id"><td><span class="pill" :class="s.online?'ok':'idle'">{{ s.online?'on':'off' }}</span></td><td>{{ s.framed_ip||'—' }}</td><td>{{ formatDuration(s.session_seconds) }}</td><td>{{ formatBytes(s.input_bytes) }}</td><td>{{ formatBytes(s.output_bytes) }}</td></tr></tbody></table></div></div>
    </div>

    <!-- Billing -->
    <div v-else-if="portalTab==='billing'">
      <div class="card"><div class="card-head"><h4>Renew Plan</h4></div><form class="form-stack" @submit.prevent="submitRenewal"><label>Plan<select v-model.number="renewForm.plan_id"><option v-for="p in plans" :key="p.id" :value="p.id">{{ p.name }} — {{ formatGB(p.data_gb) }} · {{ p.duration_days }}d · {{ formatMoney(p.price) }}</option></select></label><div v-if="selectedPlan&&selectedPlan.price>0&&walletCredit<selectedPlan.price" style="font-size:12px;color:var(--amber)">Insufficient balance. A payment request will be created.</div><button class="btn-primary" :disabled="busy||!renewForm.plan_id">{{ busy?'...':'Activate Plan' }}</button></form></div>
      <div class="card"><div class="card-head"><h4>Top-up Wallet</h4></div><form class="form-stack" @submit.prevent="submitPaymentRequest"><label>Amount<input v-model.number="paymentForm.amount" type="number" min="1" required/></label><label>Method<select v-model="paymentForm.method"><option v-for="m in paymentMethods" :key="m.id" :value="m.name">{{ m.name }}</option></select></label><div v-if="selectedPaymentMethod?.instructions" style="border:1px solid var(--border);border-radius:8px;padding:10px;color:var(--muted);font-size:12px;white-space:pre-wrap">{{ selectedPaymentMethod.instructions }}</div><label>Receipt<textarea v-model.trim="paymentForm.receipt" placeholder="Transfer reference"></textarea></label><button class="btn-primary" :disabled="busy||paymentForm.amount<=0">Submit</button></form></div>
      <div class="card"><div class="card-head"><h4>History</h4></div><div style="overflow-x:auto"><table><thead><tr><th>Amount</th><th>Method</th><th>Status</th><th>Date</th></tr></thead><tbody><tr v-for="p in payments" :key="p.id"><td style="font-weight:600">{{ formatMoney(p.amount) }}</td><td>{{ p.method }}</td><td><span class="pill" :class="p.status">{{ p.status }}</span></td><td style="color:var(--muted)">{{ formatDate(p.created_at) }}</td></tr><tr v-if="!payments.length"><td colspan="4" style="text-align:center;color:var(--muted);padding:20px">No payments</td></tr></tbody></table></div></div>
    </div>

    <!-- Support -->
    <div v-else-if="portalTab==='support'">
      <div style="margin-bottom:16px"><button class="btn-primary" style="padding:8px 14px;font-size:13px" @click="selectedTicket=null">+ New Ticket</button></div>
      <div class="card"><div class="card-head"><h4>My Tickets</h4></div><div style="overflow-x:auto"><table><thead><tr><th>Subject</th><th>Priority</th><th>Status</th><th></th></tr></thead><tbody><tr v-for="t in tickets" :key="t.id"><td>{{ t.subject }}</td><td><span class="pill warn">{{ t.priority }}</span></td><td><span class="pill" :class="t.status==='open'?'ok':'idle'">{{ t.status }}</span></td><td><button class="btn-ghost" style="padding:4px 10px;font-size:11px" @click="openTicket(t.id)">View</button></td></tr><tr v-if="!tickets.length"><td colspan="4" style="text-align:center;color:var(--muted);padding:20px">No tickets</td></tr></tbody></table></div></div>
    </div>

    <!-- Ticket Detail Modal -->
    <div v-if="selectedTicket" class="modal-backdrop" @click.self="selectedTicket=null">
      <div class="modal">
        <div class="modal-head"><h3>#{{ selectedTicket.id }}: {{ selectedTicket.subject }}</h3><button class="modal-close" @click="selectedTicket=null">✕</button></div>
        <div style="display:flex;gap:8px;align-items:center;margin-bottom:12px"><span class="pill" :class="selectedTicket.status==='open'?'ok':'idle'">{{ selectedTicket.status }}</span><span style="color:var(--muted);font-size:12px">{{ selectedTicket.priority }}</span><button v-if="selectedTicket.status==='open'" class="btn-ghost" style="margin-left:auto;padding:5px 10px;font-size:11px" @click="closeTicket">Close</button></div>
        <div class="ticket-thread"><div v-for="msg in selectedTicket.messages" :key="msg.id" class="ticket-msg" :class="msg.sender_type"><div class="msg-head"><b>{{ msg.sender_name }}</b><small>{{ formatDate(msg.created_at) }}</small></div><p>{{ msg.message }}</p></div></div>
        <form class="form-stack" style="border-top:1px solid var(--border);padding-top:12px" @submit.prevent="replyTicket"><label>Reply<textarea v-model.trim="ticketReply" placeholder="Type your message..."></textarea></label><button class="btn-primary" :disabled="busy||!ticketReply.trim()">Send</button></form>
      </div>
    </div>

    <!-- New Ticket Modal -->
    <div v-if="portalTab==='support'&&!selectedTicket" class="modal-backdrop" style="display:none"><!-- placeholder for future --></div>
  </div>
</template>
