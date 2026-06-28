<script setup lang="ts">
/**
 * UserDetailPanel — Main detail panel component for the Users tab.
 *
 * Slides in from the right using KSlideOver (KDrawer wrapper).
 * Renders a single scrollable view with sections:
 *   DetailHeader → ProfileFields → AdvancedSettings → ConnectedClients → TransactionList → Action bar
 *
 * Desktop (>1024px): max 480px, no overlay (table stays interactive)
 * Mobile (≤1024px): full-width overlay
 *
 * Fetches fresh data on open and supports switching users without close/reopen.
 * Shows error state with retry button on load failure.
 *
 * Requirements: 1.2, 1.3, 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.9, 2.10, 2.13, 2.14
 */
import { ref, watch, computed, onMounted, onUnmounted } from 'vue'
import { useApi } from '@koris/composables/useApi'
import KButton from '@koris/ui/KButton.vue'
import KThreeDotMenu from '@koris/ui/KThreeDotMenu.vue'
import KModal from '@koris/ui/KModal.vue'
import DetailHeader from './DetailHeader.vue'
import ProfileFields from './ProfileFields.vue'
import AdvancedSettings from './AdvancedSettings.vue'
import ConnectedClients from './ConnectedClients.vue'
import TransactionList from './TransactionList.vue'
import type { CustomerDetail } from '@koris/types'
import type { ProfileFormData } from './ProfileFields.vue'
import type { MenuItem } from '@koris/ui/KThreeDotMenu.vue'

export interface UserDetailPanelProps {
  userId: number | null
  open: boolean
}

const props = defineProps<UserDetailPanelProps>()

const emit = defineEmits<{
  close: []
  edit: []
  updated: []
  'top-up': [username: string, balance: number]
  'deduct': [username: string, balance: number]
}>()

// ─── API & State ────────────────────────────────────────────────────────────
const { get, post } = useApi({ showErrorToast: false })

const customer = ref<CustomerDetail | null>(null)
const loading = ref(false)
const error = ref<string | null>(null)

// ─── Responsive: detect mobile for overlay behavior ─────────────────────────
const isMobile = ref(false)

function checkMobile() {
  isMobile.value = window.innerWidth <= 1024
}

onMounted(() => {
  checkMobile()
  window.addEventListener('resize', checkMobile)
})

onUnmounted(() => {
  window.removeEventListener('resize', checkMobile)
})

// ─── Three-dot menu items ───────────────────────────────────────────────────
const menuItems: MenuItem[] = [
  { key: 'clients', label: 'Connected Clients', icon: '📡' },
  { key: 'transactions', label: 'Transactions', icon: '💳' },
  { key: 'reset-usage', label: 'Reset Usage', icon: '🔄' },
]

// ─── Modal state for Connected Clients / Transactions ───────────────────────
const showClientsModal = ref(false)
const showTransactionsModal = ref(false)

// ─── Derived form data for ProfileFields ────────────────────────────────────
const profileFormData = ref<ProfileFormData>({
  username: '',
  status: 'active',
  data_limit: '',
  expiry_date: '',
  note: '',
  allowed_protocols: [],
  protocol_options: {},
  billing_enabled: true,
})

/** Sync form data when customer data is fetched */
function syncFormFromCustomer() {
  if (!customer.value) return

  const expirationCheck = customer.value.radius_checks?.find(
    (r) => r.attribute === 'Expiration'
  )
  const protocols = customer.value.radius_replies
    ?.filter((r) => r.attribute === 'Allowed-Protocol')
    .map((r) => r.value) ?? []

  let dataLimit = ''
  if (customer.value.subscription?.data_limit_gb) {
    dataLimit = String(customer.value.subscription.data_limit_gb)
  }

  profileFormData.value = {
    username: customer.value.username,
    status: customer.value.status,
    data_limit: dataLimit,
    expiry_date: expirationCheck?.value ?? '',
    note: customer.value.notes ?? '',
    allowed_protocols: protocols,
    protocol_options: {},
    billing_enabled: customer.value.billing_enabled !== false,
  }
}

// ─── Derived advanced settings values ───────────────────────────────────────
const speedLimit = computed(() => {
  if (!customer.value) return 0
  const speedCheck = customer.value.radius_replies?.find(
    (r) => r.attribute === 'WISPr-Bandwidth-Max-Down' || r.attribute === 'Mikrotik-Rate-Limit'
  )
  if (!speedCheck) return 0
  // Value is typically in bps, convert to Mbps
  const bps = Number(speedCheck.value) || 0
  return Math.round(bps / 1_000_000)
})

