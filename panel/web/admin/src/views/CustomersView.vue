<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useCustomersStore } from '@/stores/customers'
import { useRealtimeStore } from '@/stores/realtime'
import type { BulkActionRequest } from '@/stores/customers'
import KDataTable from '@koris/ui/KDataTable.vue'
import KButton from '@koris/ui/KButton.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KAvatar from '@koris/ui/KAvatar.vue'
import KInput from '@koris/ui/KInput.vue'
import { useDebounceFn } from '@vueuse/core'
import { useConfirm } from '@koris/composables/useConfirm'
import { useToast } from '@koris/composables/useToast'
import { useI18n } from '@koris/composables/useI18n'

const { t } = useI18n()
const router = useRouter()
const store = useCustomersStore()
const realtime = useRealtimeStore()
const { confirm } = useConfirm()
const toast = useToast()

const searchQuery = ref('')
const activeStatusTab = ref<string>('all')
const currentMainTab = ref<string>('customers')

/** Tracks selected customer IDs for bulk actions */
const selectedIds = ref<number[]>([])

/** Whether all currently displayed rows are selected */
const isAllSelected = computed(() => {
  if (tableData.value.length === 0) return false
  return tableData.value.every((c: any) => selectedIds.value.includes(c.id))
})

/** Whether at least one customer is selected (controls toolbar visibility) */
const hasSelection = computed(() => selectedIds.value.length > 0)

/** Page-level navigation tabs: Customers | Archived | Resellers */
const mainTabs = computed(() => [
  { key: 'customers', label: t('customers.tab_customers') },
  { key: 'archived', label: t('customers.tab_archived') },
  { key: 'resellers', label: t('customers.tab_resellers') },
])

/** Status filter tabs (only shown when main tab is "customers") */
const statusTabs = computed(() => [
  { key: 'all', label: t('customers.all') },
  { key: 'active', label: t('customers.active') },
  { key: 'online', label: t('customers.online') },
  { key: 'limited', label: t('customers.limited') },
  { key: 'disabled', label: t('customers.disabled') },
  { key: 'expired', label: t('customers.expired') },
])

const columns = computed(() => [
  { key: 'username', label: t('user.username'), sortable: true },
  { key: 'display_name', label: t('user.display_name'), sortable: true },
  { key: 'status', label: t('user.status'), sortable: true },
  { key: 'plan', label: t('user.plan'), sortable: true },
  { key: 'credit', label: t('user.balance'), sortable: true, align: 'right' as const },
  { key: 'created_at', label: t('user.created'), sortable: true },
  { key: 'actions', label: '', sortable: false, align: 'center' as const, width: '80px' },
])

/** Set of usernames currently online (from live sessions) */
const onlineUsernames = computed(() => {
  return new Set(realtime.liveSessions.map((s) => s.username))
})

/** The data shown in the table depends on which main tab + status filter is active */
const tableData = computed(() => {
  if (currentMainTab.value === 'archived') {
    // Show deleted/archived customers
    const query = searchQuery.value.trim().toLowerCase()
    if (!query) return store.deleted
    return store.deleted.filter(
      (c: any) =>
        c.username.toLowerCase().includes(query) ||
        c.display_name.toLowerCase().includes(query)
    )
  }

  // For "customers" main tab, apply status filter
  if (activeStatusTab.value === 'online') {
    // Filter by live session presence (real online state)
    return store.paginatedList.filter((c: any) => onlineUsernames.value.has(c.username))
  }

  return store.paginatedList
})

const debouncedSearch = useDebounceFn((val: string) => {
  store.filters.search = val
}, 300)

function onSearchInput(val: string | number) {
  const strVal = String(val)
  searchQuery.value = strVal
  debouncedSearch(strVal)
}

function setMainTab(tabKey: string) {
  if (tabKey === 'resellers') {
    router.push({ name: 'resellers' })
    return
  }
  currentMainTab.value = tabKey
  // Reset status filter when switching main tabs
  if (tabKey === 'archived') {
    store.filters.status = 'all'
    activeStatusTab.value = 'all'
  }
}

