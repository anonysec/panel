<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useCustomersStore } from '@/stores/customers'
import { useResellersStore } from '@/stores/resellers'
import { usePlansStore } from '@/stores/plans'
import { useRealtimeStore } from '@/stores/realtime'
import { useAuthStore } from '@/stores/auth'
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
import { useApi } from '@koris/composables/useApi'
import { formatDate } from '@koris/composables/useFormatDate'

const { t } = useI18n()
const router = useRouter()
const store = useCustomersStore()
const resellersStore = useResellersStore()
const plansStore = usePlansStore()
const realtime = useRealtimeStore()
const authStore = useAuthStore()
const { confirm } = useConfirm()
const toast = useToast()
const api = useApi()

const isReseller = computed(() => authStore.user?.role === 'reseller')

const searchQuery = ref('')
const activeStatusTab = ref<string>('all')
const currentMainTab = ref<string>('users')

// ─── Advanced Filters Panel ─────────────────────────────────────────────────
const showAdvancedFilters = ref(false)
const filterPlanId = ref('')
const filterDateFrom = ref('')
const filterDateTo = ref('')

const planFilterOptions = computed(() => {
  const options = [{ label: t('customers.no_plan'), value: '' }]
  for (const p of plansStore.list) {
    options.push({ label: p.name, value: String(p.id) })
  }
  return options
})

// ─── Column Visibility Toggle ───────────────────────────────────────────────
const showColumnToggle = ref(false)

interface ColumnVisibility {
  key: string
  label: string
  visible: boolean
}

const columnVisibility = ref<ColumnVisibility[]>([
  { key: 'username', label: 'customers.col_username', visible: true },
  { key: 'display_name', label: 'customers.col_display_name', visible: true },
  { key: 'status', label: 'customers.col_status', visible: true },
  { key: 'plan', label: 'customers.col_plan', visible: true },
  { key: 'credit', label: 'customers.col_credit', visible: true },
  { key: 'created_by', label: 'customers.col_created_by', visible: true },
  { key: 'created_at', label: 'customers.col_created_at', visible: true },
])

function toggleColumnVisibility(key: string) {
  const col = columnVisibility.value.find(c => c.key === key)
  if (col) col.visible = !col.visible
}

// ─── Export ─────────────────────────────────────────────────────────────────
const exporting = ref(false)

async function handleExport(format: 'csv' | 'json') {
  exporting.value = true
  try {
    const params = new URLSearchParams()
    params.set('format', format)
    if (searchQuery.value) params.set('search', searchQuery.value)
    if (activeStatusTab.value !== 'all' && activeStatusTab.value !== 'online') {
      params.set('status', activeStatusTab.value)
    }
    if (filterPlanId.value) params.set('plan_id', filterPlanId.value)

    const url = `/api/admin/customers/export?${params.toString()}`
    // Use window.open for file download
    window.open(url, '_blank')
  } finally {
    exporting.value = false
  }
}

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
  avatar: '',
})

const resellerForm = ref({
  username: '',
  password: '',
  plan_id: '' as string | number,
  avatar: '',
})

const creditForm = ref({ amount: '' })

// Allowed plans for the reseller being edited
const resellerAllowedPlanIds = ref<number[]>([])

// Default avatar emojis for user and reseller selection
const defaultEmojis = ['🦊', '🐻', '🐼', '🐨', '🦁', '🐯', '🐸', '🐙', '🦋', '🌟', '🔥', '💎', '🎯', '🚀', '⚡', '🌈', '🎪', '🎭', '🏆', '👑']

// Reserved emojis (used by resellers, filtered from user creation picker)
interface ReservedEmojiInfo { emoji: string; reseller: string }
const reservedEmojiList = ref<ReservedEmojiInfo[]>([])

const availableUserEmojis = computed(() => {
  const reservedSet = new Set(reservedEmojiList.value.map(r => r.emoji))
  return defaultEmojis.filter(e => !reservedSet.has(e))
})

function getResellerForEmoji(emoji: string): string {
  const info = reservedEmojiList.value.find(r => r.emoji === emoji)
  return info?.reseller ?? ''
}

function isReservedByOther(emoji: string): boolean {
  const info = reservedEmojiList.value.find(r => r.emoji === emoji)
  if (!info) return false
  // If we're editing a reseller, their own emoji is not "reserved by other"
  if (editingResellerId.value) {
    const currentReseller = resellersStore.list.find(r => r.id === editingResellerId.value)
    if (currentReseller && currentReseller.username === info.reseller) return false
  }
  return true
}

async function loadReservedEmojis() {
  if (isReseller.value) return // resellers don't need this
  try {
    const data = await api.get<{ ok: boolean; reserved: ReservedEmojiInfo[] }>('/api/reserved-emojis')
    if (data?.ok) {
      reservedEmojiList.value = data.reserved
    }
  } catch { /* ignore */ }
}

// ─── Plan Options ───────────────────────────────────────────────────────────
const resellerPlans = ref<{ id: number; name: string; data_gb: number; duration_days: number; wholesale_price: number }[]>([])

