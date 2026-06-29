<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useDomainsStore, type VpnDomain } from '@/stores/domains'
import { useToast } from '@koris/composables/useToast'
import { useConfirm } from '@koris/composables/useConfirm'
import { useI18n } from '@koris/composables/useI18n'
import KDataTable from '@koris/ui/KDataTable.vue'
import KButton from '@koris/ui/KButton.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'
import DomainAddDrawer from '@/components/DomainAddDrawer.vue'
import DomainRotateDialog from '@/components/DomainRotateDialog.vue'
import DomainHistoryDrawer from '@/components/DomainHistoryDrawer.vue'

const { t } = useI18n()
const store = useDomainsStore()
const toast = useToast()
const { confirm } = useConfirm()

// ─── Drawer / Dialog State ───────────────────────────────────────────────────
const showAddDrawer = ref(false)
const showRotateDialog = ref(false)
const showHistoryDrawer = ref(false)
const selectedDomain = ref<VpnDomain | null>(null)

// ─── Table Columns ───────────────────────────────────────────────────────────
const columns = computed(() => [
  { key: 'name', label: 'Domain Name', sortable: true },
  { key: 'ip_address', label: 'Current IP', sortable: true },
  { key: 'status', label: 'Status', sortable: true, width: '120px' },
  { key: 'binding_count', label: 'Protocols', sortable: true, width: '100px' },
  { key: 'cert_status', label: 'Certificate', sortable: true, width: '130px' },
  { key: 'actions', label: '', sortable: false, width: '220px' },
])

// ─── Status Badge Helpers ────────────────────────────────────────────────────
type StatusVariant = 'ok' | 'bad' | 'idle'

function getStatusVariant(status: string): StatusVariant {
  switch (status) {
    case 'active': return 'ok'
    case 'blocked': return 'bad'
    case 'retired': return 'idle'
    default: return 'idle'
  }
}

type CertVariant = 'ok' | 'warn' | 'bad' | 'idle'

function getCertVariant(certStatus: string): CertVariant {
  switch (certStatus) {
    case 'valid': return 'ok'
    case 'expiring_soon': return 'warn'
    case 'expired': return 'bad'
    case 'none': return 'idle'
    default: return 'idle'
  }
}

function getCertLabel(certStatus: string): string {
  switch (certStatus) {
    case 'valid': return 'Valid'
    case 'expiring_soon': return 'Expiring Soon'
    case 'expired': return 'Expired'
    case 'none': return 'None'
    default: return certStatus
  }
}

// ─── Actions ─────────────────────────────────────────────────────────────────
function openRotateDialog(domain: VpnDomain) {
  selectedDomain.value = domain
  showRotateDialog.value = true
}

function openHistoryDrawer(domain: VpnDomain) {
  selectedDomain.value = domain
  showHistoryDrawer.value = true
}

async function changeStatus(domain: VpnDomain, newStatus: 'active' | 'blocked' | 'retired') {
  const actionLabel = newStatus === 'blocked' ? 'block' : newStatus === 'retired' ? 'retire' : 'activate'

  // When blocking a domain with active bindings, warn the admin upfront
  let confirmMessage = `Are you sure you want to ${actionLabel} "${domain.name}"?`
  if (newStatus === 'blocked' && domain.binding_count > 0) {
    confirmMessage += `\n\n⚠️ This domain has ${domain.binding_count} active protocol binding${domain.binding_count !== 1 ? 's' : ''}. Existing bindings will be preserved but clients using this domain may experience connectivity issues. No new bindings can reference a blocked domain.`
  }

  const confirmed = await confirm({
    title: `${actionLabel.charAt(0).toUpperCase() + actionLabel.slice(1)} Domain`,
    message: confirmMessage,
    variant: newStatus === 'blocked' ? 'danger' : 'default',
    icon: newStatus === 'blocked' ? '⚠' : '⚡',
    confirmText: actionLabel.charAt(0).toUpperCase() + actionLabel.slice(1),
    cancelText: 'Cancel',
  })
  if (!confirmed) return

  const success = await store.updateDomain(domain.id, { status: newStatus })
  if (success) {
    toast.success(`Domain "${domain.name}" ${newStatus === 'active' ? 'activated' : newStatus}`)

    // After blocking, show a follow-up warning about affected bindings
    if (newStatus === 'blocked' && domain.binding_count > 0) {
      toast.warning(
        `${domain.binding_count} protocol binding${domain.binding_count !== 1 ? 's' : ''} still reference "${domain.name}". Consider reassigning them to an active domain.`
      )
    }
  } else {
    toast.error(`Failed to ${actionLabel} domain`)
  }
}

