<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useCustomersStore } from '@/stores/customers'
import { useResellersStore } from '@/stores/resellers'
import { usePlansStore } from '@/stores/plans'
import { useRealtimeStore } from '@/stores/realtime'
import type { BulkActionRequest } from '@/stores/customers'
import KDataTable from '@koris/ui/KDataTable.vue'
import KButton from '@koris/ui/KButton.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KAvatar from '@koris/ui/KAvatar.vue'
import KInput from '@koris/ui/KInput.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KSelect from '@koris/ui/KSelect.vue'
import KSlideOver from '@koris/ui/KSlideOver.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'
import { useDebounceFn } from '@vueuse/core'
import { useConfirm } from '@koris/composables/useConfirm'
import { useToast } from '@koris/composables/useToast'
import { useI18n } from '@koris/composables/useI18n'
import { formatDate } from '@koris/composables/useFormatDate'

const { t } = useI18n()
const router = useRouter()
const store = useCustomersStore()
const resellersStore = useResellersStore()
const plansStore = usePlansStore()
const realtime = useRealtimeStore()
const { confirm } = useConfirm()
const toast = useToast()

const searchQuery = ref('')
const activeStatusTab = ref<string>('all')
const currentMainTab = ref<string>('users')

/** Tracks selected customer IDs for bulk actions */
const selectedIds = ref<number[]>([])

/** Whether all currently displayed rows are selected */
const isAllSelected = computed(() => {
  if (tableData.value.length === 0) return false
  return tableData.value.every((c: any) => selectedIds.value.includes(c.id))
})

/** Whether at least one customer is selected (controls toolbar visibility) */
const hasSelection = computed(() => selectedIds.value.length > 0)

// ─── Slide-Over State ───────────────────────────────────────────────────────
const showUserSlideOver = ref(false)
const showResellerSlideOver = ref(false)
const showCreditSlideOver = ref(false)
const editingResellerId = ref<number | null>(null)
const creditTarget = ref<{ id: number; username: string } | null>(null)
const saving = ref(false)

const userForm = ref({
  username: '',
  password: '',
  display_name: '',
  plan_id: '' as string | number,
  data_gb: '',
  speed_mbps: '',
  days: '',
  template_id: '' as string | number,
})

const resellerForm = ref({
  username: '',
  password: '',
  plan_id: '' as string | number,
})

const creditForm = ref({ amount: '' })

// ─── Plan Options ───────────────────────────────────────────────────────────
const planOptions = computed(() =>
  plansStore.activePlans.map((p) => ({
    value: String(p.id),
    label: `${p.name} (${p.data_gb}GB / ${p.duration_days}d — $${p.price})`,
  }))
)

const quotaPlanOptions = computed(() =>
  plansStore.list
    .filter((p) => p.is_active && (p.billing_type || 'quota') === 'quota')
    .map((p) => ({
      value: String(p.id),
      label: `${p.name} (${p.data_gb}GB / ${p.duration_days}d — $${p.price})`,
    }))
)

// ─── Tabs ───────────────────────────────────────────────────────────────────

/** Page-level navigation tabs: Users | Resellers */
const mainTabs = computed(() => [
  { key: 'users', label: t('customers.tab_users') },
  { key: 'resellers', label: t('customers.tab_resellers') },
])

/** Status filter tabs (only shown when main tab is "users") */
const statusTabs = computed(() => [
  { key: 'all', label: t('customers.all') },
  { key: 'active', label: t('customers.active') },
  { key: 'online', label: t('customers.online') },
  { key: 'limited', label: t('customers.limited') },
  { key: 'disabled', label: t('customers.disabled') },
  { key: 'expired', label: t('customers.expired') },
])

// ─── Users Table ────────────────────────────────────────────────────────────

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

/** The data shown in the table depends on which status filter is active */
const tableData = computed(() => {
  if (activeStatusTab.value === 'online') {
    return store.paginatedList.filter((c: any) => onlineUsernames.value.has(c.username))
  }
  return store.paginatedList
})

// ─── Resellers Table ────────────────────────────────────────────────────────

const resellerColumns = computed(() => [
  { key: 'username', label: t('resellers.username'), sortable: true },
  { key: 'status', label: t('resellers.status'), sortable: true },
  { key: 'credit', label: t('resellers.credit'), sortable: true, align: 'right' as const },
  { key: 'customer_count', label: t('resellers.customers'), sortable: true, align: 'right' as const },
  { key: 'created_at', label: t('resellers.created'), sortable: true },
  { key: 'actions', label: '', align: 'center' as const, width: '160px' },
])

// ─── Search ─────────────────────────────────────────────────────────────────

