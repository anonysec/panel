<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { usePortalAuthStore } from '@/stores/auth'
import { useUsageStore } from '@/stores/usage'
import { usePortalTicketsStore } from '@/stores/tickets'
import { useUsageDisplay, formatBytes } from '@/composables/useUsageDisplay'
import { useApi } from '@koris/composables/useApi'
import { useClipboard } from '@koris/composables/useClipboard'
import { useI18n } from '@koris/composables/useI18n'
import KButton from '@koris/ui/KButton.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KTextarea from '@koris/ui/KTextarea.vue'
import KInput from '@koris/ui/KInput.vue'
import UsageGauge from '@/components/UsageGauge.vue'
import TicketThread from '@/components/TicketThread.vue'

// ---- Composables ----
const auth = usePortalAuthStore()
const usageStore = useUsageStore()
const ticketsStore = usePortalTicketsStore()
const { get, loading: profilesLoading } = useApi()
const { copy, copied } = useClipboard()
const { t, locale: currentLang } = useI18n()

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
}

interface ProfilesResponse {
  ok: boolean
  profiles: VpnProfile[]
}

const profiles = ref<VpnProfile[]>([])
const subUrl = ref('')

// ---- App Links ----
interface AppLink {
  name: string
  url: string
  platform: string
  icon: string
}
const appLinks = ref<AppLink[]>([])

// ---- Support ----
const showCreateForm = ref(false)
const ticketForm = ref({ subject: '', message: '' })
const replyMessage = ref('')
const notice = ref('')
const selectedTicketId = ref<number | null>(null)

// ---- Computed ----
const displayName = computed(() => auth.displayName)
const planName = computed(() => auth.planName)
const status = computed(() => auth.status)
const usagePercent = computed(() => usageStore.usagePercent)
const maxDataBytes = computed(() => usageStore.maxDataBytes)
const totalUsageBytes = computed(() => usageStore.totalUsageBytes)
const expiresAt = computed(() => auth.user?.subscription?.expires_at ?? '')
const isOnline = computed(() => usageStore.isOnline)

