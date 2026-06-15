<script setup lang="ts">
import { onMounted, computed } from 'vue'
import { usePortalAuthStore } from '@/stores/auth'
import { useUsageStore } from '@/stores/usage'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'
import PlanCard from '@/components/PlanCard.vue'
import UsageGauge from '@/components/UsageGauge.vue'

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

function formatBytes(value: number): string {
  if (value >= 1024 ** 4) return `${(value / 1024 ** 4).toFixed(2)} TB`
  if (value >= 1024 ** 3) return `${(value / 1024 ** 3).toFixed(2)} GB`
  if (value >= 1024 ** 2) return `${(value / 1024 ** 2).toFixed(2)} MB`
  if (value >= 1024) return `${(value / 1024).toFixed(2)} KB`
  return `${Math.round(value)} B`
}

function formatMoney(value: number): string {
  return `${new Intl.NumberFormat('en', { maximumFractionDigits: 0 }).format(value)} IRT`
}
</script>
<template>
  <div class="dashboard">
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

      <div class="dashboard__usage">
        <div class="usage-card">
          <div class="usage-card__header">
            <h3>Data Usage</h3>
            <KStatusPill :status="isOnline ? 'active' : 'disabled'">
              {{ isOnline ? 'Online' : 'Offline' }}
            </KStatusPill>
          </div>
          <UsageGauge :percent="usagePercent" />
          <div class="usage-card__details">
            <span>{{ formatBytes(totalUsageBytes) }} used</span>
            <span>{{ maxDataBytes ? formatBytes(maxDataBytes) : 'Unlimited' }} total</span>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
<style scoped>
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
.usage-card__details {
  display: flex;
  justify-content: space-between;
  font-size: var(--text-xs);
  color: var(--color-muted);
  margin-top: var(--space-3);
}
</style>
