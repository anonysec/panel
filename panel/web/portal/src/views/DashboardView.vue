<script setup lang="ts">
import { onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { usePortalAuthStore } from '@/stores/auth'
import { useUsageStore } from '@/stores/usage'
import { useUsageDisplay, formatBytes } from '@/composables/useUsageDisplay'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'
import PlanCard from '@/components/PlanCard.vue'
import KUsageBar from '@koris/ui/KUsageBar.vue'

const router = useRouter()
const auth = usePortalAuthStore()
const usageStore = useUsageStore()

onMounted(() => {
  usageStore.loadUsage()
})

const displayName = computed(() => auth.displayName)
const planName = computed(() => auth.planName)
const status = computed(() => auth.status)
const credit = computed(() => auth.credit)
const isOnline = computed(() => usageStore.isOnline)
const activeSessions = computed(() => usageStore.activeSessions)
const usagePercent = computed(() => usageStore.usagePercent)
const maxDataBytes = computed(() => usageStore.maxDataBytes)
const totalUsageBytes = computed(() => usageStore.totalUsageBytes)
const expiresAt = computed(() => auth.user?.subscription?.expires_at ?? '')

// Active sessions list (up to 4 most recent)
const activeSessionsList = computed(() => {
  return usageStore.sessions.filter(s => s.online).slice(0, 4)
})

// Use the useUsageDisplay composable for dynamic color and remaining calculations
const { remainingBytes, progressColor, daysRemaining } = useUsageDisplay(
  totalUsageBytes,
  maxDataBytes,
  expiresAt
)

// Alert banner: show when usage exceeds 95%
const showCriticalAlert = computed(() => {
  if (!maxDataBytes.value) return false
  return usagePercent.value >= 95
})

// Format remaining data display (e.g., "2.4 GB remaining / 10 GB")
const remainingDisplay = computed(() => {
  if (!maxDataBytes.value) return 'Unlimited data'
  return `${formatBytes(remainingBytes.value)} remaining / ${formatBytes(maxDataBytes.value)}`
})

// Format the expiry date for display
const formattedExpiryDate = computed(() => {
  if (!expiresAt.value) return 'No expiry set'
  return new Intl.DateTimeFormat('en', {
    year: 'numeric',
    month: 'short',
    day: '2-digit',
  }).format(new Date(expiresAt.value))
})

// Progress bar color as CSS variable value
const progressBarColor = computed(() => {
  switch (progressColor.value) {
    case 'red': return 'var(--color-danger)'
    case 'amber': return 'var(--color-warning)'
    default: return 'var(--color-success, #22c55e)'
  }
})

function formatMoney(value: number): string {
  return `${new Intl.NumberFormat('en', { maximumFractionDigits: 0 }).format(value)} IRT`
}

function formatDuration(seconds: number): string {
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (hours > 0) return `${hours}h ${minutes}m`
  return `${minutes}m`
}
</script>
<template>
  <div class="dashboard">
    <!-- Critical usage alert banner (persistent when usage > 95%) -->
    <div v-if="showCriticalAlert" class="dashboard__alert" role="alert">
      <svg class="dashboard__alert-icon" viewBox="0 0 20 20" fill="currentColor" width="20" height="20">
        <path fill-rule="evenodd" d="M8.485 2.495c.673-1.167 2.357-1.167 3.03 0l6.28 10.875c.673 1.167-.168 2.625-1.516 2.625H3.72c-1.347 0-2.189-1.458-1.515-2.625L8.485 2.495zM10 5a.75.75 0 01.75.75v3.5a.75.75 0 01-1.5 0v-3.5A.75.75 0 0110 5zm0 9a1 1 0 100-2 1 1 0 000 2z" clip-rule="evenodd" />
      </svg>
      <span class="dashboard__alert-text">
        Your data usage has exceeded 95% of your plan limit. You have {{ formatBytes(remainingBytes) }} remaining.
      </span>
    </div>

    <div class="dashboard__welcome">
      <h1 class="dashboard__title">Hello, {{ displayName }}</h1>
      <p class="dashboard__subtitle">Your VPN account is active and ready to connect.</p>
    </div>

    <KSkeleton v-if="usageStore.loading && !usageStore.usage" type="card" :count="3" />

    <template v-else>
      <div class="dashboard__stats">
        <PlanCard
          :plan-name="planName"
          :status="status"
          :expires-at="auth.user?.subscription?.expires_at"
        />

        <div class="stat-card">
          <div class="stat-card__label">Connection</div>
          <div class="stat-card__value">
            <KStatusPill :status="isOnline ? 'active' : 'disabled'">
              {{ isOnline ? 'Online' : 'Offline' }}
            </KStatusPill>
          </div>
          <div class="stat-card__sub">{{ activeSessions }} active session{{ activeSessions !== 1 ? 's' : '' }}</div>
        </div>

        <div class="stat-card">
          <div class="stat-card__label">Wallet Balance</div>
          <div class="stat-card__value">{{ formatMoney(credit) }}</div>
          <div class="stat-card__sub">Available credit</div>
        </div>
      </div>

      <!-- Enhanced Usage Display Section -->
      <div class="dashboard__usage">
        <div class="usage-card">
          <div class="usage-card__header">
            <h3>Data Usage</h3>
            <KStatusPill :status="isOnline ? 'active' : 'disabled'">
              {{ isOnline ? 'Online' : 'Offline' }}
            </KStatusPill>
          </div>

          <!-- Usage gauge -->
          <KUsageBar :used="totalUsageBytes" :limit="maxDataBytes || 0" />

          <!-- Remaining data: percentage and absolute value -->
          <div class="usage-card__remaining">
            <span class="usage-card__remaining-percent">{{ Math.max(0, 100 - usagePercent) }}% remaining</span>
            <span class="usage-card__remaining-absolute">{{ remainingDisplay }}</span>
          </div>

          <!-- Dynamic color progress bar -->
          <div class="usage-card__progress">
            <div class="progress-bar">
              <div
                class="progress-bar__fill"
                :style="{ width: `${Math.min(100, usagePercent)}%`, backgroundColor: progressBarColor }"
              ></div>
            </div>
            <div class="progress-bar__labels">
              <span>{{ formatBytes(totalUsageBytes) }} used</span>
              <span>{{ maxDataBytes ? formatBytes(maxDataBytes) : 'Unlimited' }} total</span>
            </div>
          </div>

          <!-- Subscription expiry and days remaining -->
          <div class="usage-card__expiry">
            <div class="usage-card__expiry-row">
              <span class="usage-card__expiry-label">Subscription Expires</span>
              <span class="usage-card__expiry-value">{{ formattedExpiryDate }}</span>
            </div>
            <div class="usage-card__expiry-row">
              <span class="usage-card__expiry-label">Days Remaining</span>
              <span class="usage-card__expiry-days" :class="{ 'usage-card__expiry-days--warning': daysRemaining <= 7 }">
                {{ daysRemaining }} day{{ daysRemaining !== 1 ? 's' : '' }}
              </span>
            </div>
          </div>

          <!-- Link to detailed usage page -->
          <div class="usage-card__link">
            <a class="usage-card__detail-link" @click.prevent="router.push({ name: 'portal-usage' })">
              View detailed usage &rarr;
            </a>
          </div>
        </div>
      </div>

      <!-- Active Sessions Section -->
      <div class="dashboard__sessions" v-if="activeSessionsList.length > 0">
        <div class="sessions-card">
          <div class="sessions-card__header">
            <h3>Active Sessions</h3>
            <KStatusPill status="active">{{ activeSessionsList.length }} active</KStatusPill>
          </div>
          <ul class="sessions-card__list">
            <li v-for="session in activeSessionsList" :key="session.id" class="sessions-card__item">
              <div class="sessions-card__ip">
                <svg viewBox="0 0 20 20" fill="currentColor" width="14" height="14">
                  <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM4.332 8.027a6.012 6.012 0 011.912-2.706C6.512 5.73 6.974 6 7.5 6A1.5 1.5 0 019 7.5V8a2 2 0 004 0c0-.738.402-1.423 1.053-1.758a6.033 6.033 0 011.615 2.309A1.5 1.5 0 0114.5 10h-.528a2.013 2.013 0 00-1.942 1.491l-.311 1.09A2 2 0 019.78 14H9.5a1.5 1.5 0 01-1.5-1.5V12a2 2 0 00-2-2h-.528a1.506 1.506 0 01-1.14-.527z" clip-rule="evenodd" />
                </svg>
                <span>{{ session.framed_ip }}</span>
              </div>
              <div class="sessions-card__duration">{{ formatDuration(session.session_seconds) }}</div>
            </li>
          </ul>
        </div>
      </div>
    </template>
  </div>
</template>
<style scoped>
.dashboard__alert {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4);
  background: var(--color-danger-bg, #fef2f2);
  border: 1px solid var(--color-danger, #ef4444);
  border-radius: var(--radius-md);
  margin-bottom: var(--space-4);
  color: var(--color-danger, #ef4444);
}
.dashboard__alert-icon {
  flex-shrink: 0;
}
.dashboard__alert-text {
  font-size: var(--text-sm);
  font-weight: 500;
}
.dashboard__welcome {
  margin-bottom: var(--space-6);
}
.dashboard__title {
  font-size: var(--text-2xl);
  font-weight: 700;
  margin-bottom: var(--space-1);
}
.dashboard__subtitle {
  color: var(--color-muted);
  font-size: var(--text-sm);
}
.dashboard__stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: var(--space-4);
  margin-bottom: var(--space-6);
}
.stat-card {
  padding: var(--space-5);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
}
.stat-card__label {
  font-size: var(--text-xs);
  color: var(--color-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: var(--space-2);
}
.stat-card__value {
  font-size: var(--text-xl);
  font-weight: 700;
  margin-bottom: var(--space-1);
}
.stat-card__sub {
  font-size: var(--text-xs);
  color: var(--color-muted);
}
.dashboard__usage {
  margin-top: var(--space-4);
}
.usage-card {
  padding: var(--space-5);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
}
.usage-card__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--space-4);
}
.usage-card__header h3 {
  font-size: var(--text-md);
  font-weight: 600;
}
.usage-card__remaining {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--space-1);
  margin-top: var(--space-3);
  margin-bottom: var(--space-4);
}
.usage-card__remaining-percent {
  font-size: var(--text-lg);
  font-weight: 600;
}
.usage-card__remaining-absolute {
  font-size: var(--text-sm);
  color: var(--color-muted);
}
.usage-card__progress {
  margin-bottom: var(--space-4);
}
.progress-bar {
  height: 8px;
  background: var(--color-border);
  border-radius: 4px;
  overflow: hidden;
}
.progress-bar__fill {
  height: 100%;
  border-radius: 4px;
  transition: width 0.4s ease, background-color 0.3s ease;
}
.progress-bar__labels {
  display: flex;
  justify-content: space-between;
  font-size: var(--text-xs);
  color: var(--color-muted);
  margin-top: var(--space-2);
}
.usage-card__expiry {
  border-top: 1px solid var(--color-border);
  padding-top: var(--space-4);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.usage-card__expiry-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.usage-card__expiry-label {
  font-size: var(--text-xs);
  color: var(--color-muted);
}
.usage-card__expiry-value {
  font-size: var(--text-sm);
  font-weight: 500;
}
.usage-card__expiry-days {
  font-size: var(--text-sm);
  font-weight: 600;
  color: var(--color-success, #22c55e);
}
.usage-card__expiry-days--warning {
  color: var(--color-danger, #ef4444);
}
.usage-card__link {
  margin-top: var(--space-4);
  text-align: center;
}
.usage-card__detail-link {
  font-size: var(--text-sm);
  color: var(--color-primary, #2563eb);
  cursor: pointer;
  font-weight: 500;
  text-decoration: none;
}
.usage-card__detail-link:hover {
  text-decoration: underline;
}
.dashboard__sessions {
  margin-top: var(--space-4);
}
.sessions-card {
  padding: var(--space-5);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
}
.sessions-card__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--space-4);
}
.sessions-card__header h3 {
  font-size: var(--text-md);
  font-weight: 600;
}
.sessions-card__list {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.sessions-card__item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--space-3);
  background: var(--color-bg);
  border-radius: var(--radius-md);
}
.sessions-card__ip {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--text-sm);
  font-weight: 500;
  color: var(--color-text);
}
.sessions-card__ip svg {
  color: var(--color-muted);
}
.sessions-card__duration {
  font-size: var(--text-xs);
  color: var(--color-muted);
  font-weight: 500;
}
</style>
