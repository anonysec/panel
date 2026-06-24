<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { usePortalAuthStore } from '@/stores/auth'
import { useUsageStore } from '@/stores/usage'
import { usePortalTicketsStore } from '@/stores/tickets'
import { useUsageDisplay, formatBytes } from '@/composables/useUsageDisplay'
import { useEdition } from '@/composables/useEdition'
import { useApi } from '@koris/composables/useApi'
import { useClipboard } from '@koris/composables/useClipboard'
import { useI18n } from '@koris/composables/useI18n'
import { useWireGuardPortal } from '@/composables/useWireGuardPortal'
import KButton from '@koris/ui/KButton.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KTextarea from '@koris/ui/KTextarea.vue'
import KInput from '@koris/ui/KInput.vue'

// ---- Composables ----
const auth = usePortalAuthStore()
const usageStore = useUsageStore()
const ticketsStore = usePortalTicketsStore()
const { get, loading: profilesLoading } = useApi()
const { copy, copied } = useClipboard()
const { t, locale: currentLang } = useI18n()
const { peers, loading: wgLoading, fetchMyPeers, downloadConfig, getQRCodeUrl } = useWireGuardPortal()
const { isFull } = useEdition()

// ---- VPN Profiles ----
interface VpnProfile {
  type: string
  name: string
  filename: string
  available: boolean
  remote: string
  port: number
  protocol: string
  node: string
  download: string
  description?: string
}

interface ProfilesResponse {
  ok: boolean
  profiles: VpnProfile[]
}

const profiles = ref<VpnProfile[]>([])
const subUrl = ref('')
const preferredNodeId = ref(0)
const availableNodes = ref<{ id: number; name: string }[]>([])

// ---- App Links ----
interface AppLink {
  name: string
  url: string
  platform: string
  icon: string
}
const appLinks = ref<AppLink[]>([])

const displayAppLinks = computed(() => appLinks.value)

// Check if Cisco IPSec is available from profiles data
const ciscoAvailable = computed(() => profiles.value.some(p => p.type === 'cisco-ipsec' && p.available))

// ---- Telegram Proxies ----
interface TelegramProxy {
  id: number
  port: number
  status: string
  share_link: string
  tg_link: string
}
const telegramProxies = ref<TelegramProxy[]>([])
const teleProxiesLoading = ref(false)
const copiedProxyId = ref<number | null>(null)

async function fetchTelegramProxies() {
  teleProxiesLoading.value = true
  try {
    const res = await get<{ ok: boolean; proxies: TelegramProxy[] }>('/api/customer/telegram-proxies')
    telegramProxies.value = res.proxies || []
  } catch {
    // keep empty state
  } finally {
    teleProxiesLoading.value = false
  }
}

function copyProxyLink(proxy: TelegramProxy) {
  copy(proxy.tg_link)
  copiedProxyId.value = proxy.id
  setTimeout(() => {
    if (copiedProxyId.value === proxy.id) {
      copiedProxyId.value = null
    }
  }, 2000)
}

// ---- Support ----
const showCreateForm = ref(false)
const ticketForm = ref({ subject: '', category: 'general', body: '' })
const notice = ref('')
const supportTab = ref<'open' | 'closed'>('open')
const ratingValue = ref(0)
const ratingSubmitted = ref(false)

// ---- Computed ----
const displayName = computed(() => auth.displayName)
const planName = computed(() => auth.planName)
const status = computed(() => auth.status)
const usagePercent = computed(() => usageStore.usagePercent)
const maxDataBytes = computed(() => usageStore.maxDataBytes)
const totalUsageBytes = computed(() => usageStore.totalUsageBytes)
const expiresAt = computed(() => auth.user?.subscription?.expires_at ?? '')
const isOnline = computed(() => usageStore.isOnline)

const { progressColor, daysRemaining } = useUsageDisplay(
  totalUsageBytes,
  maxDataBytes,
  expiresAt
)

const progressBarColor = computed(() => {
  switch (progressColor.value) {
    case 'red': return 'var(--color-danger)'
    case 'amber': return 'var(--color-warning)'
    default: return 'var(--color-success, #22c55e)'
  }
})

const statusVariant = computed(() => {
  switch (status.value) {
    case 'active': return 'active'
    case 'expired': return 'expired'
    case 'disabled': return 'disabled'
    default: return 'expired'
  }
})

const formattedExpiry = computed(() => {
  if (!expiresAt.value) return t('portal.noExpiry')
  const localeMap: Record<string, string> = { en: 'en-US', fa: 'fa-IR', zh: 'zh-CN', ru: 'ru-RU' }
  const dtLocale = localeMap[currentLang.value] || 'en-US'
  return new Intl.DateTimeFormat(dtLocale, {
    year: 'numeric',
    month: 'short',
    day: '2-digit',
    timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone,
  }).format(new Date(expiresAt.value))
})