const planOptions = computed(() => {
  if (isReseller.value) {
    return resellerPlans.value.map((p) => ({
      value: String(p.id),
      label: `${p.name} (${p.data_gb}GB / ${p.duration_days}d)`,
    }))
  }
  return plansStore.activePlans.map((p) => ({
    value: String(p.id),
    label: `${p.name} (${p.data_gb}GB / ${p.duration_days}d — $${p.price})`,
  }))
})

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
const mainTabs = computed(() => {
  const tabs = [{ key: 'users', label: t('customers.tab_users') }]
  if (!isReseller.value) {
    tabs.push({ key: 'resellers', label: t('customers.tab_resellers') })
  }
  return tabs
})

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

const columns = computed(() => {
  const allCols = [
    { key: 'username', label: t('user.username'), sortable: true },
    { key: 'display_name', label: t('user.display_name'), sortable: true },
    { key: 'status', label: t('user.status'), sortable: true },
    { key: 'plan', label: t('user.plan'), sortable: true },
    { key: 'credit', label: t('user.balance'), sortable: true, align: 'right' as const },
  ]
  if (!isReseller.value) {
    allCols.push({ key: 'created_by', label: t('user.created_by'), sortable: true })
  }
  allCols.push(
    { key: 'created_at', label: t('user.created'), sortable: true },
    { key: 'actions', label: '', sortable: false, align: 'center' as const, width: '80px' } as any,
  )
  // Filter by visibility
  const visibleKeys = new Set(columnVisibility.value.filter(c => c.visible).map(c => c.key))
  return allCols.filter(col => col.key === 'actions' || visibleKeys.has(col.key))
})

/** Set of usernames currently online (from live sessions) */
const onlineUsernames = computed(() => {
  return new Set(realtime.liveSessions.map((s) => s.username))
})