const debouncedSearch = useDebounceFn((val: string) => {
  store.filters.search = val
}, 300)

function onSearchInput(val: string | number) {
  const strVal = String(val)
  searchQuery.value = strVal
  debouncedSearch(strVal)
}

// ─── Tab Navigation ─────────────────────────────────────────────────────────

function setMainTab(tabKey: string) {
  currentMainTab.value = tabKey
}

function setStatusFilter(status: string) {
  activeStatusTab.value = status
  if (status === 'online') {
    store.filters.status = 'all'
  } else {
    store.filters.status = status as any
  }
  store.pagination.page = 1
}

// ─── User Actions ───────────────────────────────────────────────────────────

function handleRowClick(row: any) {
  router.push({ name: 'user-detail', params: { id: String(row.id) } })
}

function openNewUserSlideOver() {
  userForm.value = { username: '', password: '', display_name: '', plan_id: '', data_gb: '', speed_mbps: '', days: '', template_id: '' }
  showUserSlideOver.value = true
}

async function handleCreateUser() {
  if (!userForm.value.username || !userForm.value.password) return
  saving.value = true
  const success = await store.createCustomer({
    username: userForm.value.username,
    password: userForm.value.password,
    display_name: userForm.value.display_name,
    plan_id: userForm.value.plan_id ? Number(userForm.value.plan_id) : 0,
    data_gb: userForm.value.data_gb ? Number(userForm.value.data_gb) : 0,
    speed_mbps: userForm.value.speed_mbps ? Number(userForm.value.speed_mbps) : 0,
    days: userForm.value.days ? Number(userForm.value.days) : 0,
    template_id: userForm.value.template_id ? Number(userForm.value.template_id) : undefined,
  })
  saving.value = false
  if (success) {
    toast.success(t('customers.created_success'))
    showUserSlideOver.value = false
  } else {
    toast.error(t('customers.created_error'))
  }
}

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
  const success = await store.deleteCustomer(id)
  if (success) {
    toast.success(t('customers.deleted_success').replace('{name}', username))
  } else {
    toast.error(t('customers.deleted_error').replace('{name}', username))
  }
}

// ─── Bulk Actions ───────────────────────────────────────────────────────────

function onSelectionChange(rows: any[]) {
  selectedIds.value = rows.map((r) => r.id)
}

function clearSelection() {
  selectedIds.value = []
}

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

// ─── Reseller Actions ───────────────────────────────────────────────────────

function openNewReseller() {
  resellerForm.value = { username: '', password: '', plan_id: '' }
  editingResellerId.value = null
  showResellerSlideOver.value = true
}

function openEditReseller(reseller: any) {
  resellerForm.value = {
    username: reseller.username,
    password: '',
    plan_id: reseller.default_plan_id ? String(reseller.default_plan_id) : '',
  }
  editingResellerId.value = reseller.id
  showResellerSlideOver.value = true
}

async function handleResellerSubmit() {
  saving.value = true
  if (editingResellerId.value) {
    const success = await resellersStore.updateReseller(editingResellerId.value, {
      password: resellerForm.value.password || undefined,
      default_plan_id: resellerForm.value.plan_id ? Number(resellerForm.value.plan_id) : undefined,
    })
    if (success) toast.success(t('resellers.updated'))
  } else {
    if (!resellerForm.value.username || !resellerForm.value.password) {
      saving.value = false
      return
    }
    const success = await resellersStore.createReseller(resellerForm.value.username, resellerForm.value.password)
    if (success) {
      toast.success(t('resellers.created_success'))
    } else {
      toast.error(t('resellers.create_error'))
    }
  }
  saving.value = false
  showResellerSlideOver.value = false
}

function openCreditAdjust(reseller: any) {
  creditTarget.value = { id: reseller.id, username: reseller.username }
  creditForm.value = { amount: '' }
  showCreditSlideOver.value = true
}

async function handleCreditAdjust() {
  if (!creditTarget.value) return
  saving.value = true
  const success = await resellersStore.adjustCredit(creditTarget.value.id, Number(creditForm.value.amount))
  saving.value = false
  showCreditSlideOver.value = false
  if (success) toast.success(t('resellers.credit_adjusted'))
  creditTarget.value = null
}

async function handleDeleteReseller(id: number, username: string) {
  const confirmed = await confirm({
    title: t('resellers.confirm_delete_title'),
    message: t('resellers.confirm_delete_msg').replace('{name}', username),
    variant: 'danger',
    icon: '\u26A0',
    confirmText: t('btn.delete'),
    cancelText: t('btn.cancel'),
  })
  if (!confirmed) return
  const success = await resellersStore.deleteReseller(id)
  if (success) {
    toast.success(t('resellers.deleted_success').replace('{name}', username))
  } else {
    toast.error(t('resellers.deleted_error').replace('{name}', username))
  }
}

