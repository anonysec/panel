<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useDomainsStore, type ProtocolBinding } from '@/stores/domains'
import { useToast } from '@koris/composables/useToast'
import { useConfirm } from '@koris/composables/useConfirm'
import { useApi } from '@koris/composables/useApi'
import KButton from '@koris/ui/KButton.vue'
import KInput from '@koris/ui/KInput.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'

/**
 * NodeDomainsTab — manage domains assigned to this node.
 *
 * Domains are used in VPN configs (OpenVPN, WireGuard, etc.) instead of raw IPs.
 * Primary domain = position 1, backups = position 2+.
 * DNS is managed externally (Cloudflare). This only tracks which domain names
 * to put in generated configs.
 */

const props = defineProps<{ nodeId: number }>()

const store = useDomainsStore()
const toast = useToast()
const { confirm } = useConfirm()
const { get } = useApi()

// ─── State ───────────────────────────────────────────────────────────────────
const bindings = ref<ProtocolBinding[]>([])
const loading = ref(false)
const newDomain = ref('')
const adding = ref(false)

// Current node domain from knode_connections (legacy field)
const currentNodeDomain = ref('')

// Domains sorted by position
const sortedDomains = computed(() => {
  return [...bindings.value]
    .filter(b => b.protocol === 'openvpn-udp')
    .sort((a, b) => a.position - b.position)
})

// ─── Load Data ───────────────────────────────────────────────────────────────
async function loadBindings() {
  loading.value = true
  try {
    const fetched = await store.fetchBindings(props.nodeId)
    bindings.value = fetched
  } finally {
    loading.value = false
  }
}

async function loadCurrentDomain() {
  try {
    const res = await get<any>(`/api/admin/knode/nodes/${props.nodeId}`)
    if (res && res.node) {
      currentNodeDomain.value = res.node.domain || ''
    }
  } catch { /* ignore */ }
}

onMounted(() => {
  loadBindings()
  loadCurrentDomain()
  store.fetchDomains()
})

// ─── Add Domain ──────────────────────────────────────────────────────────────
async function addDomain() {
  const name = newDomain.value.trim().toLowerCase()
  if (!name) {
    toast.warning('Enter a domain name')
    return
  }
  if (!isValidDomain(name)) {
    toast.warning('Invalid domain name (e.g. vpn.example.com)')
    return
  }

  adding.value = true

  // Create the domain (API returns existing if duplicate)
  let domainId: number | null = null
  const created = await store.createDomain({ name, ip_address: '0.0.0.0' })
  if (created) {
    domainId = created.id
  } else {
    // Domain might already exist — fetch list and find it
    await store.fetchDomains()
    const existing = store.domains.find(d => d.name === name)
    if (existing) {
      domainId = existing.id
    }
  }

  if (!domainId) {
    toast.error('Failed to register domain')
    adding.value = false
    return
  }

  // Create the binding (openvpn-udp as the primary protocol for configs)
  const nextPosition = sortedDomains.value.length + 1
  const success = await store.createBinding(props.nodeId, {
    protocol: 'openvpn-udp',
    domain_id: domainId,
    position: nextPosition,
  })

  adding.value = false

  if (success) {
    toast.success(`Domain "${name}" added`)
    newDomain.value = ''
    await loadBindings()
  } else {
    toast.error('Failed to add domain — it may already be assigned')
  }
}

// ─── Remove Domain ───────────────────────────────────────────────────────────
async function removeDomain(binding: ProtocolBinding) {
  const confirmed = await confirm({
    title: 'Remove Domain',
    message: `Remove "${binding.domain_name}" from this node? Configs will fall back to the next domain or the node's raw IP.`,
    variant: 'danger',
    confirmText: 'Remove',
    cancelText: 'Cancel',
  })
  if (!confirmed) return

  const success = await store.deleteBinding(props.nodeId, binding.id)
  if (success) {
    toast.success(`Removed "${binding.domain_name}"`)
    await loadBindings()
  } else {
    toast.error('Failed to remove domain')
  }
}

// ─── Move Up/Down ────────────────────────────────────────────────────────────
async function moveUp(index: number) {
  if (index === 0) return
  const ids = sortedDomains.value.map(b => b.id)
  // Swap
  ;[ids[index - 1], ids[index]] = [ids[index], ids[index - 1]]
  await store.reorderBindings(props.nodeId, { binding_ids: ids })
  await loadBindings()
}

async function moveDown(index: number) {
  if (index >= sortedDomains.value.length - 1) return
  const ids = sortedDomains.value.map(b => b.id)
  ;[ids[index], ids[index + 1]] = [ids[index + 1], ids[index]]
  await store.reorderBindings(props.nodeId, { binding_ids: ids })
  await loadBindings()
}

