<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useCustomersStore } from '@/stores/customers'
import type { BulkActionRequest } from '@/stores/customers'
import KDataTable from '@koris/ui/KDataTable.vue'
import KButton from '@koris/ui/KButton.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import { useDebounceFn } from '@vueuse/core'
import { useConfirm } from '@koris/composables/useConfirm'
import { useToast } from '@koris/composables/useToast'

const router = useRouter()
const store = useCustomersStore()
const { confirm } = useConfirm()
const toast = useToast()

const searchQuery = ref('')
const activeTab = ref<string>('all')

/** Tracks selected customer IDs for bulk actions (Requirement 2.1) */
const selectedIds = ref<number[]>([])

/** Whether all currently displayed rows are selected */
const isAllSelected = computed(() => {
  if (store.paginatedList.length === 0) return false
  return store.paginatedList.every((c: any) => selectedIds.value.includes(c.id))
})

/** Whether at least one customer is selected (controls toolbar visibility) */
const hasSelection = computed(() => selectedIds.value.length > 0)

const statusTabs = [
  { key: 'all', label: 'All' },
  { key: 'active', label: 'Online' },
  { key: 'limited', label: 'Limited' },
  { key: 'disabled', label: 'Disabled' },
  { key: 'expired', label: 'Expired' },
  { key: 'archived', label: 'Archived' },
]

const columns = [
  { key: 'username', label: 'Username', sortable: true },
  { key: 'display_name', label: 'Display Name', sortable: true },
  { key: 'status', label: 'Status', sortable: true },
  { key: 'plan', label: 'Plan', sortable: true },
  { key: 'credit', label: 'Credit', sortable: true, align: 'right' as const },
  { key: 'created_at', label: 'Created', sortable: true },
]

const debouncedSearch = useDebounceFn((val: string) => {
  store.filters.search = val
}, 300)

function onSearchInput(e: Event) {
  const val = (e.target as HTMLInputElement).value
  searchQuery.value = val
  debouncedSearch(val)
}

function setStatusFilter(status: string) {
  activeTab.value = status
  store.filters.status = status as any
  store.pagination.page = 1
}

function handleRowClick(row: any) {
  router.push({ name: 'customer-detail', params: { id: String(row.id) } })
}

function handleNewUser() {
  router.push({ name: 'customer-detail', params: { id: 'new' } })
}

/**
 * Handle selection change from KDataTable.
 * Extracts IDs from selected row objects.
 */
function onSelectionChange(rows: any[]) {
  selectedIds.value = rows.map((r) => r.id)
}

/**
 * Clear the current selection (e.g., after a bulk action completes).
 */
function clearSelection() {
  selectedIds.value = []
}

/**
 * Execute a bulk action. For delete, shows a confirmation dialog first (Requirement 2.6).
 * Displays toast with success/failure summary from BulkActionResponse (Requirement 2.7).
 */
async function executeBulkAction(action: BulkActionRequest['action']) {
  if (selectedIds.value.length === 0) return

  // For delete actions, show confirmation dialog with count (Requirement 2.6)
  if (action === 'delete') {
    const confirmed = await confirm({
      title: 'Delete Customers',
      message: `Are you sure you want to delete ${selectedIds.value.length} customer${selectedIds.value.length > 1 ? 's' : ''}? This action cannot be undone.`,
      variant: 'danger',
      icon: '⚠',
      confirmText: 'Delete',
      cancelText: 'Cancel',
    })
    if (!confirmed) return
  }

  const request: BulkActionRequest = {
    customer_ids: [...selectedIds.value],
    action,
  }

  const response = await store.bulkAction(request)

  if (response) {
    const succeededCount = response.succeeded.length
    const failedCount = response.failed.length
    const actionLabel = action === 'traffic_reset' ? 'Traffic Reset' : action.charAt(0).toUpperCase() + action.slice(1)

    if (failedCount === 0) {
      toast.success(`${actionLabel}: ${succeededCount} customer${succeededCount > 1 ? 's' : ''} updated successfully.`)
    } else if (succeededCount === 0) {
      toast.error(`${actionLabel} failed for all ${failedCount} customer${failedCount > 1 ? 's' : ''}.`)
    } else {
      toast.warning(`${actionLabel}: ${succeededCount} succeeded, ${failedCount} failed.`)
    }
    clearSelection()
  } else {
    toast.error('Bulk action failed. Please try again.')
  }
}

onMounted(() => {
  store.loadCustomers()
})
</script>