const connectionLimit = computed(() => {
  if (!customer.value) return 0
  const connCheck = customer.value.radius_checks?.find(
    (r) => r.attribute === 'Simultaneous-Use'
  )
  return Number(connCheck?.value) || 0
})

// ─── Advanced Settings Toggle (collapsed by default, shown if values non-zero) ─
const showAdvanced = ref(false)

// Auto-expand when values are non-zero
watch([speedLimit, connectionLimit], ([speed, conn]) => {
  if (speed > 0 || conn > 0) {
    showAdvanced.value = true
  }
})

// ─── Usage data ─────────────────────────────────────────────────────────────
const usedBytes = computed(() => {
  if (!customer.value?.subscription) return 0
  return (customer.value.subscription.data_used_gb ?? 0) * 1_073_741_824 // GB to bytes
})

const limitBytes = computed(() => {
  if (!customer.value?.subscription) return 0
  return (customer.value.subscription.data_limit_gb ?? 0) * 1_073_741_824 // GB to bytes
})

// ─── Fetch user data ────────────────────────────────────────────────────────

interface CustomerDetailResponse {
  ok: boolean
  customer: CustomerDetail
}

async function fetchUserData(userId: number): Promise<void> {
  loading.value = true
  error.value = null

  try {
    const res = await get<CustomerDetailResponse>(`/api/customers/${userId}`)
    customer.value = res.customer
    syncFormFromCustomer()
  } catch (e: any) {
    error.value = e?.message || 'Failed to load user data'
    customer.value = null
  } finally {
    loading.value = false
  }
}

function retry(): void {
  if (props.userId) {
    fetchUserData(props.userId)
  }
}

/** Expose refresh method so parent can trigger a data refetch (e.g., after wallet operation) */
function refresh(): void {
  if (props.userId) {
    fetchUserData(props.userId)
  }
}

defineExpose({ refresh })

// ─── Watch for open / userId changes ────────────────────────────────────────
watch(
  () => [props.open, props.userId] as const,
  ([isOpen, userId], oldValue) => {
    const wasOpen = oldValue?.[0] ?? false
    if (isOpen && userId) {
      // Fetch fresh data when panel opens or user switches
      fetchUserData(userId)
    }
    if (!isOpen && wasOpen) {
      // Clear data on close (optional)
      customer.value = null
      error.value = null
    }
  },
  { immediate: true }
)

// ─── Action handlers ────────────────────────────────────────────────────────

function handleClose(): void {
  emit('close')
}

function handleModify(): void {
  emit('edit')
}

function handleMenuSelect(key: string): void {
  if (key === 'clients') {
    showClientsModal.value = true
  } else if (key === 'transactions') {
    showTransactionsModal.value = true
  } else if (key === 'reset-usage') {
    // Reset usage — call API
    if (props.userId) {
      post(`/api/customers/${props.userId}/reset-traffic`, {}).then(() => {
        refresh()
      })
    }
  }
}

function handleTopUp(): void {
  if (customer.value) {
    emit('top-up', customer.value.username, customer.value.credit ?? 0)
  }
}

function handleDeduct(): void {
  if (customer.value) {
    emit('deduct', customer.value.username, customer.value.credit ?? 0)
  }
}

function handleBillingToggle(value: boolean): void {
  profileFormData.value = { ...profileFormData.value, billing_enabled: value }
}

function onSpeedLimitUpdate(_value: number): void {
  // Local update - saved on Modify click
}

function onConnectionLimitUpdate(_value: number): void {
  // Local update - saved on Modify click
}

// ─── Keyboard: Escape to close ──────────────────────────────────────────────
function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && props.open) {
    event.preventDefault()
    handleClose()
  }
}

onMounted(() => {
  document.addEventListener('keydown', handleKeydown)
})

onUnmounted(() => {
  document.removeEventListener('keydown', handleKeydown)
})
</script>