// ---- Lifecycle ----
onMounted(async () => {
  usageStore.loadUsage()
  ticketsStore.loadTickets()
  fetchMyPeers()
  fetchTelegramProxies()

  try {
    const res = await get<ProfilesResponse>('/api/portal/profiles')
    profiles.value = res.profiles || []
    if ((res as any).preferred_node_id) {
      preferredNodeId.value = (res as any).preferred_node_id
    }
  } catch {
    // keep empty state
  }

  try {
    const nodesRes = await get<{ ok: boolean; nodes: { id: number; name: string }[] }>('/api/portal/nodes')
    availableNodes.value = nodesRes.nodes || []
  } catch {
    // no nodes
  }

  try {
    const linksRes = await get<{ ok: boolean; links: AppLink[] }>('/api/portal/app-links')
    if (linksRes.links) appLinks.value = linksRes.links
  } catch {
    // no app links
  }

  if (auth.user?.sub_token) {
    subUrl.value = `${window.location.origin}/portal/sub/${auth.user.sub_token}`
  }
})

// ---- Methods ----
function handleCopySubUrl() {
  if (subUrl.value) {
    copy(subUrl.value)
  }
}

function getProfileIcon(type: string): string {
  switch (type) {
    case 'openvpn-udp': return '⚡'
    case 'openvpn-tcp': return '🔐'
    case 'openvpn': return '🔐'
    case 'l2tp': return '🔒'
    case 'ikev2': return '🛡️'
    case 'cisco-ipsec': return '🔑'
    default: return '📄'
  }
}

async function handleNodeChange(event: Event) {
  const nodeId = Number((event.target as HTMLSelectElement).value)
  preferredNodeId.value = nodeId
  try {
    const { post: postApi } = useApi()
    await postApi('/api/portal/preferred-node', { node_id: nodeId })
    // Reload profiles to reflect new preferred node in TCP config
    const res = await get<ProfilesResponse>('/api/portal/profiles')
    profiles.value = res.profiles || []
  } catch {
    // ignore
  }
}

async function handleCreateTicket() {
  if (!ticketForm.value.subject || !ticketForm.value.body) return
  notice.value = ''
  const id = await ticketsStore.createTicket({
    subject: ticketForm.value.subject,
    category: ticketForm.value.category,
    priority: 'medium',
    body: ticketForm.value.body,
  })
  if (id) {
    notice.value = t('portal.support.ticketCreated')
    ticketForm.value = { subject: '', category: 'general', body: '' }
    showCreateForm.value = false
  }
}

async function handleViewTicket(id: number) {
  ratingValue.value = 0
  ratingSubmitted.value = false
  await ticketsStore.loadTicketDetail(id)
}

function hasUnreadReply(ticket: any): boolean {
  if (ticket.last_reply_by) {
    return ticket.status === 'open' && ticket.last_reply_by === 'admin'
  }
  return false
}

const replyMessage = ref('')

function handleBackToList() {
  ticketsStore.clearDetail()
  ratingValue.value = 0
  ratingSubmitted.value = false
}

async function handleReply() {
  if (!replyMessage.value.trim() || !ticketsStore.detail) return
  const success = await ticketsStore.replyToTicket(ticketsStore.detail.id, replyMessage.value)
  if (success) {
    replyMessage.value = ''
    notice.value = t('portal.support.replySent')
  }
}

