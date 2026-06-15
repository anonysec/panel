<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useCustomersStore } from '@/stores/customers'
import KDataTable from '@koris/ui/KDataTable.vue'
import KButton from '@koris/ui/KButton.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import { useDebounceFn } from '@vueuse/core'

const router = useRouter()
const store = useCustomersStore()

const searchQuery = ref('')
const activeTab = ref<string>('all')

const statusTabs = [
  { key: 'all', label: 'All' },
  { key: 'active', label: 'Online' },
  { key: 'limited', label: 'Limited' },
  { key: 'disabled', label: 'Disabled' },
  { key: 'expired', label: 'Expired' },
  { key: 'archived', label: 'Archived' },
]

const columns = [
  { key: 'username', label: 'Username', sortable: true, filterable: true },
  { key: 'display_name', label: 'Display Name', sortable: true },
  { key: 'status', label: 'Status', sortable: true, filterable: true, filterType: 'select' as const, filterOptions: [
    { label: 'Active', value: 'active' },
    { label: 'Limited', value: 'limited' },
    { label: 'Disabled', value: 'disabled' },
    { label: 'Expired', value: 'expired' },
  ]},
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

.status-tabs { display: flex; gap: var(--space-1); border-bottom: 1px solid var(--color-border); padding-bottom: var(--space-2); overflow-x: auto; }
.status-tab { padding: var(--space-2) var(--space-3); border: none; background: none; color: var(--color-muted); font-size: var(--text-sm); font-weight: var(--font-medium); cursor: pointer; border-radius: var(--radius-sm); transition: all var(--duration-fast); }
.status-tab:hover { color: var(--color-text); background: var(--color-surface-2); }
.status-tab--active { color: var(--color-primary); background: rgba(37, 99, 235, 0.1); }

.search-bar { max-width: 320px; }
.search-input { width: 100%; padding: var(--space-2) var(--space-3); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-md); color: var(--color-text); font-size: var(--text-sm); outline: none; transition: border-color var(--duration-normal); }
.search-input:focus { border-color: var(--color-primary); }

.text-success { color: var(--color-success); }
.text-danger { color: var(--color-danger); }
</style>