<template>
  <Teleport to="body">
    <!-- Overlay: mobile (opaque) + desktop (transparent click-catcher) -->
    <Transition name="panel-overlay">
      <div
        v-if="open"
        class="user-detail-panel__overlay"
        :class="{ 'user-detail-panel__overlay--desktop': !isMobile }"
        aria-hidden="true"
        @click="handleClose"
      />
    </Transition>

    <!-- Panel -->
    <Transition name="panel-slide">
      <aside
        v-if="open"
        class="user-detail-panel"
        :class="{ 'user-detail-panel--mobile': isMobile }"
        role="complementary"
        aria-label="User detail panel"
      >
        <!-- Close button -->
        <button
          type="button"
          class="user-detail-panel__close-btn"
          aria-label="Close panel"
          @click="handleClose"
        >
          <svg width="20" height="20" viewBox="0 0 20 20" fill="none" aria-hidden="true">
            <path d="M15 5L5 15M5 5l10 10" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
        </button>

        <!-- Error state -->
        <div v-if="error" class="user-detail-panel__error">
          <p class="user-detail-panel__error-message">{{ error }}</p>
          <KButton variant="primary" size="sm" @click="retry">
            Retry
          </KButton>
        </div>

        <!-- Loading state -->
        <div v-else-if="loading" class="user-detail-panel__loading">
          <div class="user-detail-panel__skeleton" />
          <div class="user-detail-panel__skeleton user-detail-panel__skeleton--short" />
          <div class="user-detail-panel__skeleton user-detail-panel__skeleton--medium" />
        </div>

        <!-- Content: single scrollable view -->
        <div v-else-if="customer" class="user-detail-panel__content">
          <!-- DetailHeader -->
          <DetailHeader
            :display-name="customer.display_name || customer.username"
            :status="customer.status"
            :used-bytes="usedBytes"
            :limit-bytes="limitBytes"
            :wallet-balance="customer.credit ?? 0"
            :billing-enabled="profileFormData.billing_enabled"
            @top-up="handleTopUp"
            @deduct="handleDeduct"
            @update:billing-enabled="handleBillingToggle"
          />

          <!-- ProfileFields -->
          <div class="user-detail-panel__section">
            <ProfileFields v-model="profileFormData" />
          </div>

          <!-- AdvancedSettings (collapsible) -->
          <div class="user-detail-panel__section">
            <button
              type="button"
              class="user-detail-panel__advanced-toggle"
              @click="showAdvanced = !showAdvanced"
            >
              <span>Advanced</span>
              <svg
                width="12" height="12" viewBox="0 0 12 12" fill="none" aria-hidden="true"
                :class="{ 'user-detail-panel__chevron--open': showAdvanced }"
                class="user-detail-panel__chevron"
              >
                <path d="M3 4.5L6 7.5L9 4.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
              </svg>
            </button>
            <AdvancedSettings
              v-if="showAdvanced"
              :speed-limit="speedLimit"
              :connection-limit="connectionLimit"
              @update:speed-limit="onSpeedLimitUpdate"
              @update:connection-limit="onConnectionLimitUpdate"
            />
          </div>

          <!-- Action Bar -->
          <div class="user-detail-panel__action-bar">
            <KThreeDotMenu
              :items="menuItems"
              placement="top-start"
              @select="handleMenuSelect"
            />
            <div class="user-detail-panel__action-bar-right">
              <KButton variant="ghost" @click="handleClose">
                Cancel
              </KButton>
              <KButton variant="primary" @click="handleModify">
                Modify
              </KButton>
            </div>
          </div>
        </div>
      </aside>
    </Transition>
  </Teleport>

  <!-- Connected Clients Modal -->
  <KModal
    :open="showClientsModal"
    title="Connected Clients"
    width="600px"
    @close="showClientsModal = false"
  >
    <ConnectedClients :user-id="userId" :show-title="false" />
  </KModal>

  <!-- Transactions Modal -->
  <KModal
    :open="showTransactionsModal"
    title="Transactions"
    width="600px"
    @close="showTransactionsModal = false"
  >
    <TransactionList
      :transactions="customer?.wallet_transactions ?? []"
      :show-title="false"
    />
  </KModal>
</template>