async function handleRate() {
  if (!ticketsStore.detail || ratingValue.value < 1 || ratingValue.value > 5) return
  const success = await ticketsStore.rateTicket(ticketsStore.detail.id, ratingValue.value)
  if (success) {
    ratingSubmitted.value = true
    notice.value = t('portal.support.ratingSubmitted')
  }
}
</script>
<template>
  <div class="sp">
    <!-- Welcome -->
    <div class="sp__welcome">
      <h1 class="sp__hello">{{ t('portal.hello') }}, {{ displayName }} 👋</h1>
      <p class="sp__subtitle">{{ t('portal.welcome') }}</p>
    </div>

    <!-- ===== Section: Account Status ===== -->
    <section class="sp__section">
      <h2 class="sp__section-title">
        <svg viewBox="0 0 20 20" fill="currentColor" width="20" height="20"><path d="M10 9a3 3 0 100-6 3 3 0 000 6zm-7 9a7 7 0 1114 0H3z"/></svg>
        {{ t('portal.account.title') }}
      </h2>
      <div class="sp__account-grid">
        <div class="sp__account-item">
          <span class="sp__account-label">{{ t('portal.account.plan') }}</span>
          <span class="sp__account-value">{{ planName }}</span>
        </div>
        <div class="sp__account-item">
          <span class="sp__account-label">{{ t('portal.account.status') }}</span>
          <KStatusPill :status="statusVariant">{{ t(`portal.account.status_${status}`) }}</KStatusPill>
        </div>
        <div class="sp__account-item">
          <span class="sp__account-label">{{ t('portal.account.expires') }}</span>
          <span class="sp__account-value">{{ formattedExpiry }}</span>
        </div>
        <div class="sp__account-item">
          <span class="sp__account-label">{{ t('portal.account.daysLeft') }}</span>
          <span class="sp__account-value" :class="{ 'sp__account-value--warn': daysRemaining <= 7 }">
            {{ daysRemaining }} {{ t('portal.account.days') }}
          </span>
        </div>
        <div class="sp__account-item">
          <span class="sp__account-label">{{ t('portal.account.connection') }}</span>
          <KStatusPill :status="isOnline ? 'active' : 'disabled'">
            {{ isOnline ? t('portal.account.online') : t('portal.account.offline') }}
          </KStatusPill>
        </div>
      </div>
    </section>

    <!-- ===== Section: Usage ===== -->
    <section class="sp__section">
      <h2 class="sp__section-title">
        <svg viewBox="0 0 20 20" fill="currentColor" width="20" height="20"><path fill-rule="evenodd" d="M3 3a1 1 0 000 2v8a2 2 0 002 2h2.586l-1.293 1.293a1 1 0 101.414 1.414L10 15.414l2.293 2.293a1 1 0 001.414-1.414L12.414 15H15a2 2 0 002-2V5a1 1 0 100-2H3zm11.707 4.707a1 1 0 00-1.414-1.414L10 9.586 8.707 8.293a1 1 0 00-1.414 0l-2 2a1 1 0 101.414 1.414L8 10.414l1.293 1.293a1 1 0 001.414 0l4-4z" clip-rule="evenodd"/></svg>
        {{ t('portal.usage.title') }}
      </h2>

      <KSkeleton v-if="usageStore.loading && !usageStore.usage" type="card" :count="1" />
      <template v-else>
        <div class="sp__usage-compact">
          <div class="sp__usage-compact-header">
            <span class="sp__usage-compact-percent">{{ usagePercent }}% used</span>
            <span class="sp__usage-compact-values">{{ formatBytes(totalUsageBytes) }} / {{ maxDataBytes ? formatBytes(maxDataBytes) : 'Unlimited' }}</span>
          </div>
          <div class="sp__progress-bar">
            <div class="sp__progress-bar-fill" :style="{ width: `${Math.min(100, usagePercent)}%`, backgroundColor: progressBarColor }"></div>
          </div>
          <div class="sp__usage-compact-footer">
            <span>{{ daysRemaining }} days remaining</span>
            <span>Expires {{ formattedExpiry }}</span>
          </div>
        </div>
      </template>
    </section>

    <!-- ===== Section: VPN Profiles ===== -->
    <section class="sp__section">
      <h2 class="sp__section-title">
        <svg viewBox="0 0 20 20" fill="currentColor" width="20" height="20"><path fill-rule="evenodd" d="M18 8a6 6 0 01-7.743 5.743L10 14l-1 1-1 1H6v2H2v-4l4.257-4.257A6 6 0 1118 8zm-6-4a1 1 0 100 2 2 2 0 012 2 1 1 0 102 0 4 4 0 00-4-4z" clip-rule="evenodd"/></svg>
        {{ t('portal.vpn.title') }}
      </h2>

      <KSkeleton v-if="profilesLoading && !profiles.length" type="card" :count="2" />
      <template v-else>
        <!-- Subscription URL -->
        <div v-if="subUrl" class="sp__sub-url">
          <label class="sp__sub-url-label">{{ t('portal.vpn.subUrl') }}</label>
          <p class="sp__sub-url-desc">{{ t('portal.vpn.subUrlDesc') }}</p>
          <div class="sp__sub-url-row">
            <input type="text" :value="subUrl" class="sp__sub-url-input" readonly />
            <KButton variant="primary" size="sm" @click="handleCopySubUrl">
              {{ copied ? t('portal.vpn.copied') : t('portal.vpn.copy') }}
            </KButton>
          </div>
        </div>

        <!-- Node selector -->
        <div class="sp__node-selector">
          <label class="sp__node-label">{{ t('portal.vpn.preferredNode') }}</label>
          <select class="sp__node-select" :value="preferredNodeId" @change="handleNodeChange($event)">
            <option value="0">{{ t('portal.vpn.autoRandom') }}</option>
            <option v-for="node in availableNodes" :key="node.id" :value="node.id">
              {{ node.name }}
            </option>
          </select>
        </div>

        <!-- Profile list -->
        <KEmptyState
          v-if="!profiles.length"
          :title="t('portal.vpn.noProfiles')"
          :description="t('portal.vpn.noProfilesDesc')"
          icon="📡"
        />

        <div v-else class="sp__profiles-list">
          <div v-for="profile in profiles" :key="profile.type" class="sp__profile-card">
            <div class="sp__profile-icon">{{ getProfileIcon(profile.type) }}</div>
            <div class="sp__profile-info">
              <div class="sp__profile-name">{{ profile.name }}</div>
              <div class="sp__profile-meta">
                <span v-if="profile.description" class="sp__profile-desc">{{ profile.description }}</span>
                <span v-else>{{ profile.node }}</span>
              </div>
            </div>
            <a
              v-if="profile.available"
              :href="profile.download"
              download
              class="sp__profile-dl"
            >
              <KButton variant="primary" size="sm">{{ t('portal.vpn.download') }}</KButton>
            </a>
            <KButton v-else variant="ghost" size="sm" :disabled="true">{{ t('portal.vpn.unavailable') }}</KButton>
          </div>
          <!-- WireGuard peers inline with other profiles -->
          <div v-for="peer in peers" :key="'wg-' + peer.id" class="sp__profile-card">
            <div class="sp__profile-icon">🛡️</div>
            <div class="sp__profile-info">
              <div class="sp__profile-name">WireGuard — {{ peer.node_name || 'Server' }}</div>
              <div class="sp__profile-meta">{{ peer.allowed_ips }}</div>
            </div>
            <KButton variant="primary" size="sm" @click="downloadConfig(peer.id)">
              {{ t('portal.vpn.download') }}
            </KButton>
          </div>
        </div>

        <!-- Cisco IPSec Setup Instructions -->
        <div v-if="ciscoAvailable" class="sp__cisco-instructions">
          <h3 class="sp__cisco-instructions-title">{{ t('portal.cisco.setupTitle') }}</h3>
          <ul class="sp__cisco-instructions-list">
            <li>{{ t('portal.cisco.setupIOS') }}</li>
            <li>{{ t('portal.cisco.setupAndroid') }}</li>
            <li><strong>{{ t('portal.cisco.server') }}:</strong> {{ t('portal.cisco.serverNote') }}</li>
            <li><strong>{{ t('portal.cisco.username') }}:</strong> {{ auth.username }}</li>
            <li><strong>{{ t('portal.cisco.psk') }}:</strong> {{ t('portal.cisco.pskNote') }}</li>
          </ul>
        </div>
      </template>
    </section>

    <!-- ===== Section: Telegram Proxies ===== -->
    <section v-if="isFull && (telegramProxies.length || teleProxiesLoading)" class="sp__section">
      <h2 class="sp__section-title">
        <svg viewBox="0 0 20 20" fill="currentColor" width="20" height="20"><path d="M17.05 2.65a1 1 0 00-1.42-.08L2.46 14.23a1 1 0 00-.15 1.3l1.73 2.6a1 1 0 001.5.23l13.2-11.42a1 1 0 00.1-1.42l-1.79-2.87zM4.5 17.5l-.87-1.3L15.5 5.5l.87 1.3L4.5 17.5z"/></svg>
        {{ t('portal.teleproxy.title') }}
      </h2>

      <KSkeleton v-if="teleProxiesLoading && !telegramProxies.length" type="card" :count="1" />

      <KEmptyState
        v-else-if="!telegramProxies.length"
        :title="t('portal.teleproxy.noProxies')"
        :description="t('portal.teleproxy.noProxiesDesc')"
        icon="📡"
      />

      <div v-else class="sp__teleproxy-list">
        <div v-for="proxy in telegramProxies" :key="proxy.id" class="sp__teleproxy-card">
          <div class="sp__teleproxy-status">
            <span class="sp__teleproxy-dot" :class="{ 'sp__teleproxy-dot--active': proxy.status === 'active' }"></span>
          </div>
          <div class="sp__teleproxy-info">
            <div class="sp__teleproxy-name">
              {{ t('portal.teleproxy.port') }}: {{ proxy.port }}
            </div>
            <div class="sp__teleproxy-meta">
              <KStatusPill :status="proxy.status === 'active' ? 'active' : 'disabled'" size="sm">
                {{ t(`portal.teleproxy.status_${proxy.status === 'active' ? 'active' : 'stopped'}`) }}
              </KStatusPill>
            </div>
          </div>
          <KButton variant="primary" size="sm" @click="copyProxyLink(proxy)">
            {{ copiedProxyId === proxy.id ? t('portal.teleproxy.copied') : t('portal.teleproxy.copyLink') }}
          </KButton>
        </div>
      </div>
    </section>

    <!-- ===== Section: Download Apps ===== -->
    <section v-if="displayAppLinks.length > 0" class="sp__section">
      <h2 class="sp__section-title">
        <svg viewBox="0 0 20 20" fill="currentColor" width="20" height="20"><path fill-rule="evenodd" d="M3 17a1 1 0 011-1h12a1 1 0 110 2H4a1 1 0 01-1-1zm3.293-7.707a1 1 0 011.414 0L9 10.586V3a1 1 0 112 0v7.586l1.293-1.293a1 1 0 111.414 1.414l-3 3a1 1 0 01-1.414 0l-3-3a1 1 0 010-1.414z" clip-rule="evenodd"/></svg>
        {{ t('portal.apps.title') }}
      </h2>
      <p class="sp__apps-desc">{{ t('portal.apps.desc') }}</p>
      <div class="sp__apps-grid">
        <a
          v-for="link in displayAppLinks"
          :key="link.platform"
          :href="link.url"
          target="_blank"
          rel="noopener noreferrer"
          class="sp__app-card"
        >
          <span class="sp__app-icon">{{ link.icon }}</span>
          <span class="sp__app-name">{{ link.name }}</span>
        </a>
      </div>
    </section>

    <!-- ===== Section: Support ===== -->
    <section v-if="isFull" class="sp__section">
      <h2 class="sp__section-title">
        <svg viewBox="0 0 20 20" fill="currentColor" width="20" height="20"><path fill-rule="evenodd" d="M18 10c0 3.866-3.582 7-8 7a8.841 8.841 0 01-4.083-.98L2 17l1.338-3.123C2.493 12.767 2 11.434 2 10c0-3.866 3.582-7 8-7s8 3.134 8 7zM7 9H5v2h2V9zm8 0h-2v2h2V9zM9 9h2v2H9V9z" clip-rule="evenodd"/></svg>
        {{ t('portal.support.title') }}
      </h2>

      <div v-if="notice" class="sp__notice" role="status">{{ notice }}</div>

      <KSkeleton v-if="ticketsStore.loading && !ticketsStore.list.length" type="card" :count="1" />

      <template v-else>
        <!-- Ticket Detail (shown when a ticket is selected) -->
        <div v-if="ticketsStore.detail" class="sp__ticket-detail">
          <button class="sp__back-btn" @click="handleBackToList">← {{ t('portal.support.backToList') }}</button>
          <div class="sp__ticket-header">
            <span class="sp__ticket-subject">#{{ ticketsStore.detail.id }}: {{ ticketsStore.detail.subject }}</span>
            <KStatusPill :status="ticketsStore.detail.status === 'open' || ticketsStore.detail.status === 'in_progress' ? 'active' : 'disabled'" size="sm">
              {{ ticketsStore.detail.status }}
            </KStatusPill>
          </div>

          <!-- Messages thread -->
          <div class="sp__messages">
            <div
              v-for="msg in (ticketsStore.detail.messages || []).filter((m: any) => !m.is_internal)"
              :key="msg.id"
              class="sp__message"
              :class="{ 'sp__message--admin': msg.sender_type === 'admin' }"
            >
              <div class="sp__message-header">
                <span class="sp__message-sender">{{ msg.sender_name }}</span>
                <span class="sp__message-time">{{ msg.created_at }}</span>
              </div>
              <p class="sp__message-body">{{ msg.body }}</p>
            </div>
          </div>

          <!-- Reply form (only for open/in_progress/waiting tickets) -->
          <form
            v-if="ticketsStore.detail.status === 'open' || ticketsStore.detail.status === 'in_progress' || ticketsStore.detail.status === 'waiting'"
            class="sp__reply-form"
            @submit.prevent="handleReply"
          >
            <KFormField :label="t('portal.support.yourReply')">
              <KTextarea v-model="replyMessage" :placeholder="t('portal.support.replyPlaceholder')" :rows="3" />
            </KFormField>
            <KButton type="submit" variant="primary" size="sm" :loading="ticketsStore.loading" :disabled="!replyMessage.trim()">
              {{ t('portal.support.send') }}
            </KButton>
          </form>

          <!-- Satisfaction Survey (for resolved/closed tickets without rating) -->
          <div
            v-if="(ticketsStore.detail.status === 'resolved' || ticketsStore.detail.status === 'closed') && !ticketsStore.detail.satisfaction_rating && !ratingSubmitted"
            class="sp__rating"
          >
            <p class="sp__rating-title">{{ t('portal.support.ratingTitle') }}</p>
            <p class="sp__rating-desc">{{ t('portal.support.ratingDesc') }}</p>
            <div class="sp__stars">
              <button
                v-for="star in 5"
                :key="star"
                type="button"
                class="sp__star"
                :class="{ 'sp__star--active': star <= ratingValue }"
                :aria-label="`${star} star${star > 1 ? 's' : ''}`"
                @click="ratingValue = star"
              >
                ★
              </button>
            </div>
            <KButton variant="primary" size="sm" :disabled="ratingValue < 1" :loading="ticketsStore.loading" @click="handleRate">
              {{ t('portal.support.submitRating') }}
            </KButton>
          </div>

          <!-- Already rated -->
          <div v-if="ticketsStore.detail.satisfaction_rating" class="sp__rated">
            <span class="sp__rated-label">{{ t('portal.support.yourRating') }}:</span>
            <span class="sp__rated-stars">
              <span v-for="star in 5" :key="star" :class="{ 'sp__star--active': star <= (ticketsStore.detail.satisfaction_rating || 0) }">★</span>
            </span>
          </div>
        </div>

        <!-- Ticket List (when no ticket is selected) -->
        <div v-else class="sp__support-simple">
          <p class="sp__support-desc">{{ t('portal.support.noTicketsDesc') }}</p>
          <div class="sp__support-actions">
            <KButton variant="primary" size="sm" @click="showCreateForm = true">
              + {{ t('portal.support.newTicket') }}
            </KButton>
            <span v-if="ticketsStore.openTickets.length" class="sp__support-count">
              {{ ticketsStore.openTickets.length }} open ticket{{ ticketsStore.openTickets.length > 1 ? 's' : '' }}
            </span>
          </div>

          <!-- Ticket tabs -->
          <div v-if="ticketsStore.list.length" class="sp__ticket-tabs">
            <button :class="['sp__ticket-tab', { 'sp__ticket-tab--active': supportTab === 'open' }]" @click="supportTab = 'open'">
              {{ t('portal.support.open') }} ({{ ticketsStore.openTickets.length }})
            </button>
            <button :class="['sp__ticket-tab', { 'sp__ticket-tab--active': supportTab === 'closed' }]" @click="supportTab = 'closed'">
              {{ t('portal.support.closed') }} ({{ ticketsStore.closedTickets.length }})
            </button>
          </div>

          <!-- Filtered ticket list -->
          <div v-if="ticketsStore.list.length" class="sp__tickets-list">
            <div v-for="ticket in (supportTab === 'open' ? ticketsStore.openTickets : ticketsStore.closedTickets)" :key="ticket.id" class="sp__ticket-row" @click="handleViewTicket(ticket.id)">
              <div class="sp__ticket-row-info">
                <span class="sp__ticket-row-subject">{{ ticket.subject }}</span>
                <KStatusPill :status="ticket.status === 'open' || ticket.status === 'in_progress' ? 'active' : 'disabled'" size="sm">
                  {{ ticket.status }}
                </KStatusPill>
                <span v-if="hasUnreadReply(ticket)" class="sp__ticket-unread">●</span>
              </div>
            </div>
          </div>
        </div>

        <!-- Create form (hidden by default) -->
        <form v-if="showCreateForm && !ticketsStore.detail" class="sp__ticket-form" @submit.prevent="handleCreateTicket">
          <KFormField :label="t('portal.support.subject')" :required="true">
            <KInput v-model="ticketForm.subject" :placeholder="t('portal.support.subjectPlaceholder')" />
          </KFormField>
          <div class="sp__ticket-form-row">
            <select v-model="ticketForm.category" class="sp__category-select">
              <option value="general">{{ t('portal.support.categoryGeneral') }}</option>
              <option value="technical">{{ t('portal.support.categoryTechnical') }}</option>
              <option value="billing">{{ t('portal.support.categoryBilling') }}</option>
            </select>
          </div>
          <KFormField :label="t('portal.support.message')" :required="true">
            <KTextarea v-model="ticketForm.body" :placeholder="t('portal.support.messagePlaceholder')" :rows="4" />
          </KFormField>
          <div class="sp__form-actions">
            <KButton variant="ghost" size="sm" @click="showCreateForm = false">{{ t('portal.support.cancel') }}</KButton>
            <KButton type="submit" variant="primary" size="sm" :loading="ticketsStore.loading" :disabled="!ticketForm.subject || !ticketForm.body">
              {{ t('portal.support.create') }}
            </KButton>
          </div>
        </form>
      </template>
    </section>
  </div>