// ─── Validation ──────────────────────────────────────────────────────────────
function isValidDomain(name: string): boolean {
  if (!name || name.length > 253) return false
  const labels = name.split('.')
  if (labels.length < 2) return false
  for (const label of labels) {
    if (label.length === 0 || label.length > 63) return false
    if (!/^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/.test(label)) return false
  }
  return true
}
</script>

<template>
  <div class="node-domains-tab">
    <div class="tab-header">
      <div>
        <h3 class="tab-title">Domains</h3>
        <p class="tab-subtitle">
          Domains used in VPN configs. Position 1 = primary, rest = backup (failover order).
          DNS is managed in Cloudflare.
        </p>
      </div>
    </div>

    <!-- Current node domain (legacy info) -->
    <div v-if="currentNodeDomain" class="legacy-domain">
      <span class="legacy-domain__label">Current node domain:</span>
      <code class="legacy-domain__value">{{ currentNodeDomain }}</code>
    </div>

    <!-- Add Domain Form -->
    <div class="add-domain-form">
      <KInput
        v-model="newDomain"
        placeholder="Enter domain name (e.g. tr.koris.space)"
        @keyup.enter="addDomain"
      />
      <KButton variant="primary" :loading="adding" @click="addDomain">
        Add
      </KButton>
    </div>

    <!-- Loading -->
    <div v-if="loading" class="loading-state">Loading domains...</div>

    <!-- Domain List -->
    <div v-else-if="sortedDomains.length > 0" class="domain-list">
      <div
        v-for="(binding, index) in sortedDomains"
        :key="binding.id"
        class="domain-item"
      >
        <div class="domain-item__position">
          <span class="position-badge" :class="{ 'position-badge--primary': index === 0 }">
            {{ index === 0 ? '★' : index + 1 }}
          </span>
        </div>

        <div class="domain-item__info">
          <span class="domain-item__name">{{ binding.domain_name }}</span>
          <span class="domain-item__role">{{ index === 0 ? 'Primary' : 'Backup' }}</span>
        </div>

        <div class="domain-item__actions">
          <KButton
            v-if="index > 0"
            variant="ghost"
            size="sm"
            @click="moveUp(index)"
          >
            ↑
          </KButton>
          <KButton
            v-if="index < sortedDomains.length - 1"
            variant="ghost"
            size="sm"
            @click="moveDown(index)"
          >
            ↓
          </KButton>
          <KButton
            variant="ghost"
            size="sm"
            class="remove-btn"
            @click="removeDomain(binding)"
          >
            ✕
          </KButton>
        </div>
      </div>
    </div>

    <!-- Empty State -->
    <KEmptyState
      v-else-if="!loading"
      icon="🌐"
      title="No domains assigned"
      description="Add a domain to use in VPN configs instead of the node's raw IP. Clients will connect via the domain name."
    />
  </div>
</template>

<style scoped>
.node-domains-tab {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.tab-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
}

.tab-title {
  margin: 0;
  font-size: var(--text-lg);
  font-weight: var(--font-semibold);
}

.tab-subtitle {
  margin: 4px 0 0;
  font-size: var(--text-sm);
  color: var(--color-muted);
}

/* ─── Legacy Domain ─── */
.legacy-domain {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-3);
  background: var(--color-surface-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
}

.legacy-domain__label {
  color: var(--color-muted);
}

.legacy-domain__value {
  font-family: var(--font-mono, monospace);
  color: var(--color-text);
}

/* ─── Add Form ─── */
.add-domain-form {
  display: flex;
  gap: var(--space-2);
  align-items: center;
}

/* ─── Loading ─── */
.loading-state {
  padding: var(--space-4);
  text-align: center;
  color: var(--color-muted);
  font-size: var(--text-sm);
}

/* ─── Domain List ─── */
.domain-list {
  display: flex;
  flex-direction: column;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  overflow: hidden;
}

.domain-item {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4);
  border-bottom: 1px solid var(--color-border);
}

.domain-item:last-child {
  border-bottom: none;
}

.domain-item:hover {
  background: var(--color-surface-2);
}

/* ─── Position Badge ─── */
.domain-item__position {
  flex-shrink: 0;
}

.position-badge {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  font-size: var(--text-xs);
  font-weight: var(--font-semibold);
  background: var(--color-surface-2);
  border: 1px solid var(--color-border);
  color: var(--color-muted);
}

.position-badge--primary {
  background: rgba(34, 197, 94, 0.12);
  border-color: rgba(34, 197, 94, 0.3);
  color: var(--color-success);
}

/* ─── Domain Info ─── */
.domain-item__info {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.domain-item__name {
  font-family: var(--font-mono, monospace);
  font-size: var(--text-base);
  font-weight: var(--font-medium);
  color: var(--color-text);
}

.domain-item__role {
  font-size: var(--text-xs);
  color: var(--color-muted);
}

/* ─── Actions ─── */
.domain-item__actions {
  display: flex;
  gap: var(--space-1);
  flex-shrink: 0;
}

.remove-btn {
  color: var(--color-danger, #ef4444);
}
</style>