<style scoped>
/* ─── Panel Container ─────────────────────────────────────────────────────── */
.user-detail-panel {
  position: fixed;
  top: 0;
  right: 0;
  bottom: 0;
  width: 480px;
  max-width: 480px;
  z-index: var(--z-panel, 150);
  display: flex;
  flex-direction: column;
  background: var(--color-surface, #0b1120);
  border-left: 1px solid var(--color-border, #28333f);
  box-shadow: var(--shadow-xl, 0 30px 80px rgba(0, 0, 0, 0.4));
  overflow: hidden;
}

/* Mobile: full-width overlay */
.user-detail-panel--mobile {
  width: 100vw;
  max-width: 100vw;
  z-index: var(--z-modal, 200);
  border-left: none;
}

/* ─── Overlay (mobile only) ───────────────────────────────────────────────── */
.user-detail-panel__overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  z-index: calc(var(--z-modal, 200) - 1);
}

/* Desktop: transparent overlay just to catch clicks */
.user-detail-panel__overlay--desktop {
  background: transparent;
  z-index: calc(var(--z-panel, 150) - 1);
}

/* ─── Close Button ────────────────────────────────────────────────────────── */
.user-detail-panel__close-btn {
  position: absolute;
  top: var(--space-3, 12px);
  right: var(--space-3, 12px);
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  padding: 0;
  border: none;
  border-radius: var(--radius-sm, 6px);
  background: transparent;
  color: var(--color-muted, #8b98a5);
  cursor: pointer;
  transition: background var(--duration-fast, 0.12s) ease,
              color var(--duration-fast, 0.12s) ease;
}

.user-detail-panel__close-btn:hover {
  background: var(--color-surface-2, #1e2630);
  color: var(--color-text, #e6edf3);
}

.user-detail-panel__close-btn:focus-visible {
  outline: 2px solid var(--color-primary, #2563eb);
  outline-offset: 2px;
}

/* ─── Error State ─────────────────────────────────────────────────────────── */
.user-detail-panel__error {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--space-4, 16px);
  flex: 1;
  padding: var(--space-6, 24px);
  text-align: center;
}

.user-detail-panel__error-message {
  margin: 0;
  font-size: var(--text-sm, 0.875rem);
  color: var(--color-danger, #ef4444);
}

/* ─── Loading State ───────────────────────────────────────────────────────── */
.user-detail-panel__loading {
  display: flex;
  flex-direction: column;
  gap: var(--space-4, 16px);
  padding: var(--space-6, 24px);
}

.user-detail-panel__skeleton {
  height: 20px;
  width: 100%;
  border-radius: var(--radius-sm, 6px);
  background: linear-gradient(
    90deg,
    var(--color-surface-2, #1e2630) 25%,
    var(--color-surface-3, #2a3544) 50%,
    var(--color-surface-2, #1e2630) 75%
  );
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
}

.user-detail-panel__skeleton--short {
  width: 60%;
}

.user-detail-panel__skeleton--medium {
  width: 80%;
}

@keyframes shimmer {
  0% { background-position: 200% 0; }
  100% { background-position: -200% 0; }
}

/* ─── Content (scrollable) ────────────────────────────────────────────────── */
.user-detail-panel__content {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
}

.user-detail-panel__section {
  padding: var(--space-4, 16px);
  border-bottom: 1px solid var(--color-border, #28333f);
}

/* ─── Advanced Toggle ─────────────────────────────────────────────────────── */
.user-detail-panel__advanced-toggle {
  display: flex;
  align-items: center;
  gap: var(--space-2, 8px);
  width: 100%;
  padding: 0;
  margin-bottom: var(--space-3, 12px);
  border: none;
  background: none;
  color: var(--color-muted, #8b98a5);
  font-size: var(--text-sm, 13px);
  font-weight: 500;
  cursor: pointer;
  font-family: var(--font-family);
}

.user-detail-panel__advanced-toggle:hover {
  color: var(--color-text, #e6edf3);
}

.user-detail-panel__chevron {
  transition: transform 150ms ease;
}

.user-detail-panel__chevron--open {
  transform: rotate(180deg);
}

/* ─── Action Bar (fixed at bottom) ────────────────────────────────────────── */
.user-detail-panel__action-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2, 8px);
  padding: var(--space-4, 16px);
  border-top: 1px solid var(--color-border, #28333f);
  background: var(--color-surface, #0b1120);
  flex-shrink: 0;
  margin-top: auto;
}

.user-detail-panel__action-bar-right {
  display: flex;
  align-items: center;
  gap: var(--space-2, 8px);
}

/* ─── Panel Slide Transition (280ms ease-out) ─────────────────────────────── */
.panel-slide-enter-active {
  transition: transform 280ms ease-out;
}

.panel-slide-leave-active {
  transition: transform 280ms ease-out;
}

.panel-slide-enter-from,
.panel-slide-leave-to {
  transform: translateX(100%);
}

/* ─── Overlay Transition ──────────────────────────────────────────────────── */
.panel-overlay-enter-active,
.panel-overlay-leave-active {
  transition: opacity 280ms ease-out;
}

.panel-overlay-enter-from,
.panel-overlay-leave-to {
  opacity: 0;
}

/* ─── Respect reduced motion ──────────────────────────────────────────────── */
@media (prefers-reduced-motion: reduce) {
  .panel-slide-enter-active,
  .panel-slide-leave-active {
    transition-duration: 0ms;
  }

  .panel-overlay-enter-active,
  .panel-overlay-leave-active {
    transition-duration: 0ms;
  }

  .user-detail-panel__skeleton {
    animation: none;
  }
}
</style>