<template>
  <div class="page customers-view">
    <!-- Header -->
    <header class="page-header">
      <h2 class="page-title">Customers</h2>
      <KButton variant="primary" icon="+" @click="handleNewUser">New User</KButton>
    </header>

    <!-- Bulk Action Toolbar (visible when selection > 0) — Requirement 2.1 -->
    <Transition name="bulk-toolbar">
      <div v-if="hasSelection" class="bulk-toolbar" role="toolbar" aria-label="Bulk actions">
        <span class="bulk-toolbar__count">{{ selectedIds.length }} selected</span>
        <div class="bulk-toolbar__actions">
          <KButton variant="ghost" size="sm" @click="executeBulkAction('enable')">Enable</KButton>
          <KButton variant="ghost" size="sm" @click="executeBulkAction('disable')">Disable</KButton>
          <KButton variant="ghost" size="sm" @click="executeBulkAction('traffic_reset')">Traffic Reset</KButton>
          <KButton variant="danger" size="sm" @click="executeBulkAction('delete')">Delete</KButton>
        </div>
        <button class="bulk-toolbar__clear" @click="clearSelection" aria-label="Clear selection">Clear</button>
      </div>
    </Transition>

    <!-- Status Tabs -->
    <nav class="status-tabs" aria-label="Customer status filter">
      <button
        v-for="tab in statusTabs"
        :key="tab.key"
        :class="['status-tab', { 'status-tab--active': activeTab === tab.key }]"
        @click="setStatusFilter(tab.key)"
      >
        {{ tab.label }}
      </button>
    </nav>

    <!-- Search -->
    <div class="search-bar">
      <input
        type="search"
        class="search-input"
        placeholder="Search customers..."
        :value="searchQuery"
        @input="onSearchInput"
        aria-label="Search customers"
      />
    </div>

    <!-- Data Table -->
    <KDataTable
      :columns="columns"
      :data="store.paginatedList"
      :loading="store.loading"
      :page-size="store.pagination.pageSize"
      row-key="id"
      selectable
      @row-click="handleRowClick"
      @selection-change="onSelectionChange"
    >
      <template #cell-status="{ value }">
        <KStatusPill :status="value" size="sm" />
      </template>
      <template #cell-credit="{ value }">
        <span :class="{ 'text-success': value > 0, 'text-danger': value < 0 }">
          ${{ typeof value === 'number' ? value.toFixed(2) : '0.00' }}
        </span>
      </template>
      <template #cell-created_at="{ value }">
        {{ value?.slice(0, 10) }}
      </template>
    </KDataTable>
  </div>
</template>

<style scoped>
.customers-view { display: flex; flex-direction: column; gap: var(--space-4); }

.page-header { display: flex; align-items: center; justify-content: space-between; }
.page-title { margin: 0; font-size: var(--text-xl); font-weight: var(--font-bold); }

/* Bulk Action Toolbar — Requirement 2.1 */
.bulk-toolbar {
  display: flex;
  align-items: center;
  gap: var(--space-4);
  padding: var(--space-3) var(--space-4);
  background: rgba(37, 99, 235, 0.08);
  border: 1px solid rgba(37, 99, 235, 0.2);
  border-radius: var(--radius-md);
}

.bulk-toolbar__count {
  font-size: var(--text-sm);
  font-weight: var(--font-semibold);
  color: var(--color-primary);
  white-space: nowrap;
}

.bulk-toolbar__actions {
  display: flex;
  gap: var(--space-2);
  flex-wrap: wrap;
}

.bulk-toolbar__clear {
  margin-left: auto;
  padding: var(--space-1) var(--space-2);
  border: none;
  background: none;
  color: var(--color-muted);
  font-size: var(--text-xs);
  cursor: pointer;
  border-radius: var(--radius-sm);
  transition: color var(--duration-fast), background var(--duration-fast);
}

.bulk-toolbar__clear:hover {
  color: var(--color-text);
  background: var(--color-surface-2);
}

/* Toolbar enter/leave transition */
.bulk-toolbar-enter-active,
.bulk-toolbar-leave-active {
  transition: opacity var(--duration-normal) var(--ease-out),
              transform var(--duration-normal) var(--ease-out);
}

.bulk-toolbar-enter-from,
.bulk-toolbar-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}

.status-tabs { display: flex; gap: var(--space-1); border-bottom: 1px solid var(--color-border); padding-bottom: var(--space-2); overflow-x: auto; }
.status-tab { padding: var(--space-2) var(--space-3); border: none; background: none; color: var(--color-muted); font-size: var(--text-sm); font-weight: var(--font-medium); cursor: pointer; border-radius: var(--radius-sm); transition: all var(--duration-fast); }
.status-tab:hover { color: var(--color-text); background: var(--color-surface-2); }
.status-tab--active { color: var(--color-primary); background: rgba(37, 99, 235, 0.1); }

.search-bar { max-width: 320px; }
.search-input { width: 100%; padding: var(--space-2) var(--space-3); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-md); color: var(--color-text); font-size: var(--text-sm); outline: none; transition: border-color var(--duration-normal); }
.search-input:focus { border-color: var(--color-primary); }

.text-success { color: var(--color-success); }
.text-danger { color: var(--color-danger); }

@media (prefers-reduced-motion: reduce) {
  .bulk-toolbar-enter-active,
  .bulk-toolbar-leave-active {
    transition: opacity var(--duration-fast) var(--ease-default);
  }
  .bulk-toolbar-enter-from,
  .bulk-toolbar-leave-to {
    transform: none;
  }
}
</style>