// ─── Lifecycle ──────────────────────────────────────────────────────────────

onMounted(() => {
  store.loadCustomers()
  resellersStore.loadResellers()
  plansStore.loadPlans()
})
</script>

<template>
  <div class="page customers-view">
    <!-- Header -->
    <header class="page-header">
      <KButton
        v-if="currentMainTab === 'users'"
        variant="primary"
        icon="+"
        @click="openNewUserSlideOver"
      >{{ t('customers.new_user') }}</KButton>
      <KButton
        v-if="currentMainTab === 'resellers'"
        variant="primary"
        icon="+"
        @click="openNewReseller"
      >{{ t('resellers.add') }}</KButton>
    </header>

    <!-- Page-level sub-tab navigation: Users | Resellers -->
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

    <!-- ═══════════════ USERS TAB ═══════════════ -->
    <template v-if="currentMainTab === 'users'">
      <!-- Bulk Action Toolbar -->
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

      <!-- Filter Row: Status tabs + Search -->
      <div class="filter-row">
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

      <!-- Users Data Table -->
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
          {{ formatDate(value) }}
        </template>
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
    </template>

    <!-- ═══════════════ RESELLERS TAB ═══════════════ -->
    <template v-if="currentMainTab === 'resellers'">
      <KEmptyState
        v-if="!resellersStore.loading && resellersStore.list.length === 0"
        icon="🤝"
        :title="t('resellers.empty_title')"
        :description="t('resellers.empty_desc')"
      />

      <KDataTable
        v-else
        :columns="resellerColumns"
        :data="resellersStore.list"
        :loading="resellersStore.loading"
        :page-size="20"
        row-key="id"
      >
        <template #cell-status="{ row }">
          <KStatusPill :status="row.is_active ? 'active' : 'disabled'" size="sm" />
        </template>
        <template #cell-credit="{ value }">
          <span class="credit-cell">${{ typeof value === 'number' ? value.toFixed(2) : '0.00' }}</span>
        </template>
        <template #cell-customer_count="{ row }">
          <span class="customer-count">{{ row.customer_count ?? 0 }}</span>
        </template>
        <template #cell-created_at="{ value }">
          {{ formatDate(value) }}
        </template>
        <template #cell-actions="{ row }">
          <div class="action-btns">
            <KButton variant="ghost" size="sm" @click.stop="openEditReseller(row)">{{ t('btn.edit') }}</KButton>
            <KButton variant="ghost" size="sm" @click.stop="openCreditAdjust(row)">{{ t('resellers.credit') }}</KButton>
            <KButton variant="danger" size="sm" @click.stop="handleDeleteReseller(row.id, row.username)">{{ t('btn.delete') }}</KButton>
          </div>
        </template>
      </KDataTable>
    </template>

    <!-- ═══════════════ SLIDE-OVERS ═══════════════ -->

    <!-- New User Slide-Over -->
    <KSlideOver :open="showUserSlideOver" :title="t('customers.new_user')" @close="showUserSlideOver = false">
      <form class="slide-form" @submit.prevent="handleCreateUser">
        <KFormField name="user-username" :label="t('user.username')" required>
          <template #default="{ fieldId }">
            <KInput :id="fieldId" v-model="userForm.username" placeholder="username" />
          </template>
        </KFormField>
        <KFormField name="user-password" :label="t('user.password')" required>
          <template #default="{ fieldId }">
            <KInput :id="fieldId" v-model="userForm.password" type="password" placeholder="••••••" />
          </template>
        </KFormField>
        <KFormField name="user-display-name" :label="t('user.display_name')">
          <template #default="{ fieldId }">
            <KInput :id="fieldId" v-model="userForm.display_name" :placeholder="t('customer.placeholder_display_name')" />
          </template>
        </KFormField>
        <KFormField name="user-plan" :label="t('user.plan')">
          <template #default="{ fieldId }">
            <KSelect :id="fieldId" v-model="userForm.plan_id" :options="planOptions" :placeholder="t('resellers.select_plan')" />
          </template>
        </KFormField>
        <div class="form-row-3">
          <KFormField name="user-data" :label="t('plans.data_gb')">
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="userForm.data_gb" type="number" :placeholder="t('customer.placeholder_plan_default')" />
            </template>
          </KFormField>
          <KFormField name="user-speed" :label="t('plans.speed')">
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="userForm.speed_mbps" type="number" :placeholder="t('customer.placeholder_plan_default')" />
            </template>
          </KFormField>
          <KFormField name="user-days" :label="t('plans.duration_days')">
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="userForm.days" type="number" :placeholder="t('customer.placeholder_plan_default')" />
            </template>
          </KFormField>
        </div>
        <div class="slide-form__footer">
          <KButton variant="ghost" @click="showUserSlideOver = false">{{ t('btn.cancel') }}</KButton>
          <KButton type="submit" variant="primary" :loading="saving">{{ t('btn.create') }}</KButton>
        </div>
      </form>
    </KSlideOver>

    <!-- Reseller Create/Edit Slide-Over -->
    <KSlideOver
      :open="showResellerSlideOver"
      :title="editingResellerId ? t('resellers.edit') : t('resellers.new')"
      @close="showResellerSlideOver = false"
    >
      <form class="slide-form" @submit.prevent="handleResellerSubmit">
        <KFormField name="reseller-username" :label="t('resellers.username')" required>
          <template #default="{ fieldId }">
            <KInput :id="fieldId" v-model="resellerForm.username" placeholder="reseller_name" :disabled="!!editingResellerId" />
          </template>
        </KFormField>
        <KFormField name="reseller-password" :label="t('resellers.password')" :required="!editingResellerId">
          <template #default="{ fieldId }">
            <KInput
              :id="fieldId"
              v-model="resellerForm.password"
              type="password"
              :placeholder="editingResellerId ? t('resellers.password_unchanged') : t('resellers.password_placeholder')"
            />
          </template>
        </KFormField>
        <KFormField name="reseller-plan" :label="t('resellers.default_plan')" :hint="t('resellers.plan_hint')">
          <template #default="{ fieldId, describedBy }">
            <KSelect :id="fieldId" v-model="resellerForm.plan_id" :options="quotaPlanOptions" :placeholder="t('resellers.select_plan')" :aria-describedby="describedBy" />
          </template>
        </KFormField>
        <div class="slide-form__footer">
          <KButton variant="ghost" @click="showResellerSlideOver = false">{{ t('btn.cancel') }}</KButton>
          <KButton type="submit" variant="primary" :loading="saving">
            {{ editingResellerId ? t('btn.save') : t('resellers.create') }}
          </KButton>
        </div>
      </form>
    </KSlideOver>

    <!-- Credit Adjustment Slide-Over -->
    <KSlideOver :open="showCreditSlideOver" :title="`${t('resellers.adjust_credit')}: ${creditTarget?.username ?? ''}`" width="360px" @close="showCreditSlideOver = false">
      <form class="slide-form" @submit.prevent="handleCreditAdjust">
        <KFormField name="credit-amount" :label="t('resellers.credit')" :hint="t('resellers.credit_hint')" required>
          <template #default="{ fieldId, describedBy }">
            <KInput :id="fieldId" v-model="creditForm.amount" type="number" placeholder="10.00" :aria-describedby="describedBy" />
          </template>
        </KFormField>
        <div class="slide-form__footer">
          <KButton variant="ghost" @click="showCreditSlideOver = false">{{ t('btn.cancel') }}</KButton>
          <KButton type="submit" variant="primary" :loading="saving">{{ t('resellers.adjust_credit') }}</KButton>
        </div>
      </form>
    </KSlideOver>
  </div>