function onDomainAdded() {
  showAddDrawer.value = false
  store.fetchDomains()
}

function onIPRotated() {
  showRotateDialog.value = false
  selectedDomain.value = null
  store.fetchDomains()
}

async function removeDomain(domain: VpnDomain) {
  const confirmed = await confirm({
    title: 'Delete Domain',
    message: domain.binding_count > 0
      ? `Cannot delete "${domain.name}" — it has ${domain.binding_count} active binding${domain.binding_count !== 1 ? 's' : ''}. Remove bindings first.`
      : `Delete "${domain.name}"? This action cannot be undone.`,
    variant: 'danger',
    confirmText: 'Delete',
    cancelText: 'Cancel',
  })
  if (!confirmed) return
  if (domain.binding_count > 0) return

  const success = await store.deleteDomain(domain.id)
  if (success) {
    toast.success(`Domain "${domain.name}" deleted`)
  } else {
    toast.error('Failed to delete domain')
  }
}

// ─── Lifecycle ───────────────────────────────────────────────────────────────
onMounted(() => {
  store.fetchDomains()
})
</script>

<template>
  <div class="page domains-view">
    <header class="page-header">
      <KButton variant="primary" icon="+" @click="showAddDrawer = true">
        Add Domain
      </KButton>
    </header>

    <!-- Domains Table -->
    <KDataTable
      :columns="columns"
      :data="store.domains"
      :loading="store.loading"
      row-key="id"
    >
      <!-- Domain Name -->
      <template #cell-name="{ value }">
        <span class="domain-name">{{ value }}</span>
      </template>

      <!-- Current IP -->
      <template #cell-ip_address="{ value }">
        <code class="ip-address">{{ value }}</code>
      </template>

      <!-- Status Badge -->
      <template #cell-status="{ value }">
        <span
          :class="['status-badge', `status-badge--${getStatusVariant(value)}`]"
          role="status"
          :aria-label="`Status: ${value}`"
        >
          <span class="status-badge__dot" aria-hidden="true" />
          <span class="status-badge__text">{{ value }}</span>
        </span>
      </template>

      <!-- Bound Protocols Count -->
      <template #cell-binding_count="{ value }">
        <span class="binding-count">{{ value }}</span>
      </template>

      <!-- Certificate Status -->
      <template #cell-cert_status="{ value }">
        <span
          :class="['cert-indicator', `cert-indicator--${getCertVariant(value)}`]"
          role="status"
          :aria-label="`Certificate: ${getCertLabel(value)}`"
        >
          <span class="cert-indicator__dot" aria-hidden="true" />
          <span class="cert-indicator__text">{{ getCertLabel(value) }}</span>
        </span>
      </template>

      <!-- Row Actions -->
      <template #cell-actions="{ row }">
        <div class="actions-cell">
          <KButton variant="ghost" size="sm" @click="openRotateDialog(row)">
            Rotate IP
          </KButton>
          <KButton variant="ghost" size="sm" @click="openHistoryDrawer(row)">
            History
          </KButton>
          <KButton
            v-if="row.status !== 'blocked'"
            variant="ghost"
            size="sm"
            @click="changeStatus(row, 'blocked')"
          >
            Block
          </KButton>
          <KButton
            v-if="row.status !== 'retired'"
            variant="ghost"
            size="sm"
            @click="changeStatus(row, 'retired')"
          >
            Retire
          </KButton>
          <KButton
            v-if="row.status !== 'active'"
            variant="ghost"
            size="sm"
            @click="changeStatus(row, 'active')"
          >
            Activate
          </KButton>
          <KButton
            variant="ghost"
            size="sm"
            class="delete-btn"
            @click="removeDomain(row)"
          >
            Delete
          </KButton>
        </div>
      </template>
    </KDataTable>

    <KEmptyState
      v-if="!store.loading && store.domains.length === 0"
      icon="🌐"
      title="No Domains"
      description="Add a domain to get started with censorship-resilient VPN connectivity."
    />

    <!-- Add Domain Drawer -->
    <DomainAddDrawer
      :open="showAddDrawer"
      @close="showAddDrawer = false"
      @created="onDomainAdded"
    />

    <!-- Rotate IP Dialog -->
    <DomainRotateDialog
      v-if="showRotateDialog && selectedDomain"
      :domain="selectedDomain"
      @close="showRotateDialog = false; selectedDomain = null"
      @rotated="onIPRotated"
    />

    <!-- History Drawer -->
    <DomainHistoryDrawer
      v-if="showHistoryDrawer && selectedDomain"
      :domain="selectedDomain"
      :open="showHistoryDrawer"
      @close="showHistoryDrawer = false; selectedDomain = null"
    />
  </div>
