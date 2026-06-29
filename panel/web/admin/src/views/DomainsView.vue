<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useDomainsStore, type VpnDomain } from '@/stores/domains'
import { useToast } from '@koris/composables/useToast'
import { useConfirm } from '@koris/composables/useConfirm'
import KDataTable from '@koris/ui/KDataTable.vue'
import KButton from '@koris/ui/KButton.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'
import KInput from '@koris/ui/KInput.vue'

const store = useDomainsStore()
const toast = useToast()
const { confirm } = useConfirm()

// ─── State ───────────────────────────────────────────────────────────────────
const showAddForm = ref(false)
const newDomainName = ref('')
const adding = ref(false)

const editingId = ref<number | null>(null)
const editName = ref('')

// ─── Table ───────────────────────────────────────────────────────────────────
const columns = computed(() => [
  { key: 'name', label: 'Domain', sortable: true },
  { key: 'created_at', label: 'Added', sortable: true, width: '180px' },
  { key: 'actions', label: '', sortable: false, width: '160px' },
])

// ─── Add Domain ──────────────────────────────────────────────────────────────
async function addDomain() {
  const name = newDomainName.value.trim().toLowerCase()
  if (!name) {
    toast.warning('Enter a domain name')
    return
  }
  if (!isValidDomain(name)) {
    toast.warning('Invalid domain name')
    return
  }

  adding.value = true
  // API still requires ip_address — pass a placeholder since DNS is managed in Cloudflare
  const result = await store.createDomain({ name, ip_address: '0.0.0.0' })
  adding.value = false

  if (result) {
    toast.success(`Domain "${name}" added`)
    newDomainName.value = ''
    showAddForm.value = false
  } else {
    toast.error('Failed to add domain')
  }
}

// ─── Edit Domain ─────────────────────────────────────────────────────────────
function startEdit(domain: VpnDomain) {
  editingId.value = domain.id
  editName.value = domain.name
}

function cancelEdit() {
  editingId.value = null
  editName.value = ''
}

async function saveEdit(domain: VpnDomain) {
  const name = editName.value.trim().toLowerCase()
  if (!name || name === domain.name) {
    cancelEdit()
    return
  }
  if (!isValidDomain(name)) {
    toast.warning('Invalid domain name')
    return
  }

  // For now, delete old and create new (API doesn't support name rename directly)
  const deleted = await store.deleteDomain(domain.id)
  if (!deleted) {
    toast.error('Failed to update domain')
    cancelEdit()
    return
  }
  const created = await store.createDomain({ name, ip_address: '0.0.0.0' })
  if (created) {
    toast.success(`Domain updated to "${name}"`)
  } else {
    toast.error('Failed to recreate domain — old domain was removed')
  }
  cancelEdit()
}

// ─── Delete Domain ───────────────────────────────────────────────────────────
async function removeDomain(domain: VpnDomain) {
  const confirmed = await confirm({
    title: 'Delete Domain',
    message: `Delete "${domain.name}"? Configs referencing this domain will fall back to the node IP.`,
    variant: 'danger',
    confirmText: 'Delete',
    cancelText: 'Cancel',
  })
  if (!confirmed) return

  const success = await store.deleteDomain(domain.id)
  if (success) {
    toast.success(`Domain "${domain.name}" deleted`)
  } else {
    toast.error('Failed to delete domain — it may have active bindings')
  }
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

function formatDate(dateStr: string): string {
  if (!dateStr) return ''
  return new Date(dateStr).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
}

// ─── Lifecycle ───────────────────────────────────────────────────────────────
onMounted(() => {
  store.fetchDomains()
})
</script>

<template>
  <div class="page domains-view">
    <header class="page-header">
      <div class="page-header__title">
        <h2>Domains</h2>
        <p class="page-header__subtitle">Domains used in VPN configs instead of raw IPs. DNS is managed in Cloudflare.</p>
      </div>
      <KButton variant="primary" icon="+" @click="showAddForm = !showAddForm">
        Add Domain
      </KButton>
    </header>

    <!-- Add Domain Inline Form -->
    <div v-if="showAddForm" class="add-form">
      <KInput
        v-model="newDomainName"
        placeholder="vpn.example.com"
        @keyup.enter="addDomain"
      />
      <KButton variant="primary" :loading="adding" @click="addDomain">
        Add
      </KButton>
      <KButton variant="ghost" @click="showAddForm = false; newDomainName = ''">
        Cancel
      </KButton>
    </div>

    <!-- Domains Table -->
    <KDataTable
      v-if="store.domains.length > 0 || store.loading"
      :columns="columns"
      :data="store.domains"
      :loading="store.loading"
      row-key="id"
    >
      <!-- Domain Name (editable) -->
      <template #cell-name="{ row }">
        <div v-if="editingId === row.id" class="edit-cell">
          <KInput
            v-model="editName"
            size="sm"
            @keyup.enter="saveEdit(row)"
            @keyup.escape="cancelEdit"
          />
          <KButton variant="primary" size="sm" @click="saveEdit(row)">Save</KButton>
          <KButton variant="ghost" size="sm" @click="cancelEdit">Cancel</KButton>
        </div>
        <span v-else class="domain-name">{{ row.name }}</span>
      </template>

      <!-- Added Date -->
      <template #cell-created_at="{ value }">
        <span class="date-text">{{ formatDate(value) }}</span>
      </template>

      <!-- Row Actions -->
      <template #cell-actions="{ row }">
        <div v-if="editingId !== row.id" class="actions-cell">
          <KButton variant="ghost" size="sm" @click="startEdit(row)">
            Edit
          </KButton>
          <KButton variant="ghost" size="sm" class="delete-btn" @click="removeDomain(row)">
            Delete
          </KButton>
        </div>
      </template>
    </KDataTable>

    <KEmptyState
      v-if="!store.loading && store.domains.length === 0 && !showAddForm"
      icon="🌐"
      title="No Domains"
      description="Add a domain to use in VPN configs instead of raw IPs."
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

.page-header__title h2 {
  margin: 0;
  font-size: var(--text-xl);
  font-weight: var(--font-semibold);
}

.page-header__subtitle {
  margin: 4px 0 0;
  font-size: var(--text-sm);
  color: var(--color-muted);
}

/* ─── Add Form ─── */
.add-form {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
  background: var(--color-surface-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
}

/* ─── Edit Cell ─── */
.edit-cell {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

/* ─── Domain Name ─── */
.domain-name {
  font-weight: var(--font-medium);
  font-family: var(--font-mono, monospace);
}

/* ─── Date ─── */
.date-text {
  font-size: var(--text-sm);
  color: var(--color-muted);
}

/* ─── Actions ─── */
.actions-cell {
  display: flex;
  gap: var(--space-1);
}

.delete-btn {
  color: var(--color-danger, #ef4444);
}
</style>