</template>
<style scoped>
.sp {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
  padding-bottom: calc(var(--space-8) + env(safe-area-inset-bottom, 20px));
}

/* Welcome */
.sp__welcome {
  margin-bottom: var(--space-2);
}
.sp__hello {
  font-size: var(--text-xl);
  font-weight: 700;
}
.sp__subtitle {
  color: var(--color-muted);
  font-size: var(--text-sm);
  margin-top: var(--space-1);
}

/* Sections */
.sp__section {
  padding: var(--space-5);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
}
.sp__section-title {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--text-md);
  font-weight: 600;
  margin-bottom: var(--space-4);
  color: var(--color-text);
}
.sp__section-title svg {
  color: var(--color-primary);
  flex-shrink: 0;
}

/* Account grid */
.sp__account-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: var(--space-4);
}
.sp__account-item {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}
.sp__account-label {
  font-size: var(--text-xs);
  color: var(--color-muted);
  text-transform: uppercase;
  letter-spacing: 0.03em;
}
.sp__account-value {
  font-size: var(--text-sm);
  font-weight: 600;
}
.sp__account-value--warn {
  color: var(--color-danger);
}

/* Usage */
.sp__usage-compact {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.sp__usage-compact-header {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
}
.sp__usage-compact-percent {
  font-size: var(--text-lg);
  font-weight: 700;
}
.sp__usage-compact-values {
  font-size: var(--text-sm);
  color: var(--color-muted);
}
.sp__usage-compact-footer {
  display: flex;
  justify-content: space-between;
  font-size: var(--text-xs);
  color: var(--color-muted);
  margin-top: var(--space-1);
}
.sp__progress-bar {
  height: 8px;
  background: var(--color-border);
  border-radius: 4px;
  overflow: hidden;
}
.sp__progress-bar-fill {
  height: 100%;
  border-radius: 4px;
  transition: width 0.4s ease, background-color 0.3s ease;
}

/* VPN */
.sp__sub-url {
  margin-bottom: var(--space-4);
  padding-bottom: var(--space-4);
  border-bottom: 1px solid var(--color-border);
}
.sp__sub-url-label {
  font-size: var(--text-sm);
  font-weight: 600;
  display: block;
  margin-bottom: var(--space-1);
}
.sp__sub-url-desc {
  font-size: var(--text-xs);
  color: var(--color-muted);
  margin-bottom: var(--space-3);
}
.sp__sub-url-row {
  display: flex;
  gap: var(--space-2);
  align-items: center;
}
.sp__sub-url-input {
  flex: 1;
  padding: var(--space-2) var(--space-3);
  background: var(--color-bg);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  color: var(--color-text);
  font-size: var(--text-sm);
  font-family: monospace;
  min-width: 0;
}
.sp__node-selector {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  margin-bottom: var(--space-3);
}
.sp__node-label {
  font-size: var(--text-sm);
  color: var(--color-muted);
  white-space: nowrap;
}
.sp__node-select {
  flex: 1;
  max-width: 240px;
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  color: var(--color-text);
  font-size: var(--text-sm);
}
.sp__profile-desc {
  font-size: var(--text-xs);
  color: var(--color-muted);
}
.sp__profiles-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.sp__profile-card {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4);
  background: var(--color-bg);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  min-height: 56px;
}
.sp__profile-icon {
  font-size: 1.4rem;
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--color-surface-2);
  border-radius: var(--radius-md);
  flex-shrink: 0;
}
.sp__profile-info {
  flex: 1;
  min-width: 0;
}
.sp__profile-name {
  font-size: var(--text-sm);
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.sp__profile-meta {
  font-size: var(--text-xs);
  color: var(--color-muted);
}
.sp__profile-dl {
  text-decoration: none;
  flex-shrink: 0;
}

/* Cisco IPSec Instructions */
.sp__cisco-instructions {
  margin-top: var(--space-4);
  padding: var(--space-3) var(--space-4);
  background: var(--color-bg);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
}
.sp__cisco-instructions-title {
  font-size: var(--text-sm);
  font-weight: 600;
  margin-bottom: var(--space-2);
}
.sp__cisco-instructions-list {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  font-size: var(--text-sm);
  color: var(--color-muted);
}
.sp__cisco-instructions-list li {
  padding-inline-start: var(--space-3);
  position: relative;
}
.sp__cisco-instructions-list li::before {
  content: '•';
  position: absolute;
  inset-inline-start: 0;
  color: var(--color-primary);
}

/* Telegram Proxies */
.sp__teleproxy-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.sp__teleproxy-card {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4);
  background: var(--color-bg);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  min-height: 56px;
}
.sp__teleproxy-status {
  flex-shrink: 0;
}
.sp__teleproxy-dot {
  display: block;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: var(--color-muted);
}
.sp__teleproxy-dot--active {
  background: var(--color-success, #22c55e);
}
.sp__teleproxy-info {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}
.sp__teleproxy-name {
  font-size: var(--text-sm);
  font-weight: 600;
}
.sp__teleproxy-meta {
  font-size: var(--text-xs);
}

/* App Downloads */
.sp__apps-desc {
  font-size: var(--text-sm);
  color: var(--color-muted);
  margin-bottom: var(--space-4);
}
.sp__apps-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: var(--space-3);
}
.sp__app-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-4);
  background: var(--color-bg);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  text-decoration: none;
  color: var(--color-text);
  transition: border-color 0.15s, transform 0.15s;
  min-height: 80px;
}
.sp__app-card:hover {
  border-color: var(--color-primary);
  transform: translateY(-2px);
}
.sp__app-icon {
  font-size: 1.8rem;
}
.sp__app-name {
  font-size: var(--text-sm);
  font-weight: 500;
  text-align: center;
}