/** The data shown in the table depends on which status filter is active */
const tableData = computed(() => {
  let result = store.paginatedList as any[]

  if (activeStatusTab.value === 'online') {
    result = result.filter((c: any) => onlineUsernames.value.has(c.username))
  }

  // Advanced filter: plan
  if (filterPlanId.value) {
    result = result.filter((c: any) => String(c.plan_id) === filterPlanId.value)
  }

  // Advanced filter: date from
  if (filterDateFrom.value) {
    result = result.filter((c: any) => {
      if (!c.created_at) return false
      return c.created_at >= filterDateFrom.value
    })
  }

  // Advanced filter: date to
  if (filterDateTo.value) {
    result = result.filter((c: any) => {
      if (!c.created_at) return false
      return c.created_at <= filterDateTo.value + 'T23:59:59'
    })
  }

  return result
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
  userForm.value = { username: '', password: '', display_name: '', plan_id: '', data_gb: '', speed_mbps: '', days: '', template_id: '', avatar: '' }
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
    avatar: userForm.value.avatar || undefined,
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
  } else if (action === 'disable') {
    const confirmed = await confirm({
      title: t('customers.confirm_bulk_title'),
      message: t('customers.confirm_bulk_msg').replace('{action}', t('customers.disable')).replace('{count}', String(selectedIds.value.length)),
      variant: 'danger',
      icon: '\u26A0',
      confirmText: t('btn.disable'),
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

// Extended bulk actions (using admin API)
const showBulkPlanSlide = ref(false)
const bulkPlanId = ref('')
const bulkExtendDays = ref('')
const bulkAddDataGb = ref('')
const showBulkExtendSlide = ref(false)
const showBulkDataSlide = ref(false)

// Bulk Assign Tag
const showBulkTagSlide = ref(false)
const bulkTagId = ref('')
const availableTags = ref<{ id: number; name: string; color: string }[]>([])

// Import
const showImportSlide = ref(false)
const importMode = ref<'file' | 'paste'>('file')
const importFileInput = ref<HTMLInputElement | null>(null)
const importCsvText = ref('')
const importPreview = ref<{ rows: any[]; errors: string[] } | null>(null)
const importLoading = ref(false)
const importResult = ref<{ created: number; updated: number; skipped: number; errors: string[] } | null>(null)

async function executeBulkExtend() {
  if (!bulkExtendDays.value || selectedIds.value.length === 0) return
  saving.value = true
  try {
    const res = await api.post<{ ok: boolean }>('/api/admin/customers/bulk', {
      customer_ids: [...selectedIds.value],
      action: 'extend',
      days: Number(bulkExtendDays.value),
    })
    if (res?.ok) {
      toast.success(t('customers.bulk_success').replace('{count}', String(selectedIds.value.length)))
      clearSelection()
      store.loadCustomers()
    }
  } catch {
    toast.error(t('customers.bulk_error').replace('{count}', String(selectedIds.value.length)))
  } finally {
    saving.value = false
    showBulkExtendSlide.value = false
    bulkExtendDays.value = ''
  }
}

async function executeBulkChangePlan() {
  if (!bulkPlanId.value || selectedIds.value.length === 0) return
  saving.value = true
  try {
    const res = await api.post<{ ok: boolean }>('/api/admin/customers/bulk', {
      customer_ids: [...selectedIds.value],
      action: 'change_plan',
      plan_id: Number(bulkPlanId.value),
    })
    if (res?.ok) {
      toast.success(t('customers.bulk_success').replace('{count}', String(selectedIds.value.length)))
      clearSelection()
      store.loadCustomers()
    }
  } catch {
    toast.error(t('customers.bulk_error').replace('{count}', String(selectedIds.value.length)))
  } finally {
    saving.value = false
    showBulkPlanSlide.value = false
    bulkPlanId.value = ''
  }
}

async function executeBulkAddData() {
  if (!bulkAddDataGb.value || selectedIds.value.length === 0) return
  saving.value = true
  try {
    const res = await api.post<{ ok: boolean }>('/api/admin/customers/bulk', {
      customer_ids: [...selectedIds.value],
      action: 'add_data',
      data_gb: Number(bulkAddDataGb.value),
    })
    if (res?.ok) {
      toast.success(t('customers.bulk_success').replace('{count}', String(selectedIds.value.length)))
      clearSelection()
      store.loadCustomers()
    }
  } catch {
    toast.error(t('customers.bulk_error').replace('{count}', String(selectedIds.value.length)))
  } finally {
    saving.value = false
    showBulkDataSlide.value = false
    bulkAddDataGb.value = ''
  }
}

// ─── Bulk Assign Tag ────────────────────────────────────────────────────────

async function loadAvailableTags() {
  try {
    const data = await api.get<{ ok: boolean; tags: { id: number; name: string; color: string }[] }>('/api/tags')
    if (data?.ok) {
      availableTags.value = data.tags || []
    }
  } catch { /* ignore */ }
}

function openBulkTagSlide() {
  loadAvailableTags()
  bulkTagId.value = ''
  showBulkTagSlide.value = true
}

async function executeBulkAssignTag() {
  if (!bulkTagId.value || selectedIds.value.length === 0) return
  saving.value = true
  try {
    const res = await api.post<{ ok: boolean }>('/api/admin/customers/bulk', {
      customer_ids: [...selectedIds.value],
      action: 'assign_tag',
      tag_id: Number(bulkTagId.value),
    })
    if (res?.ok) {
      toast.success(t('customers.bulk_success').replace('{count}', String(selectedIds.value.length)))
      clearSelection()
      store.loadCustomers()
    }
  } catch {
    toast.error(t('customers.bulk_error').replace('{count}', String(selectedIds.value.length)))
  } finally {
    saving.value = false
    showBulkTagSlide.value = false
    bulkTagId.value = ''
  }
}

// ─── CSV Import ─────────────────────────────────────────────────────────────

function openImportSlide() {
  importMode.value = 'file'
  importCsvText.value = ''
  importPreview.value = null
  importResult.value = null
  showImportSlide.value = true
}

function handleFileSelect(event: Event) {
  const input = event.target as HTMLInputElement
  if (!input.files?.length) return
  const file = input.files[0]
  const reader = new FileReader()
  reader.onload = (e) => {
    importCsvText.value = e.target?.result as string
    previewImport()
  }
  reader.readAsText(file)
}

async function previewImport() {
  if (!importCsvText.value.trim()) {
    toast.error(t('customers.import_empty'))
    return
  }
  importLoading.value = true
  try {
    const data = await api.post<{ ok: boolean; rows: any[]; errors: string[] }>('/api/customers/import/preview', {
      csv_data: importCsvText.value,
    })
    if (data?.ok) {
      importPreview.value = { rows: data.rows || [], errors: data.errors || [] }
    }
  } catch {
    // error toast handled by useApi
  } finally {
    importLoading.value = false
  }
}

async function executeImport() {
  if (!importCsvText.value.trim()) return
  importLoading.value = true
  try {
    const data = await api.post<{ ok: boolean; created: number; updated: number; skipped: number; errors: string[] }>('/api/customers/import', {
      csv_data: importCsvText.value,
    })
    if (data?.ok) {
      importResult.value = {
        created: data.created || 0,
        updated: data.updated || 0,
        skipped: data.skipped || 0,
        errors: data.errors || [],
      }
      toast.success(t('customers.import_success'))
      store.loadCustomers()
    }
  } catch {
    // error toast handled by useApi
  } finally {
    importLoading.value = false
  }
}

function closeImport() {
  showImportSlide.value = false
  importPreview.value = null
  importResult.value = null
  importCsvText.value = ''
}

// ─── Reseller Actions ───────────────────────────────────────────────────────

function openNewReseller() {
  resellerForm.value = { username: '', password: '', plan_id: '', avatar: '' }
  editingResellerId.value = null
  showResellerSlideOver.value = true
}

function openEditReseller(reseller: any) {
  resellerForm.value = {
    username: reseller.username,
    password: '',
    plan_id: reseller.default_plan_id ? String(reseller.default_plan_id) : '',
    avatar: reseller.avatar || '',
  }
  editingResellerId.value = reseller.id
  resellerAllowedPlanIds.value = []
  loadResellerAllowedPlans(reseller.id)
  showResellerSlideOver.value = true
}

async function loadResellerAllowedPlans(resellerId: number) {
  try {
    const data = await api.get<{ ok: boolean; plan_ids: number[] }>(`/api/resellers/${resellerId}/plans`)
    if (data?.ok) {
      resellerAllowedPlanIds.value = data.plan_ids || []
    }
  } catch { /* ignore */ }
}

function toggleAllowedPlan(planId: number) {
  const idx = resellerAllowedPlanIds.value.indexOf(planId)
  if (idx >= 0) {
    resellerAllowedPlanIds.value.splice(idx, 1)
  } else {
    resellerAllowedPlanIds.value.push(planId)
  }
}

async function handleResellerSubmit() {
  saving.value = true
  if (editingResellerId.value) {
    const success = await resellersStore.updateReseller(editingResellerId.value, {
      password: resellerForm.value.password || undefined,
      default_plan_id: resellerForm.value.plan_id ? Number(resellerForm.value.plan_id) : undefined,
      avatar: resellerForm.value.avatar,
    })
    // Save allowed plans
    try {
      await api.post(`/api/resellers/${editingResellerId.value}/plans`, { plan_ids: resellerAllowedPlanIds.value })
    } catch { /* ignore */ }
    if (success) toast.success(t('resellers.updated'))
  } else {
    if (!resellerForm.value.username || !resellerForm.value.password) {
      saving.value = false
      return
    }
    const success = await resellersStore.createReseller(resellerForm.value.username, resellerForm.value.password, resellerForm.value.avatar)
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

onMounted(async () => {
  store.loadCustomers()
  resellersStore.loadResellers()
  plansStore.loadPlans()
  loadReservedEmojis()

  // Load reseller-specific allowed plans
  if (isReseller.value) {
    try {
      const data = await api.get<{ ok: boolean; plans: any[] }>('/api/reseller/plan-prices')
      if (data?.ok) {
        resellerPlans.value = data.plans
      }
    } catch { /* ignore */ }
  }
})
</script>

<template>
  <div class="page customers-view">
    <!-- Header -->
    <header class="page-header">
      <div class="page-header__actions">
        <KButton
          v-if="currentMainTab === 'users'"
          variant="ghost"
          size="sm"
          @click="showAdvancedFilters = !showAdvancedFilters"
        >{{ t('customers.advanced_filters') }}</KButton>
        <KButton
          v-if="currentMainTab === 'users'"
          variant="ghost"
          size="sm"
          @click="showColumnToggle = !showColumnToggle"
        >{{ t('customers.columns') }}</KButton>
        <KButton
          v-if="currentMainTab === 'users'"
          variant="ghost"
          size="sm"
          @click="openImportSlide"
        >{{ t('customers.import_csv') }}</KButton>
      </div>
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
      <!-- Column Visibility Toggle Panel -->
      <Transition name="panel-slide">
        <div v-if="showColumnToggle" class="columns-panel" role="group" aria-label="Column visibility">
          <label
            v-for="col in columnVisibility"
            :key="col.key"
            class="column-toggle"
          >
            <input
              type="checkbox"
              :checked="col.visible"
              @change="toggleColumnVisibility(col.key)"
            />
            <span>{{ t(col.label) }}</span>
          </label>
        </div>
      </Transition>

      <!-- Advanced Filters Panel -->
      <Transition name="panel-slide">
        <div v-if="showAdvancedFilters" class="advanced-filters-panel" role="search" aria-label="Advanced filters">
          <div class="advanced-filters-grid">
            <KFormField name="filter-plan" :label="t('customers.filter_plan')">
              <template #default="{ fieldId }">
                <KSelect :id="fieldId" v-model="filterPlanId" :options="planFilterOptions" />
              </template>
            </KFormField>
            <KFormField name="filter-date-from" :label="t('customers.filter_date_from')">
              <template #default="{ fieldId }">
                <input :id="fieldId" v-model="filterDateFrom" type="date" class="filter-date-input" />
              </template>
            </KFormField>
            <KFormField name="filter-date-to" :label="t('customers.filter_date_to')">
              <template #default="{ fieldId }">
                <input :id="fieldId" v-model="filterDateTo" type="date" class="filter-date-input" />
              </template>
            </KFormField>
          </div>
        </div>
      </Transition>

      <!-- Bulk Action Toolbar -->
      <Transition name="bulk-toolbar">
        <div v-if="hasSelection" class="bulk-toolbar" role="toolbar" aria-label="Bulk actions">
          <span class="bulk-toolbar__count">{{ selectedIds.length }} {{ t('customers.selected') }}</span>
          <div class="bulk-toolbar__actions">
            <KButton variant="ghost" size="sm" @click="executeBulkAction('enable')">{{ t('customers.enable') }}</KButton>
            <KButton variant="ghost" size="sm" @click="executeBulkAction('disable')">{{ t('customers.disable') }}</KButton>
            <KButton variant="ghost" size="sm" @click="executeBulkAction('traffic_reset')">{{ t('customers.traffic_reset') }}</KButton>
            <KButton variant="ghost" size="sm" @click="showBulkExtendSlide = true">{{ t('customers.bulk_extend') }}</KButton>
            <KButton variant="ghost" size="sm" @click="showBulkPlanSlide = true">{{ t('customers.bulk_change_plan') }}</KButton>
            <KButton variant="ghost" size="sm" @click="showBulkDataSlide = true">{{ t('customers.bulk_add_data') }}</KButton>
            <KButton variant="ghost" size="sm" @click="openBulkTagSlide">{{ t('customers.bulk_assign_tag') }}</KButton>
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
            <KAvatar :name="row.display_name || value" size="sm" :emoji="row.avatar || undefined" />
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
        <template #cell-created_by="{ value }">
          <span class="created-by-cell">{{ value || '—' }}</span>
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
        <KFormField v-if="!isReseller" name="user-avatar" :label="t('user.avatar')">
          <template #default>
            <div class="emoji-picker">
              <button
                v-for="em in availableUserEmojis"
                :key="em"
                type="button"
                class="emoji-btn"
                :class="{ 'emoji-btn--active': userForm.avatar === em }"
                @click="userForm.avatar = userForm.avatar === em ? '' : em"
              >{{ em }}</button>
              <button
                v-for="em in reservedEmojiList"
                :key="'reserved-' + em.emoji"
                type="button"
                class="emoji-btn emoji-btn--reserved"
                disabled
                :title="`Used by reseller: ${em.reseller}`"
              >{{ em.emoji }}</button>
            </div>
          </template>
        </KFormField>
        <div class="slide-form__footer">
          <KButton type="button" variant="ghost" @click="showUserSlideOver = false">{{ t('btn.cancel') }}</KButton>
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
        <KFormField name="reseller-avatar" :label="t('resellers.avatar')" :hint="t('resellers.avatar_hint')">
          <template #default>
            <div class="emoji-picker">
              <button
                v-for="em in defaultEmojis"
                :key="em"
                type="button"
                class="emoji-btn"
                :class="{
                  'emoji-btn--active': resellerForm.avatar === em,
                  'emoji-btn--reserved': isReservedByOther(em),
                }"
                :disabled="isReservedByOther(em)"
                :title="isReservedByOther(em) ? `Used by: ${getResellerForEmoji(em)}` : ''"
                @click="resellerForm.avatar = resellerForm.avatar === em ? '' : em"
              >{{ em }}</button>
            </div>
          </template>
        </KFormField>
        <KFormField v-if="editingResellerId" name="reseller-allowed-plans" :label="t('resellers.allowed_plans')" :hint="t('resellers.allowed_plans_hint')">
          <template #default>
            <div class="allowed-plans-checklist">
              <label
                v-for="plan in quotaPlanOptions"
                :key="plan.value"
                class="plan-check"
              >
                <input
                  type="checkbox"
                  :value="Number(plan.value)"
                  :checked="resellerAllowedPlanIds.includes(Number(plan.value))"
                  @change="toggleAllowedPlan(Number(plan.value))"
                />
                <span>{{ plan.label }}</span>
              </label>
              <p v-if="quotaPlanOptions.length === 0" class="empty-hint">{{ t('resellers.plan_hint') }}</p>
            </div>
          </template>
        </KFormField>
        <div class="slide-form__footer">
          <KButton type="button" variant="ghost" @click="showResellerSlideOver = false">{{ t('btn.cancel') }}</KButton>
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
          <KButton type="button" variant="ghost" @click="showCreditSlideOver = false">{{ t('btn.cancel') }}</KButton>
          <KButton type="submit" variant="primary" :loading="saving">{{ t('resellers.adjust_credit') }}</KButton>
        </div>
      </form>
    </KSlideOver>

    <!-- Bulk Extend Slide-Over -->
    <KSlideOver :open="showBulkExtendSlide" :title="t('customers.bulk_extend')" width="360px" @close="showBulkExtendSlide = false">
      <form class="slide-form" @submit.prevent="executeBulkExtend">
        <p class="slide-form__hint">{{ selectedIds.length }} {{ t('customers.selected') }}</p>
        <KFormField name="bulk-extend-days" :label="t('plans.duration_days')" required>
          <template #default="{ fieldId }">
            <KInput :id="fieldId" v-model="bulkExtendDays" type="number" placeholder="30" />
          </template>
        </KFormField>
        <div class="slide-form__footer">
          <KButton type="button" variant="ghost" @click="showBulkExtendSlide = false">{{ t('btn.cancel') }}</KButton>
          <KButton type="submit" variant="primary" :loading="saving">{{ t('customers.bulk_extend') }}</KButton>
        </div>
      </form>
    </KSlideOver>

    <!-- Bulk Change Plan Slide-Over -->
    <KSlideOver :open="showBulkPlanSlide" :title="t('customers.bulk_change_plan')" width="360px" @close="showBulkPlanSlide = false">
      <form class="slide-form" @submit.prevent="executeBulkChangePlan">
        <p class="slide-form__hint">{{ selectedIds.length }} {{ t('customers.selected') }}</p>
        <KFormField name="bulk-plan" :label="t('user.plan')" required>
          <template #default="{ fieldId }">
            <KSelect :id="fieldId" v-model="bulkPlanId" :options="planOptions" :placeholder="t('resellers.select_plan')" />
          </template>
        </KFormField>
        <div class="slide-form__footer">
          <KButton type="button" variant="ghost" @click="showBulkPlanSlide = false">{{ t('btn.cancel') }}</KButton>
          <KButton type="submit" variant="primary" :loading="saving">{{ t('customers.bulk_change_plan') }}</KButton>
        </div>
      </form>
    </KSlideOver>

    <!-- Bulk Add Data Slide-Over -->
    <KSlideOver :open="showBulkDataSlide" :title="t('customers.bulk_add_data')" width="360px" @close="showBulkDataSlide = false">
      <form class="slide-form" @submit.prevent="executeBulkAddData">
        <p class="slide-form__hint">{{ selectedIds.length }} {{ t('customers.selected') }}</p>
        <KFormField name="bulk-data-gb" :label="t('plans.data_gb')" required>
          <template #default="{ fieldId }">
            <KInput :id="fieldId" v-model="bulkAddDataGb" type="number" placeholder="10" />
          </template>
        </KFormField>
        <div class="slide-form__footer">
          <KButton type="button" variant="ghost" @click="showBulkDataSlide = false">{{ t('btn.cancel') }}</KButton>
          <KButton type="submit" variant="primary" :loading="saving">{{ t('customers.bulk_add_data') }}</KButton>
        </div>
      </form>
    </KSlideOver>

    <!-- Bulk Assign Tag Slide-Over -->
    <KSlideOver :open="showBulkTagSlide" :title="t('customers.bulk_assign_tag')" width="400px" @close="showBulkTagSlide = false">
      <form class="slide-form" @submit.prevent="executeBulkAssignTag">
        <p class="slide-form__hint">{{ selectedIds.length }} {{ t('customers.selected') }}</p>
        <KFormField name="bulk-tag" :label="t('tags.select_tag')" required>
          <template #default>
            <div class="tag-select-list">
              <label
                v-for="tag in availableTags"
                :key="tag.id"
                class="tag-select-item"
                :class="{ 'tag-select-item--active': bulkTagId === String(tag.id) }"
              >
                <input
                  v-model="bulkTagId"
                  type="radio"
                  name="bulk-tag"
                  :value="String(tag.id)"
                  class="tag-radio"
                />
                <span class="tag-swatch" :style="{ backgroundColor: tag.color }" />
                <span class="tag-select-name">{{ tag.name }}</span>
              </label>
              <p v-if="availableTags.length === 0" class="empty-hint">{{ t('tags.empty_title') }}</p>
            </div>
          </template>
        </KFormField>
        <div class="slide-form__footer">
          <KButton type="button" variant="ghost" @click="showBulkTagSlide = false">{{ t('btn.cancel') }}</KButton>
          <KButton type="submit" variant="primary" :loading="saving" :disabled="!bulkTagId">{{ t('customers.bulk_assign_tag') }}</KButton>
        </div>
      </form>
    </KSlideOver>

    <!-- CSV Import Slide-Over -->
    <KSlideOver :open="showImportSlide" :title="t('customers.import_title')" width="600px" @close="closeImport">
      <div class="slide-form">
        <!-- Mode Toggle -->
        <div class="import-mode-toggle">
          <button
            type="button"
            class="mode-btn"
            :class="{ 'mode-btn--active': importMode === 'file' }"
            @click="importMode = 'file'"
          >{{ t('customers.import_file') }}</button>
          <button
            type="button"
            class="mode-btn"
            :class="{ 'mode-btn--active': importMode === 'paste' }"
            @click="importMode = 'paste'"
          >{{ t('customers.import_paste') }}</button>
        </div>

        <!-- File Upload -->
        <div v-if="importMode === 'file'" class="import-file-section">
          <label class="import-file-label">
            <input
              ref="importFileInput"
              type="file"
              accept=".csv,text/csv"
              class="import-file-input"
              @change="handleFileSelect"
            />
            <span class="import-file-btn">{{ t('customers.import_choose_file') }}</span>
          </label>
          <p class="import-hint">{{ t('customers.import_format_hint') }}</p>
        </div>

        <!-- CSV Paste -->
        <div v-if="importMode === 'paste'" class="import-paste-section">
          <KFormField name="import-csv" :label="t('customers.import_csv_data')">
            <template #default="{ fieldId }">
              <textarea
                :id="fieldId"
                v-model="importCsvText"
                class="import-textarea"
                rows="8"
                :placeholder="t('customers.import_csv_placeholder')"
              />
            </template>
          </KFormField>
          <KButton variant="ghost" size="sm" @click="previewImport" :loading="importLoading">
            {{ t('customers.import_preview') }}
          </KButton>
        </div>

        <!-- Preview Results -->
        <div v-if="importPreview" class="import-preview">
          <h4 class="import-preview__title">{{ t('customers.import_preview_title') }}</h4>
          <p class="import-preview__count">
            {{ importPreview.rows.length }} {{ t('customers.import_rows_valid') }}
          </p>
          <div v-if="importPreview.errors.length > 0" class="import-errors">
            <p class="import-errors__title">{{ t('customers.import_errors') }}:</p>
            <ul class="import-errors__list">
              <li v-for="(err, idx) in importPreview.errors.slice(0, 10)" :key="idx">{{ err }}</li>
              <li v-if="importPreview.errors.length > 10">
                ... {{ t('customers.import_more_errors').replace('{count}', String(importPreview.errors.length - 10)) }}
              </li>
            </ul>
          </div>
          <div v-if="importPreview.rows.length > 0" class="import-preview-table">
            <table class="preview-table">
              <thead>
                <tr>
                  <th>{{ t('user.username') }}</th>
                  <th>{{ t('user.status') }}</th>
                  <th>{{ t('user.plan') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(row, idx) in importPreview.rows.slice(0, 5)" :key="idx">
                  <td>{{ row.username }}</td>
                  <td>{{ row.status || 'active' }}</td>
                  <td>{{ row.plan || '—' }}</td>
                </tr>
                <tr v-if="importPreview.rows.length > 5">
                  <td colspan="3" class="preview-more">
                    ... {{ importPreview.rows.length - 5 }} {{ t('customers.import_more_rows') }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <!-- Import Result -->
        <div v-if="importResult" class="import-result">
          <h4 class="import-result__title">{{ t('customers.import_complete') }}</h4>
          <div class="import-result__stats">
            <span class="stat stat--success">{{ importResult.created }} {{ t('customers.import_created') }}</span>
            <span class="stat stat--info">{{ importResult.updated }} {{ t('customers.import_updated') }}</span>
            <span class="stat stat--warn">{{ importResult.skipped }} {{ t('customers.import_skipped') }}</span>
          </div>
          <div v-if="importResult.errors.length > 0" class="import-errors">
            <ul class="import-errors__list">
              <li v-for="(err, idx) in importResult.errors.slice(0, 10)" :key="idx">{{ err }}</li>
            </ul>
          </div>
        </div>

        <!-- Actions -->
        <div class="slide-form__footer">
          <KButton type="button" variant="ghost" @click="closeImport">{{ t('btn.cancel') }}</KButton>
          <KButton
            v-if="importPreview && !importResult"
            variant="primary"
            :loading="importLoading"
            :disabled="!importPreview.rows.length"
            @click="executeImport"
          >{{ t('customers.import_execute') }}</KButton>
        </div>
      </div>
    </KSlideOver>
  </div>
</template>

<style scoped>
.customers-view { display: flex; flex-direction: column; gap: var(--space-4); }

.page-header { display: flex; align-items: center; justify-content: flex-end; gap: var(--space-3); }
.page-header__actions { display: flex; gap: var(--space-2); margin-right: auto; }

/* Advanced Filters Panel */
.advanced-filters-panel {
  padding: var(--space-4);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
}

.advanced-filters-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: var(--space-3);
}

.filter-date-input {
  width: 100%;
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  color: var(--color-text);
  font-size: var(--text-sm);
}

.filter-date-input:focus {
  outline: none;
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px rgba(37, 99, 235, 0.15);
}

/* Column Visibility Toggle */
.columns-panel {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
}

.column-toggle {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--text-sm);
  cursor: pointer;
  white-space: nowrap;
}

.column-toggle input[type="checkbox"] {
  width: 16px;
  height: 16px;
  accent-color: var(--color-primary);
}

/* Panel transition */
.panel-slide-enter-active,
.panel-slide-leave-active {
  transition: opacity var(--duration-normal) var(--ease-out),
              max-height var(--duration-normal) var(--ease-out);
  overflow: hidden;
}

.panel-slide-enter-from,
.panel-slide-leave-to {
  opacity: 0;
  max-height: 0;
}

.panel-slide-enter-to,
.panel-slide-leave-from {
  max-height: 200px;
}

/* Slide form hint */
.slide-form__hint {
  font-size: var(--text-sm);
  color: var(--color-muted);
  margin: 0;
}

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
.created-by-cell { font-size: var(--text-xs); color: var(--color-muted); }

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

/* Emoji Picker for reseller avatar */
.emoji-picker {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.emoji-btn {
  width: 36px;
  height: 36px;
  border-radius: var(--radius-md, 8px);
  border: 1px solid var(--color-border, #28333f);
  background: var(--color-surface, #0b1120);
  font-size: 18px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all var(--duration-fast, 0.1s);
}

.emoji-btn:hover {
  border-color: var(--color-primary, #2563eb);
  background: rgba(37, 99, 235, 0.08);
}

.emoji-btn--active {
  border-color: var(--color-primary, #2563eb);
  background: rgba(37, 99, 235, 0.15);
  box-shadow: 0 0 0 2px rgba(37, 99, 235, 0.3);
}

.emoji-btn--reserved {
  opacity: 0.35;
  cursor: not-allowed;
  filter: grayscale(0.7);
}

.emoji-btn--reserved:hover {
  border-color: var(--color-border, #28333f);
  background: var(--color-surface, #0b1120);
}

/* Allowed Plans Checklist */
.allowed-plans-checklist {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 200px;
  overflow-y: auto;
  padding: 8px;
  border: 1px solid var(--color-border, #28333f);
  border-radius: 8px;
  background: var(--color-surface, #0b1120);
}

.plan-check {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 0.875rem;
  cursor: pointer;
  padding: 4px 0;
}

.plan-check input[type="checkbox"] {
  width: 16px;
  height: 16px;
  accent-color: var(--color-primary, #3b82f6);
}

.plan-check span {
  color: var(--color-text, #e2e8f0);
}

.empty-hint {
  color: var(--color-text-secondary, #64748b);
  font-size: 0.8rem;
  margin: 0;
}

/* Tag Select in Bulk Assign */
.tag-select-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 240px;
  overflow-y: auto;
  padding: 8px;
  border: 1px solid var(--color-border, #28333f);
  border-radius: 8px;
  background: var(--color-surface, #0b1120);
}

.tag-select-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: background 0.1s;
}

.tag-select-item:hover {
  background: var(--color-surface-2);
}

.tag-select-item--active {
  background: rgba(37, 99, 235, 0.1);
}

.tag-radio {
  width: 16px;
  height: 16px;
  accent-color: var(--color-primary, #3b82f6);
}

.tag-swatch {
  width: 14px;
  height: 14px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--color-border);
  flex-shrink: 0;
}

.tag-select-name {
  font-size: var(--text-sm);
  color: var(--color-text);
}

/* Import Slide-Over */
.import-mode-toggle {
  display: flex;
  gap: 0;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  overflow: hidden;
}

.mode-btn {
  flex: 1;
  padding: var(--space-2) var(--space-3);
  border: none;
  background: var(--color-surface);
  color: var(--color-text-muted);
  font-size: var(--text-sm);
  cursor: pointer;
  transition: all 0.15s;
}

.mode-btn:not(:last-child) {
  border-right: 1px solid var(--color-border);
}

.mode-btn--active {
  background: var(--color-primary);
  color: white;
}

.import-file-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.import-file-label {
  display: block;
  cursor: pointer;
}

.import-file-input {
  display: none;
}

.import-file-btn {
  display: inline-flex;
  align-items: center;
  padding: var(--space-2) var(--space-4);
  background: var(--color-surface-2);
  border: 1px dashed var(--color-border);
  border-radius: var(--radius-md);
  color: var(--color-text);
  font-size: var(--text-sm);
  transition: border-color 0.15s;
}

.import-file-btn:hover {
  border-color: var(--color-primary);
}

.import-hint {
  font-size: var(--text-xs);
  color: var(--color-text-muted);
  margin: 0;
}

.import-paste-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.import-textarea {
  width: 100%;
  padding: var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-surface);
  color: var(--color-text);
  font-family: monospace;
  font-size: var(--text-xs);
  resize: vertical;
}

.import-textarea:focus {
  outline: none;
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px var(--color-primary-subtle);
}

.import-preview {
  background: var(--color-surface-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  padding: var(--space-3);
}

.import-preview__title {
  font-size: var(--text-sm);
  font-weight: 600;
  margin: 0 0 var(--space-2);
}

.import-preview__count {
  font-size: var(--text-sm);
  color: var(--color-text-muted);
  margin: 0 0 var(--space-2);
}

.import-errors {
  margin-top: var(--space-2);
}

.import-errors__title {
  font-size: var(--text-xs);
  font-weight: 600;
  color: var(--color-danger, #ef4444);
  margin: 0 0 var(--space-1);
}

.import-errors__list {
  list-style: none;
  padding: 0;
  margin: 0;
  font-size: var(--text-xs);
  color: var(--color-danger, #ef4444);
}

.import-errors__list li {
  padding: 2px 0;
}

.import-preview-table {
  margin-top: var(--space-3);
  overflow-x: auto;
}

.preview-table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--text-xs);
}

.preview-table th,
.preview-table td {
  padding: 4px 8px;
  border: 1px solid var(--color-border);
  text-align: left;
}

.preview-table th {
  background: var(--color-surface);
  font-weight: 600;
}

.preview-more {
  text-align: center;
  color: var(--color-text-muted);
  font-style: italic;
}

.import-result {
  background: var(--color-surface-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  padding: var(--space-3);
}

.import-result__title {
  font-size: var(--text-sm);
  font-weight: 600;
  margin: 0 0 var(--space-2);
}

.import-result__stats {
  display: flex;
  gap: var(--space-3);
  flex-wrap: wrap;
}

.stat {
  font-size: var(--text-sm);
  font-weight: 500;
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-sm);
}

.stat--success {
  color: var(--color-success, #22c55e);
  background: rgba(34, 197, 94, 0.1);
}

.stat--info {
  color: var(--color-primary);
  background: rgba(37, 99, 235, 0.1);
}

.stat--warn {
  color: var(--color-warning, #f59e0b);
  background: rgba(245, 158, 11, 0.1);
}
</style>