</template>

<style scoped>
.customers-view { display: flex; flex-direction: column; gap: var(--space-4); }

.page-header { display: flex; align-items: center; justify-content: flex-end; }

/* Main page-level tabs (Users | Resellers) */
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

/* Resellers tab */
.credit-cell { font-weight: var(--font-semibold); color: var(--color-accent); }
.customer-count { font-weight: var(--font-medium); color: var(--color-muted); }
.action-btns { display: flex; gap: var(--space-1); }

.text-success { color: var(--color-success, #22c55e); }
.text-danger { color: var(--color-danger, #ef4444); }

/* Slide-over form styles */
.slide-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.slide-form__footer {
  display: flex;
  justify-content: flex-end;
  gap: var(--space-2);
  padding-top: var(--space-4);
  border-top: 1px solid var(--color-border);
  margin-top: var(--space-2);
}

.form-row-3 {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--space-3);
}

/* Responsive */
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

  .form-row-3 {
    grid-template-columns: 1fr;
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

@media (max-width: 768px) {
  .customers-view :deep(.k-table-wrapper),
  .customers-view :deep(.k-data-table) {
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
  }

  .customers-view :deep(table) {
    min-width: 700px;
  }

  .page-header {
    justify-content: stretch;
  }

  .page-header :deep(.k-btn) {
    width: 100%;
  }

  .main-tabs {
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
  }

  .main-tab {
    white-space: nowrap;
  }
}
</style>