/* Support */
.sp__notice {
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  background: rgba(34, 197, 94, 0.1);
  color: var(--color-success);
  font-size: var(--text-sm);
  margin-bottom: var(--space-4);
  border: 1px solid rgba(34, 197, 94, 0.2);
}
.sp__support-simple {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.sp__support-desc {
  font-size: var(--text-sm);
  color: var(--color-muted);
  margin: 0;
}
.sp__support-actions {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}
.sp__support-count {
  font-size: var(--text-sm);
  color: var(--color-muted);
}
.sp__ticket-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  margin-top: var(--space-3);
}
.sp__form-actions {
  display: flex;
  gap: var(--space-2);
  justify-content: flex-end;
}
.sp__tickets-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.sp__ticket-row {
  display: flex;
  align-items: center;
  padding: var(--space-3) var(--space-4);
  background: var(--color-bg);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  cursor: pointer;
  min-height: 48px;
  transition: background 0.15s;
}
.sp__ticket-row:hover {
  background: var(--color-surface-2);
}
.sp__ticket-row-info {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  flex: 1;
  min-width: 0;
}
.sp__ticket-row-subject {
  font-size: var(--text-sm);
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.sp__ticket-detail {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.sp__back-btn {
  display: inline-flex;
  align-items: center;
  background: none;
  border: none;
  color: var(--color-primary);
  font-size: var(--text-sm);
  cursor: pointer;
  padding: var(--space-1) 0;
  min-height: 44px;
}
.sp__ticket-header {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  flex-wrap: wrap;
}
.sp__ticket-subject {
  font-size: var(--text-md);
  font-weight: 600;
}
.sp__messages {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.sp__message {
  padding: var(--space-3) var(--space-4);
  background: var(--color-bg);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
}
.sp__message--admin {
  background: rgba(99, 102, 241, 0.05);
  border-color: rgba(99, 102, 241, 0.2);
}
.sp__message-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--space-2);
}
.sp__message-sender {
  font-size: var(--text-sm);
  font-weight: 600;
}
.sp__message-time {
  font-size: var(--text-xs);
  color: var(--color-muted);
}
.sp__message-body {
  font-size: var(--text-sm);
  line-height: 1.5;
  white-space: pre-wrap;
}
.sp__reply-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  margin-top: var(--space-3);
  padding-top: var(--space-3);
  border-top: 1px solid var(--color-border);
}
.sp__ticket-tabs { display: flex; gap: var(--space-2); margin-bottom: var(--space-3); margin-top: var(--space-3); }
.sp__ticket-tab { padding: 6px 14px; font-size: var(--text-xs); font-weight: 500; border: 1px solid var(--color-border); border-radius: var(--radius-md); background: transparent; color: var(--color-muted); cursor: pointer; min-height: 36px; }
.sp__ticket-tab--active { background: var(--color-primary); color: #fff; border-color: var(--color-primary); }
.sp__ticket-unread { color: var(--color-primary); font-size: 10px; margin-left: auto; }

/* Category select */
.sp__ticket-form-row {
  display: flex;
  gap: var(--space-3);
}
.sp__category-select {
  flex: 1;
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  color: var(--color-text);
  font-size: var(--text-sm);
}

/* Satisfaction Rating */
.sp__rating {
  margin-top: var(--space-4);
  padding-top: var(--space-4);
  border-top: 1px solid var(--color-border);
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  align-items: flex-start;
}
.sp__rating-title {
  font-size: var(--text-sm);
  font-weight: 600;
  margin: 0;
}
.sp__rating-desc {
  font-size: var(--text-sm);
  color: var(--color-muted);
  margin: 0;
}
.sp__stars {
  display: flex;
  gap: var(--space-1);
}
.sp__star {
  background: none;
  border: none;
  font-size: 1.8rem;
  cursor: pointer;
  color: var(--color-border);
  transition: color 0.15s, transform 0.1s;
  padding: 0;
  line-height: 1;
}
.sp__star:hover,
.sp__star--active {
  color: #f59e0b;
  transform: scale(1.1);
}
.sp__rated {
  margin-top: var(--space-4);
  padding-top: var(--space-4);
  border-top: 1px solid var(--color-border);
  display: flex;
  align-items: center;
  gap: var(--space-2);
}
.sp__rated-label {
  font-size: var(--text-sm);
  color: var(--color-muted);
}
.sp__rated-stars {
  display: flex;
  gap: 2px;
  font-size: 1.2rem;
  color: var(--color-border);
}
.sp__rated-stars .sp__star--active {
  color: #f59e0b;
}

/* ===== Mobile responsive ===== */
@media (max-width: 768px) {
  .sp {
    gap: var(--space-4);
  }

  .sp__section {
    padding: var(--space-4);
  }

  .sp__hello {
    font-size: var(--text-lg);
  }

  .sp__account-grid {
    grid-template-columns: 1fr 1fr;
  }

  .sp__sub-url-row {
    flex-direction: column;
    align-items: stretch;
  }

  .sp__sub-url-input {
    width: 100%;
  }

  .sp__node-selector {
    flex-direction: column;
    align-items: stretch;
    gap: var(--space-2);
  }

  .sp__node-select {
    max-width: 100%;
  }

  .sp__profile-card {
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .sp__profile-info {
    min-width: calc(100% - 60px);
  }

  .sp__profile-dl,
  .sp__profile-card > :last-child {
    width: 100%;
  }

  .sp__apps-grid {
    grid-template-columns: repeat(2, 1fr);
  }

  .sp__form-actions {
    flex-direction: column;
  }

  .sp__form-actions > * {
    width: 100%;
  }

  .sp__ticket-header {
    flex-direction: column;
    align-items: flex-start;
    gap: var(--space-2);
  }
}

@media (max-width: 400px) {
  .sp__account-grid {
    grid-template-columns: 1fr;
  }

  .sp__apps-grid {
    grid-template-columns: 1fr 1fr;
  }
}

</style>
