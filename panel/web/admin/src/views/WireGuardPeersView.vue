<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useWireGuard, type WireGuardPeerFilters } from '@/composables/useWireGuard'
import { useNodesStore } from '@/stores/nodes'
import { useConfirm } from '@koris/composables/useConfirm'
import { useToast } from '@koris/composables/useToast'
import { useI18n } from '@koris/composables/useI18n'
import KDataTable from '@koris/ui/KDataTable.vue'
import KButton from '@koris/ui/KButton.vue'
import KSelect from '@koris/ui/KSelect.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'
import WireGuardPeerCreate from '@/components/WireGuardPeerCreate.vue'

const { t } = useI18n()
const { peers, loading, fetchPeers, deletePeer, downloadConfig } = useWireGuard()
const nodesStore = useNodesStore()
const { confirm } = useConfirm()
const toast = useToast()

const showCreateDialog = ref(false)
const filterNode = ref<number | undefined>(undefined)
const filterStatus = ref<string>('')
const filterCustomer = ref<string>('')

const columns = computed(() => [
  { key: 'id', label: 'ID', sortable: true, width: '60px' },
  { key: 'customer_username', label: t('wireguard.customer'), sortable: true },
  { key: 'node_name', label: t('wireguard.node'), sortable: true },
  { key: 'public_key', label: t('wireguard.public_key'), sortable: false },
  { key: 'allowed_ips', label: t('wireguard.allowed_ips'), sortable: true },
  { key: 'status', label: t('wireguard.status'), sortable: true },
  { key: 'last_handshake_at', label: t('wireguard.last_handshake'), sortable: true },
  { key: 'transfer', label: t('wireguard.transfer'), sortable: false },
  { key: 'actions', label: '', sortable: false, width: '120px' },
])

const nodeOptions = computed(() => [
  { label: t('wireguard.all_nodes'), value: '' },
  ...nodesStore.list.map(n => ({ label: n.name, value: String(n.id) })),
])

const statusOptions = computed(() => [
  { label: t('wireguard.all_statuses'), value: '' },
  { label: t('wireguard.active'), value: 'active' },
  { label: t('wireguard.revoked'), value: 'revoked' },
])

const filteredPeers = computed(() => {
  let result = peers.value
  if (filterCustomer.value) {
    const q = filterCustomer.value.toLowerCase()
    result = result.filter(p => p.customer_username?.toLowerCase().includes(q))
  }
  return result
})

function truncateKey(key: string): string {
  if (!key) return ''
  return key.length > 12 ? key.slice(0, 8) + '…' + key.slice(-4) : key
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1073741824) return `${(bytes / 1048576).toFixed(1)} MB`
  return `${(bytes / 1073741824).toFixed(2)} GB`
}

function formatHandshake(ts: string | null): string {
  if (!ts) return '—'
  const d = new Date(ts)
  const now = Date.now()
  const diff = now - d.getTime()
  if (diff < 60000) return t('wireguard.just_now')
  if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`
  return d.toLocaleDateString()
}

async function loadPeers() {
  const filters: WireGuardPeerFilters = {}
  if (filterNode.value) filters.node_id = filterNode.value
  if (filterStatus.value) filters.status = filterStatus.value
  await fetchPeers(filters)
}

async function handleDelete(id: number) {
  const confirmed = await confirm({
    title: t('wireguard.confirm_revoke_title'),
    message: t('wireguard.confirm_revoke_msg'),
    variant: 'danger',
    icon: '⚠',
    confirmText: t('wireguard.revoke'),
    cancelText: t('btn.cancel'),
  })
  if (!confirmed) return
  const success = await deletePeer(id)
  if (success) {
    toast.success(t('wireguard.peer_revoked'))
  } else {
    toast.error(t('wireguard.revoke_error'))
  }
}

function onNodeFilterChange(val: string | number) {
  filterNode.value = val ? Number(val) : undefined
  loadPeers()
}

function onStatusFilterChange(val: string | number) {
  filterStatus.value = String(val)
  loadPeers()
}

function onPeerCreated() {
  showCreateDialog.value = false
  loadPeers()
}

onMounted(() => {
  nodesStore.loadNodes()
  loadPeers()
})
</script>

<template>
  <div class="page wireguard-peers-view">
    <header class="page-header">
      <KButton variant="primary" icon="+" @click="showCreateDialog = true">
        {{ t('wireguard.create_peer') }}
      </KButton>
    </header>

    <!-- Filters -->
    <div class="filter-row">
      <KSelect
        :model-value="filterNode ? String(filterNode) : ''"
        :options="nodeOptions"
        :aria-label="t('wireguard.filter_node')"
        @update:model-value="onNodeFilterChange"
      />
      <KSelect
        :model-value="filterStatus"
        :options="statusOptions"
        :aria-label="t('wireguard.filter_status')"
        @update:model-value="onStatusFilterChange"
      />
      <input
        v-model="filterCustomer"
        class="filter-input"
        :placeholder="t('wireguard.filter_customer')"
        :aria-label="t('wireguard.filter_customer')"
      />
    </div>

    <!-- Data Table -->
    <KDataTable
      :columns="columns"
      :data="filteredPeers"
      :loading="loading"
      row-key="id"
    >
      <template #cell-customer_username="{ value }">
        {{ value || '—' }}
      </template>
      <template #cell-public_key="{ value }">
        <code class="key-truncated" :title="value">{{ truncateKey(value) }}</code>
      </template>
      <template #cell-status="{ value }">
        <KStatusPill :status="value" size="sm" />
      </template>
      <template #cell-last_handshake_at="{ value }">
        {{ formatHandshake(value) }}
      </template>
      <template #cell-transfer="{ row }">
        <span class="transfer-cell">
          <span class="text-muted">↓</span> {{ formatBytes(row.rx_bytes) }}
          <span class="text-muted" style="margin-left:8px">↑</span> {{ formatBytes(row.tx_bytes) }}
        </span>
      </template>
      <template #cell-actions="{ row }">
        <div class="actions-cell">
          <KButton variant="ghost" size="sm" @click="downloadConfig(row.id)">
            {{ t('wireguard.download') }}
          </KButton>
          <KButton variant="danger" size="sm" @click="handleDelete(row.id)">
            {{ t('wireguard.revoke') }}
          </KButton>
        </div>
      </template>
    </KDataTable>

    <KEmptyState
      v-if="!loading && filteredPeers.length === 0"
      icon="🔒"
      :title="t('wireguard.no_peers')"
      :description="t('wireguard.no_peers_desc')"
    />

    <!-- Create Peer Dialog -->
    <WireGuardPeerCreate
      v-if="showCreateDialog"
      @close="showCreateDialog = false"
      @created="onPeerCreated"
    />
  </div>
</template>

<style scoped>
.wireguard-peers-view {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.filter-row {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  flex-wrap: wrap;
}

.filter-input {
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-surface);
  color: var(--color-text);
  font-size: var(--text-sm);
  width: 200px;
}

.filter-input:focus {
  outline: none;
  border-color: var(--color-primary);
}

.key-truncated {
  font-size: var(--text-xs);
  background: var(--color-surface-2);
  padding: 2px 6px;
  border-radius: var(--radius-sm);
}

.transfer-cell {
  font-size: var(--text-xs);
  white-space: nowrap;
}

.actions-cell {
  display: flex;
  gap: var(--space-2);
}

@media (max-width: 640px) {
  .filter-row {
    flex-direction: column;
    align-items: stretch;
  }
  .filter-input {
    width: 100%;
  }
}
</style>