const { remainingBytes, progressColor, daysRemaining } = useUsageDisplay(
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

const remainingDisplay = computed(() => {
  if (!maxDataBytes.value) return t('portal.unlimitedData')
  return `${formatBytes(remainingBytes.value)} / ${formatBytes(maxDataBytes.value)}`
})

const selectedTicket = computed(() => ticketsStore.detail)

// ---- Lifecycle ----
onMounted(async () => {
  usageStore.loadUsage()
  ticketsStore.loadTickets()

  try {
    const res = await get<ProfilesResponse>('/api/portal/profiles')
    profiles.value = res.profiles || []
  } catch {
    // keep empty state
  }

  try {
    const linksRes = await get<{ ok: boolean; links: AppLink[] }>('/api/portal/app-links')
    if (linksRes.links) appLinks.value = linksRes.links
  } catch {
    // no app links
  }

  if (auth.user?.sub_token) {
    subUrl.value = `${window.location.origin}/sub/${auth.user.sub_token}`
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
    case 'openvpn': return '🔐'
    case 'l2tp': return '🔒'
    case 'ikev2': return '🛡️'
    default: return '📄'
  }
}

async function handleCreateTicket() {
  if (!ticketForm.value.subject || !ticketForm.value.message) return
  notice.value = ''
  const id = await ticketsStore.createTicket({
    subject: ticketForm.value.subject,
    priority: 'normal',
    message: ticketForm.value.message,
  })
  if (id) {
    notice.value = t('portal.support.ticketCreated')
    ticketForm.value = { subject: '', message: '' }
    showCreateForm.value = false
    await ticketsStore.loadTicketDetail(id)
    selectedTicketId.value = id
  }
}

async function handleViewTicket(id: number) {
  selectedTicketId.value = id
  await ticketsStore.loadTicketDetail(id)
}

async function handleReply() {
  if (!selectedTicket.value || !replyMessage.value.trim()) return
  notice.value = ''
  const success = await ticketsStore.replyToTicket(selectedTicket.value.id, replyMessage.value)
  if (success) {
    notice.value = t('portal.support.replySent')
    replyMessage.value = ''
  }
}

function handleBackToList() {
  selectedTicketId.value = null
  ticketsStore.clearDetail()
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
        <div class="sp__usage-content">
          <UsageGauge :percent="usagePercent" :size="140" />
          <div class="sp__usage-info">
            <div class="sp__usage-remaining">{{ remainingDisplay }}</div>
            <div class="sp__usage-label">{{ t('portal.usage.remaining') }}</div>
            <div class="sp__progress-bar">
              <div class="sp__progress-bar-fill" :style="{ width: `${Math.min(100, usagePercent)}%`, backgroundColor: progressBarColor }"></div>
            </div>
            <div class="sp__progress-labels">
              <span>{{ formatBytes(totalUsageBytes) }} {{ t('portal.usage.used') }}</span>
              <span>{{ maxDataBytes ? formatBytes(maxDataBytes) : t('portal.unlimitedData') }}</span>
            </div>
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
              <div class="sp__profile-meta">{{ profile.node }}</div>
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
        </div>
      </template>
    </section>

    <!-- ===== Section: Download Apps ===== -->
    <section v-if="appLinks.length" class="sp__section">
      <h2 class="sp__section-title">
        <svg viewBox="0 0 20 20" fill="currentColor" width="20" height="20"><path fill-rule="evenodd" d="M3 17a1 1 0 011-1h12a1 1 0 110 2H4a1 1 0 01-1-1zm3.293-7.707a1 1 0 011.414 0L9 10.586V3a1 1 0 112 0v7.586l1.293-1.293a1 1 0 111.414 1.414l-3 3a1 1 0 01-1.414 0l-3-3a1 1 0 010-1.414z" clip-rule="evenodd"/></svg>
        {{ t('portal.apps.title') }}
      </h2>
      <p class="sp__apps-desc">{{ t('portal.apps.desc') }}</p>
      <div class="sp__apps-grid">
        <a
          v-for="link in appLinks"
          :key="link.url"
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
    <section class="sp__section">
      <h2 class="sp__section-title">
        <svg viewBox="0 0 20 20" fill="currentColor" width="20" height="20"><path fill-rule="evenodd" d="M18 10c0 3.866-3.582 7-8 7a8.841 8.841 0 01-4.083-.98L2 17l1.338-3.123C2.493 12.767 2 11.434 2 10c0-3.866 3.582-7 8-7s8 3.134 8 7zM7 9H5v2h2V9zm8 0h-2v2h2V9zM9 9h2v2H9V9z" clip-rule="evenodd"/></svg>
        {{ t('portal.support.title') }}
      </h2>

      <div v-if="notice" class="sp__notice" role="status">{{ notice }}</div>

      <KSkeleton v-if="ticketsStore.loading && !ticketsStore.list.length && !selectedTicket" type="card" :count="1" />

      <!-- Ticket detail -->
      <template v-else-if="selectedTicket && selectedTicketId">
        <div class="sp__ticket-detail">
          <button class="sp__back-btn" @click="handleBackToList">
            &larr; {{ t('portal.support.backToList') }}
          </button>
          <div class="sp__ticket-header">
            <h3 class="sp__ticket-subject">#{{ selectedTicket.id }}: {{ selectedTicket.subject }}</h3>
            <KStatusPill :status="selectedTicket.status === 'open' ? 'active' : 'disabled'">
              {{ selectedTicket.status === 'open' ? t('portal.support.open') : t('portal.support.closed') }}
            </KStatusPill>
          </div>
          <TicketThread :messages="selectedTicket.messages" />
          <form v-if="selectedTicket.status === 'open'" class="sp__reply-form" @submit.prevent="handleReply">
            <KFormField :label="t('portal.support.yourReply')">
              <KTextarea v-model="replyMessage" :placeholder="t('portal.support.replyPlaceholder')" :rows="3" />
            </KFormField>
            <KButton type="submit" variant="primary" :loading="ticketsStore.loading" :disabled="!replyMessage.trim()">
              {{ t('portal.support.send') }}
            </KButton>
          </form>
        </div>
      </template>

      <!-- Create form + list -->
      <template v-else>
        <!-- New ticket form -->
        <div class="sp__new-ticket">
          <button v-if="!showCreateForm" class="sp__new-ticket-btn" @click="showCreateForm = true">
            + {{ t('portal.support.newTicket') }}
          </button>
          <form v-if="showCreateForm" class="sp__ticket-form" @submit.prevent="handleCreateTicket">
            <KFormField :label="t('portal.support.subject')" :required="true">
              <KInput v-model="ticketForm.subject" :placeholder="t('portal.support.subjectPlaceholder')" />
            </KFormField>
            <KFormField :label="t('portal.support.message')" :required="true">
              <KTextarea v-model="ticketForm.message" :placeholder="t('portal.support.messagePlaceholder')" :rows="4" />
            </KFormField>
            <div class="sp__form-actions">
              <KButton variant="ghost" size="sm" @click="showCreateForm = false">{{ t('portal.support.cancel') }}</KButton>
              <KButton type="submit" variant="primary" size="sm" :loading="ticketsStore.loading" :disabled="!ticketForm.subject || !ticketForm.message">
                {{ t('portal.support.create') }}
              </KButton>
            </div>
          </form>
        </div>

        <!-- Ticket list -->
        <KEmptyState
          v-if="!ticketsStore.list.length && !showCreateForm"
          :title="t('portal.support.noTickets')"
          :description="t('portal.support.noTicketsDesc')"
          icon="🎫"
        />

        <div v-else-if="ticketsStore.list.length" class="sp__tickets-list">
          <div
            v-for="ticket in ticketsStore.list"
            :key="ticket.id"
            class="sp__ticket-row"
            @click="handleViewTicket(ticket.id)"
          >
            <div class="sp__ticket-row-info">
              <span class="sp__ticket-row-subject">{{ ticket.subject }}</span>
              <KStatusPill :status="ticket.status === 'open' ? 'active' : 'disabled'" class="sp__ticket-row-status">
                {{ ticket.status === 'open' ? t('portal.support.open') : t('portal.support.closed') }}
              </KStatusPill>
            </div>
          </div>
        </div>
      </template>
    </section>
  </div>
</template>
<style scoped>
.sp {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
  padding-bottom: var(--space-8);
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
.sp__usage-content {
  display: flex;
  align-items: center;
  gap: var(--space-6);
}
.sp__usage-info {
  flex: 1;
}
.sp__usage-remaining {
  font-size: var(--text-lg);
  font-weight: 700;
  margin-bottom: var(--space-1);
}
.sp__usage-label {
  font-size: var(--text-xs);
  color: var(--color-muted);
  margin-bottom: var(--space-3);
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
.sp__progress-labels {
  display: flex;
  justify-content: space-between;
  font-size: var(--text-xs);
  color: var(--color-muted);
  margin-top: var(--space-2);
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
.sp__new-ticket {
  margin-bottom: var(--space-4);
}
.sp__new-ticket-btn {
  display: inline-flex;
  align-items: center;
  padding: var(--space-3) var(--space-4);
  background: var(--color-primary);
  color: #fff;
  border: none;
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
  font-weight: 500;
  cursor: pointer;
  min-height: 44px;
  transition: opacity 0.2s;
}
.sp__new-ticket-btn:hover {
  opacity: 0.9;
}
.sp__ticket-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
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
.sp__ticket-row-status {
  flex-shrink: 0;
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
.sp__reply-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  margin-top: var(--space-3);
  padding-top: var(--space-3);
  border-top: 1px solid var(--color-border);
}

/* ===== Mobile responsive ===== */
@media (max-width: 640px) {
  .sp__usage-content {
    flex-direction: column;
    align-items: stretch;
    text-align: center;
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
  .sp__profile-card {
    flex-wrap: wrap;
  }
}

@media (max-width: 400px) {
  .sp__account-grid {
    grid-template-columns: 1fr;
  }
}

/* ===== RTL support ===== */
[dir="rtl"] .sp__welcome {
  text-align: right;
}
[dir="rtl"] .sp__section {
  text-align: right;
}
[dir="rtl"] .sp__section-title {
  flex-direction: row-reverse;
  text-align: right;
}
[dir="rtl"] .sp__account-item {
  text-align: right;
}
[dir="rtl"] .sp__usage-content {
  flex-direction: row-reverse;
}
[dir="rtl"] .sp__usage-info {
  text-align: right;
}
[dir="rtl"] .sp__progress-labels {
  flex-direction: row-reverse;
}
[dir="rtl"] .sp__profile-card {
  flex-direction: row-reverse;
}
[dir="rtl"] .sp__profile-info {
  text-align: right;
}
[dir="rtl"] .sp__sub-url-row {
  flex-direction: row-reverse;
}
[dir="rtl"] .sp__sub-url-label {
  text-align: right;
}
[dir="rtl"] .sp__sub-url-desc {
  text-align: right;
}
[dir="rtl"] .sp__sub-url-input {
  direction: ltr;
  text-align: left;
}
[dir="rtl"] .sp__ticket-row {
  flex-direction: row-reverse;
}
[dir="rtl"] .sp__ticket-row-info {
  flex-direction: row-reverse;
  text-align: right;
}
[dir="rtl"] .sp__ticket-header {
  flex-direction: row-reverse;
}
[dir="rtl"] .sp__form-actions {
  flex-direction: row-reverse;
}
[dir="rtl"] .sp__back-btn {
  flex-direction: row-reverse;
}
[dir="rtl"] .sp__new-ticket-btn {
  flex-direction: row-reverse;
}
[dir="rtl"] .sp__notice {
  text-align: right;
}

@media (max-width: 640px) {
  [dir="rtl"] .sp__usage-content {
    flex-direction: column-reverse;
    text-align: center;
  }
  [dir="rtl"] .sp__sub-url-row {
    flex-direction: column-reverse;
  }
}
</style>