</template>

<style scoped>
.domains-view {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

/* ─── Domain Name ─── */
.domain-name {
  font-weight: var(--font-medium);
}

/* ─── IP Address ─── */
.ip-address {
  font-size: var(--text-xs);
  background: var(--color-surface-2);
  padding: 2px 6px;
  border-radius: var(--radius-sm);
}

/* ─── Status Badge ─── */
.status-badge {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  border-radius: var(--radius-full);
  padding: 3px 8px;
  font-size: var(--text-xs);
  font-weight: var(--font-medium);
  white-space: nowrap;
  line-height: 1;
}

.status-badge__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  flex-shrink: 0;
}

.status-badge__text {
  text-transform: capitalize;
}

.status-badge--ok {
  background: rgba(34, 197, 94, 0.12);
  color: var(--color-success);
}

.status-badge--ok .status-badge__dot {
  background: var(--color-success);
}

.status-badge--bad {
  background: rgba(239, 68, 68, 0.12);
  color: var(--color-danger);
}

.status-badge--bad .status-badge__dot {
  background: var(--color-danger);
}

.status-badge--idle {
  background: rgba(139, 152, 165, 0.12);
  color: var(--color-muted);
}

.status-badge--idle .status-badge__dot {
  background: var(--color-muted);
}

/* ─── Certificate Indicator ─── */
.cert-indicator {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  border-radius: var(--radius-full);
  padding: 3px 8px;
  font-size: var(--text-xs);
  font-weight: var(--font-medium);
  white-space: nowrap;
  line-height: 1;
}

.cert-indicator__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  flex-shrink: 0;
}

.cert-indicator--ok {
  background: rgba(34, 197, 94, 0.12);
  color: var(--color-success);
}

.cert-indicator--ok .cert-indicator__dot {
  background: var(--color-success);
}

.cert-indicator--warn {
  background: rgba(245, 158, 11, 0.12);
  color: var(--color-warning);
}

.cert-indicator--warn .cert-indicator__dot {
  background: var(--color-warning);
}

.cert-indicator--bad {
  background: rgba(239, 68, 68, 0.12);
  color: var(--color-danger);
}

.cert-indicator--bad .cert-indicator__dot {
  background: var(--color-danger);
}

.cert-indicator--idle {
  background: rgba(139, 152, 165, 0.12);
  color: var(--color-muted);
}

.cert-indicator--idle .cert-indicator__dot {
  background: var(--color-muted);
}

/* ─── Binding Count ─── */
.binding-count {
  font-variant-numeric: tabular-nums;
}

/* ─── Actions ─── */
.actions-cell {
  display: flex;
  gap: var(--space-1);
  flex-wrap: wrap;
}

.delete-btn {
  color: var(--color-danger, #ef4444);
}

@media (max-width: 640px) {
  .actions-cell {
    flex-direction: column;
  }
}
</style>