function setStatusFilter(status: string) {
  activeStatusTab.value = status
  if (status === 'online') {
    // "Online" uses live session data, so set store filter to 'all' and filter client-side
    store.filters.status = 'all'
  } else {
    store.filters.status = status as any
  }
  store.pagination.page = 1
}

function handleRowClick(row: any) {
  router.push({ name: 'customer-detail', params: { id: String(row.id) } })
}

function handleNewUser() {
  router.push({ name: 'customer-detail', params: { id: 'new' } })
}

/**
 * Delete a single customer with confirmation dialog.
 */
async function deleteCustomer(id: number, username: string) {
  const confirmed = await confirm({
    title: t('customers.confirm_delete_title'),
    message: t('customers.confirm_delete_msg').replace('{name}', username),
    variant: 'danger',
    icon: '\u26A0',
    confirmText: t('btn.delete'),
    cancelText: t('btn.cancel'),
  })
  if (!confirmed) return
  const success = await store.archiveCustomer(id)
  if (success) {
    toast.success(t('customers.deleted_success').replace('{name}', username))
  } else {
    toast.error(t('customers.deleted_error').replace('{name}', username))
  }
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
 * Execute a bulk action. For delete, shows a confirmation dialog first.
 * Displays toast with success/failure summary from BulkActionResponse.
 */
async function executeBulkAction(action: BulkActionRequest['action']) {
  if (selectedIds.value.length === 0) return

  if (action === 'delete') {
    const confirmed = await confirm({
      title: t('customers.confirm_delete_title'),
      message: t('customers.confirm_delete_msg').replace('{name}', String(selectedIds.value.length)),
      variant: 'danger',
      icon: '\u26A0',
      confirmText: t('btn.delete'),
      cancelText: t('btn.cancel'),
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

    if (failedCount === 0) {
      toast.success(t('customers.bulk_success').replace('{count}', String(succeededCount)))
    } else if (succeededCount === 0) {
      toast.error(t('customers.bulk_error').replace('{count}', String(failedCount)))
    } else {
      toast.warning(t('customers.bulk_partial').replace('{succeeded}', String(succeededCount)).replace('{failed}', String(failedCount)))
    }
    clearSelection()
  } else {
    toast.error(t('customers.bulk_error').replace('{count}', String(selectedIds.value.length)))
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
      <KButton variant="primary" icon="+" @click="handleNewUser">{{ t('customers.new_user') }}</KButton>
    </header>

    <!-- Page-level sub-tab navigation: Customers | Archived | Resellers -->
    <nav class="main-tabs" aria-label="Customer section navigation">
      <button
        v-for="tab in mainTabs"
        :key="tab.key"
        :class="['main-tab', { 'main-tab--active': currentMainTab === tab.key }]"
        @click="setMainTab(tab.key)"
      >
        {{ tab.label }}
      </button>
    </nav>

    <!-- Bulk Action Toolbar (visible when selection > 0) -->
    <Transition name="bulk-toolbar">
      <div v-if="hasSelection" class="bulk-toolbar" role="toolbar" aria-label="Bulk actions">
        <span class="bulk-toolbar__count">{{ selectedIds.length }} {{ t('customers.selected') }}</span>
        <div class="bulk-toolbar__actions">
          <KButton variant="ghost" size="sm" @click="executeBulkAction('enable')">{{ t('customers.enable') }}</KButton>
          <KButton variant="ghost" size="sm" @click="executeBulkAction('disable')">{{ t('customers.disable') }}</KButton>
          <KButton variant="ghost" size="sm" @click="executeBulkAction('traffic_reset')">{{ t('customers.traffic_reset') }}</KButton>
          <KButton variant="danger" size="sm" @click="executeBulkAction('delete')">{{ t('customers.delete') }}</KButton>
        </div>
        <button class="bulk-toolbar__clear" @click="clearSelection" :aria-label="t('customers.clear')">{{ t('customers.clear') }}</button>
      </div>
    </Transition>

    <!-- Filter Row: Status tabs + Search on same line -->
    <div v-if="currentMainTab === 'customers'" class="filter-row">
      <nav class="status-tabs" aria-label="Customer status filter">
        <button
          v-for="tab in statusTabs"
          :key="tab.key"
          :class="['status-tab', { 'status-tab--active': activeStatusTab === tab.key }]"
          @click="setStatusFilter(tab.key)"
        >
          {{ tab.label }}
        </button>
      </nav>
      <div class="filter-row__search">
        <KInput
          :model-value="searchQuery"
          :placeholder="t('customers.search')"
          aria-label="Search customers"
          @update:model-value="onSearchInput"
        />
      </div>
    </div>

    <!-- Archived header + search (when viewing archived tab) -->
    <div v-if="currentMainTab === 'archived'" class="archived-section">
      <p class="archived-description">{{ t('customers.archived_desc') }}</p>
      <div class="filter-row__search">
        <KInput
          :model-value="searchQuery"
          :placeholder="t('customers.search_archived')"
          aria-label="Search archived customers"
          @update:model-value="onSearchInput"
        />
      </div>
    </div>

    <!-- Data Table -->
    <KDataTable
      :columns="columns"
      :data="tableData"
      :loading="store.loading"
      :page-size="store.pagination.pageSize"
      row-key="id"
      selectable
      @row-click="handleRowClick"
      @selection-change="onSelectionChange"
    >
      <!-- Username cell with avatar -->
      <template #cell-username="{ row, value }">
        <div class="username-cell">
          <KAvatar :name="row.display_name || value" size="sm" />
          <span class="username-cell__text">{{ value }}</span>
          <span v-if="onlineUsernames.has(value)" class="online-dot" title="Online" />
        </div>
      </template>
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
      <!-- Actions column with per-row delete button -->
      <template #cell-actions="{ row }">
        <button
          class="action-btn action-btn--delete"
          :title="t('btn.delete')"
          :aria-label="t('btn.delete')"
          @click.stop="deleteCustomer(row.id, row.username)"
        >
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path d="M2 4h12M5.333 4V2.667a1.333 1.333 0 011.334-1.334h2.666a1.333 1.333 0 011.334 1.334V4m2 0v9.333a1.333 1.333 0 01-1.334 1.334H4.667a1.333 1.333 0 01-1.334-1.334V4h9.334z" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round"/>
            <path d="M6.667 7.333v4M9.333 7.333v4" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
        </button>
      </template>
    </KDataTable>
  </div>
</template>

<style scoped>
.customers-view { display: flex; flex-direction: column; gap: var(--space-4); }

.page-header { display: flex; align-items: center; justify-content: flex-end; }

/* Main page-level tabs (Customers | Archived | Resellers) */
.main-tabs {
  display: flex;
  gap: 0;
  border-bottom: 2px solid var(--color-border);
}

.main-tab {
  padding: var(--space-3) var(--space-4);
  border: none;
  background: none;
  color: var(--color-muted);
  font-size: var(--text-sm);
  font-weight: var(--font-semibold);
  cursor: pointer;
  border-bottom: 2px solid transparent;
  margin-bottom: -2px;
  transition: all var(--duration-fast);
}

.main-tab:hover {
  color: var(--color-text);
}

.main-tab--active {
  color: var(--color-primary);
  border-bottom-color: var(--color-primary);
}

/* Bulk Action Toolbar */
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

/* Filter row: status tabs + search side by side */
.filter-row {
  display: flex;
  align-items: center;
  gap: var(--space-4);
}

.filter-row__search {
  flex-shrink: 0;
  width: 240px;
}

/* Status filter tabs - compact pill style */
.status-tabs {
  display: flex;
  gap: var(--space-1);
  overflow-x: auto;
  flex: 1;
  min-width: 0;
}

.status-tab {
  padding: var(--space-1) var(--space-3);
  border: none;
  background: none;
  color: var(--color-muted);
  font-size: var(--text-xs);
  font-weight: var(--font-medium);
  cursor: pointer;
  border-radius: 9999px;
  white-space: nowrap;
  transition: all var(--duration-fast);
}

.status-tab:hover {
  color: var(--color-text);
  background: var(--color-surface-2);
}

.status-tab--active {
  color: var(--color-primary);
  background: rgba(37, 99, 235, 0.1);
  font-weight: var(--font-semibold);
}

/* Archived section */
.archived-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.archived-description {
  margin: 0;
  font-size: var(--text-sm);
  color: var(--color-muted);
}

/* Compact table rows */
:deep(tbody td) {
  padding: 8px 12px;
}

/* Username cell with avatar */
.username-cell {
  display: flex;
  align-items: center;
  gap: 6px;
}

.username-cell__text {
  font-weight: var(--font-medium);
}

.online-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--color-success, #22c55e);
  flex-shrink: 0;
  animation: pulse-dot 2s infinite;
}

@keyframes pulse-dot {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

/* Per-row action buttons */
.action-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border: none;
  background: none;
  border-radius: var(--radius-sm);
  cursor: pointer;
  color: var(--color-muted);
  transition: all var(--duration-fast);
}

.action-btn:hover {
  background: var(--color-surface-2);
}

.action-btn--delete:hover {
  color: var(--color-danger, #ef4444);
  background: rgba(239, 68, 68, 0.1);
}

/* Checkbox styling override for KDataTable selectable */
:deep(.k-table__checkbox) {
  appearance: none;
  -webkit-appearance: none;
  width: 14px;
  height: 14px;
  border: 2px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-surface);
  cursor: pointer;
  position: relative;
  flex-shrink: 0;
  transition: all var(--duration-fast);
}

:deep(.k-table__checkbox:hover) {
  border-color: var(--color-primary);
}

:deep(.k-table__checkbox:checked) {
  background: var(--color-primary);
  border-color: var(--color-primary);
}

:deep(.k-table__checkbox:checked::after) {
  content: '';
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  border: solid #fff;
  border-width: 0 2px 2px 0;
  width: 25%;
  height: 50%;
  margin: auto;
  transform: rotate(45deg) translateY(-1px);
}

:deep(.k-table__checkbox:indeterminate) {
  background: var(--color-primary);
  border-color: var(--color-primary);
}

:deep(.k-table__checkbox:indeterminate::after) {
  content: '';
  position: absolute;
  top: 50%;
  left: 50%;
  width: 50%;
  height: 2px;
  background: #fff;
  border-radius: 1px;
  transform: translate(-50%, -50%);
}

:deep(.k-table__checkbox:focus-visible) {
  outline: 2px solid var(--color-primary);
  outline-offset: 2px;
}

.text-success { color: var(--color-success, #22c55e); }
.text-danger { color: var(--color-danger, #ef4444); }

/* Responsive: stack filter row on mobile */
@media (max-width: 640px) {
  .filter-row {
    flex-direction: column;
    align-items: stretch;
    gap: var(--space-3);
  }

  .filter-row__search {
    width: 100%;
  }

  .status-tabs {
    padding-bottom: var(--space-2);
  }

  .bulk-toolbar {
    flex-wrap: wrap;
  }

  .bulk-toolbar__actions {
    flex: 1 1 100%;
    order: 3;
  }
}

@media (prefers-reduced-motion: reduce) {
  .bulk-toolbar-enter-active,
  .bulk-toolbar-leave-active {
    transition: opacity var(--duration-fast) var(--ease-default);
  }
  .bulk-toolbar-enter-from,
  .bulk-toolbar-leave-to {
    transform: none;
  }
  .online-dot {
    animation: none;
  }
}

/* RTL support */
[dir="rtl"] .page-header { flex-direction: row-reverse; }
[dir="rtl"] .filter-row { flex-direction: row-reverse; }
[dir="rtl"] .status-tabs { flex-direction: row-reverse; }
[dir="rtl"] .bulk-toolbar { flex-direction: row-reverse; }
[dir="rtl"] .bulk-toolbar__actions { flex-direction: row-reverse; }
[dir="rtl"] .bulk-toolbar__clear { margin-left: 0; margin-right: auto; }
[dir="rtl"] .main-tabs { flex-direction: row-reverse; }
[dir="rtl"] .username-cell { flex-direction: row-reverse; }
[dir="rtl"] .username-cell__text { text-align: right; }
[dir="rtl"] .archived-description { text-align: right; }
</style>
