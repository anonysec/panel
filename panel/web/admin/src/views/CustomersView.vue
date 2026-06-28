<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useCustomersStore } from '@/stores/customers'
import { useResellersStore } from '@/stores/resellers'
import { usePlansStore } from '@/stores/plans'
import { useRealtimeStore } from '@/stores/realtime'
import { useAuthStore } from '@/stores/auth'
import type { BulkActionRequest } from '@/stores/customers'
import KDataTable from '@koris/ui/KDataTable.vue'
import KButton from '@koris/ui/KButton.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KInput from '@koris/ui/KInput.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KSelect from '@koris/ui/KSelect.vue'
import KSlideOver from '@koris/ui/KSlideOver.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'
import { useDebounceFn } from '@vueuse/core'
import { useConfirm } from '@koris/composables/useConfirm'
import { useToast } from '@koris/composables/useToast'
import { useI18n } from '@koris/composables/useI18n'
import { useApi } from '@koris/composables/useApi'
import { formatDate } from '@koris/composables/useFormatDate'
import { useVirtualScroll } from '@koris/composables/useVirtualScroll'
import { useDetailPanel } from '@/composables/useDetailPanel'
import { useExpandableRows } from '@/composables/useExpandableRows'
import { useQuickActions } from '@/composables/useQuickActions'
import UserDetailPanel from '@/components/users/UserDetailPanel.vue'
import WalletActions from '@/components/users/WalletActions.vue'
import UserEditModal from '@/components/users/UserEditModal.vue'
import ExpandableRow from '@/components/users/ExpandableRow.vue'
import RowQuickActions from '@/components/users/RowQuickActions.vue'
import ProfileFields from '@/components/users/ProfileFields.vue'
import type { ProfileFormData } from '@/components/users/ProfileFields.vue'

const { t } = useI18n()
const store = useCustomersStore()
const resellersStore = useResellersStore()
const plansStore = usePlansStore()
const realtime = useRealtimeStore()
const authStore = useAuthStore()
const { confirm } = useConfirm()
const toast = useToast()
const api = useApi()

const isReseller = computed(() => authStore.user?.role === 'reseller')

// ─── Detail Panel (CSS Grid layout) ────────────────────────────────────────
const { selectedUserId, isOpen: detailPanelOpen, open: openDetailPanel, close: closeDetailPanel, switchUser } = useDetailPanel()
const detailPanelRef = ref<InstanceType<typeof UserDetailPanel> | null>(null)

// ─── Wallet Modal State ─────────────────────────────────────────────────────
const walletModalOpen = ref(false)
const walletMode = ref<'top-up' | 'deduct'>('top-up')
const walletUsername = ref('')
const walletBalance = ref(0)

function openWalletModal(mode: 'top-up' | 'deduct', username: string, balance: number) {
  walletMode.value = mode
  walletUsername.value = username
  walletBalance.value = balance
  walletModalOpen.value = true
}

function closeWalletModal() {
  walletModalOpen.value = false
}

function onWalletSuccess() {
  walletModalOpen.value = false
  // Refresh the detail panel data to reflect updated balance and transactions
  detailPanelRef.value?.refresh()
}

// ─── Edit Modal State ───────────────────────────────────────────────────────
const editModalOpen = ref(false)
const editModalUserId = ref<number | null>(null)

function openEditModal(userId: number | null = selectedUserId.value) {
  editModalUserId.value = userId
  editModalOpen.value = true
}

function closeEditModal() {
  editModalOpen.value = false
  editModalUserId.value = null
}

function onEditModalSaved() {
  editModalOpen.value = false
  editModalUserId.value = null
  // Refresh the customers list after edit
  store.loadCustomers()
}

function onDetailPanelEdit() {
  openEditModal(selectedUserId.value)
}

// ─── Expandable Rows — MOBILE ONLY (Requirements: 6.1, 6.2, 6.5, 6.7) ─────
const { expandedIds, toggle: toggleExpandRow, isExpanded: isRowExpanded, collapseAll } = useExpandableRows()

/** Whether we're on mobile (expandable rows active) */
const isMobileView = ref(window.innerWidth <= 1024)

function checkMobileView() {
  isMobileView.value = window.innerWidth <= 1024
}

onMounted(() => {
  window.addEventListener('resize', checkMobileView)
})

onUnmounted(() => {
  window.removeEventListener('resize', checkMobileView)
})

// ─── Quick Actions (Requirements: 13.1, 13.5, 13.6) ────────────────────────
const quickActions = useQuickActions()

/** Tracks which row is currently hovered (for RowQuickActions visibility) */
const hoveredRowId = ref<number | null>(null)

function onRowMouseEnter(rowId: number) {
  hoveredRowId.value = rowId
}

function onRowMouseLeave() {
  hoveredRowId.value = null
}

/** Handle expand row edit icon → opens UserEditModal (not detail panel) (Req 6.5) */
function handleExpandedRowEdit(userId: number) {
  openEditModal(userId)
}

/** Handle quick action: toggle status (Req 13.1, 13.2) */
function handleQuickToggleStatus(user: any) {
  quickActions.toggleStatus(user.id, user.status)
}

/** Handle quick action: reset traffic (Req 13.1, 13.2) */
function handleQuickResetTraffic(userId: number) {
  quickActions.resetTraffic(userId)
}

/** Handle quick action: delete with confirmation (Req 13.3) */
async function handleQuickDelete(user: any) {
  const confirmed = await confirm({
    title: t('customers.confirm_delete_title'),
    message: t('customers.confirm_delete_msg').replace('{name}', user.username),
    variant: 'danger',
    icon: '\u26A0',
    confirmText: t('btn.delete'),
    cancelText: t('btn.cancel'),
  })
  if (!confirmed) return
  quickActions.deleteUser(user.id)
}

// ─── Virtual Scroll ─────────────────────────────────────────────────────────
const VIRTUAL_SCROLL_THRESHOLD = 100
const ROW_HEIGHT = 44
const tableContainerRef = ref<HTMLElement | null>(null)
const containerHeight = ref(600)

/** Whether virtual scrolling is active (list > 100 items) */
const virtualScrollEnabled = computed(() => tableData.value.length > VIRTUAL_SCROLL_THRESHOLD)

const totalItems = computed(() => tableData.value.length)

const {
  startIndex,
  endIndex,
  offsetY,
  totalHeight,
  onScroll: handleVirtualScroll,
} = useVirtualScroll({
  totalItems,
  rowHeight: ROW_HEIGHT,
  containerHeight,
  bufferSize: 5,
})

/** The data slice rendered in the virtual scroll viewport */
const visibleTableData = computed(() => {
  if (!virtualScrollEnabled.value) return tableData.value
  return tableData.value.slice(startIndex.value, endIndex.value + 1)
})

// Observe container height for virtual scroll
let resizeObserver: ResizeObserver | null = null

onMounted(() => {
  resizeObserver = new ResizeObserver((entries) => {
    for (const entry of entries) {
      containerHeight.value = entry.contentRect.height
    }
  })
})

onUnmounted(() => {
  resizeObserver?.disconnect()
})

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

const newUserProfileData = ref<ProfileFormData>({
  username: '',
  status: 'active',
  plan_id: '',
  data_limit: '',
  expiry_date: '',
  note: '',
  allowed_protocols: [],
  protocol_options: {},
  billing_enabled: false,
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
    { key: 'expand', label: '', sortable: false, width: '40px' } as any,
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
    { key: 'actions', label: '', sortable: false, align: 'right' as const, width: '200px' } as any,
  )
  // Filter by visibility
  const visibleKeys = new Set(columnVisibility.value.filter(c => c.visible).map(c => c.key))
  return allCols.filter(col => col.key === 'actions' || col.key === 'expand' || visibleKeys.has(col.key))
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

/** Whether no results match the current search/filter criteria */
const noResultsMatch = computed(() => {
  return !store.loading && tableData.value.length === 0 && (searchQuery.value.trim() !== '' || activeStatusTab.value !== 'all' || filterPlanId.value !== '')
})

/** Reset all active search and filter values */
function resetFilters() {
  searchQuery.value = ''
  store.filters.search = ''
  activeStatusTab.value = 'all'
  store.filters.status = 'all'
  filterPlanId.value = ''
  filterDateFrom.value = ''
  filterDateTo.value = ''
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
  if (detailPanelOpen.value) {
    switchUser(row.id)
  } else {
    openDetailPanel(row.id)
  }
}

function openNewUserSlideOver() {
  // Default to the first plan (typically "Pay as Go")
  const defaultPlanId = planOptions.value.length > 0 ? String(planOptions.value[0].value) : ''
  userForm.value = { username: '', password: '', display_name: '', plan_id: defaultPlanId, data_gb: '', speed_mbps: '', days: '', template_id: '', avatar: '' }
  newUserProfileData.value = {
    username: '',
    status: 'active',
    plan_id: defaultPlanId,
    data_limit: '',
    expiry_date: '',
    note: '',
    allowed_protocols: [],
    protocol_options: {},
    billing_enabled: false,
    avatar: '',
  }
  showUserSlideOver.value = true
}

async function handleCreateUser() {
  if (!userForm.value.username || !userForm.value.password) return
  saving.value = true
  const success = await store.createCustomer({
    username: userForm.value.username,
    password: userForm.value.password,
    display_name: userForm.value.display_name,
    plan_id: newUserProfileData.value.plan_id ? Number(newUserProfileData.value.plan_id) : 0,
    data_gb: newUserProfileData.value.data_limit ? Number(newUserProfileData.value.data_limit) : 0,
    speed_mbps: userForm.value.speed_mbps ? Number(userForm.value.speed_mbps) : 0,
    days: userForm.value.days ? Number(userForm.value.days) : 0,
    template_id: userForm.value.template_id ? Number(userForm.value.template_id) : undefined,
    avatar: newUserProfileData.value.avatar || undefined,
    status: newUserProfileData.value.status || 'active',
    expiry_date: newUserProfileData.value.expiry_date || undefined,
    note: newUserProfileData.value.note || undefined,
    allowed_protocols: newUserProfileData.value.allowed_protocols || [],
    protocol_options: newUserProfileData.value.protocol_options || {},
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
  <div :class="['page', 'customers-view', { 'customers-view--panel-open': detailPanelOpen }]">
    <!-- Main content area (table side) -->
    <div class="customers-view__main">
    <!-- Header -->
    <header class="page-header">
      <div class="page-header__actions">
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

      <!-- Filter Row: Status tabs + Search + Filter/Columns buttons -->
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
        <div class="filter-row__inline-actions">
          <button
            type="button"
            class="filter-row__icon-btn"
            :class="{ 'filter-row__icon-btn--active': showAdvancedFilters }"
            :title="t('customers.advanced_filters')"
            @click="showAdvancedFilters = !showAdvancedFilters"
          >
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden="true">
              <path d="M1.5 2.5h11M3.5 5.5h7M5.5 8.5h3M6.5 11.5h1" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/>
            </svg>
          </button>
          <button
            type="button"
            class="filter-row__icon-btn"
            :class="{ 'filter-row__icon-btn--active': showColumnToggle }"
            :title="t('customers.columns')"
            @click="showColumnToggle = !showColumnToggle"
          >
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden="true">
              <path d="M2 2h3v10H2zM6 2h3v10H6zM10 2h2v10h-2z" stroke="currentColor" stroke-width="1.1" stroke-linecap="round" stroke-linejoin="round"/>
            </svg>
          </button>
        </div>
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
      <!-- Loading state: KSkeleton rows (Req 12.3) -->
      <div v-if="store.loading && tableData.length === 0" class="skeleton-table" aria-busy="true" aria-label="Loading users">
        <KSkeleton variant="table-row" :count="store.pagination.pageSize" />
      </div>

      <!-- Empty state: no results match (Req 12.6) -->
      <KEmptyState
        v-else-if="noResultsMatch"
        icon="🔍"
        :title="t('customers.no_results_title') || 'No results found'"
        :description="t('customers.no_results_desc') || 'No users match the current search or filter criteria.'"
        :action-text="t('customers.reset_filters') || 'Reset filters'"
        @action="resetFilters"
      />

      <!-- Virtual scroll table container (Req 12.4) -->
      <div
        v-else
        ref="tableContainerRef"
        :class="['table-scroll-container', { 'table-scroll-container--virtual': virtualScrollEnabled }]"
        @scroll="virtualScrollEnabled ? handleVirtualScroll($event) : undefined"
      >
        <!-- Virtual scroll spacer for correct scrollbar height -->
        <div
          v-if="virtualScrollEnabled"
          class="virtual-scroll-spacer"
          :style="{ height: totalHeight + 'px' }"
        >
          <div class="virtual-scroll-content" :style="{ transform: `translateY(${offsetY}px)` }">
            <KDataTable
              :columns="columns"
              :data="visibleTableData"
              :loading="false"
              :page-size="visibleTableData.length"
              row-key="id"
              selectable
              @row-click="handleRowClick"
              @selection-change="onSelectionChange"
            >
              <template #cell-expand="{ row }">
                <button
                  class="expand-chevron"
                  :class="{ 'expand-chevron--expanded': isRowExpanded(row.id) }"
                  :aria-label="isRowExpanded(row.id) ? 'Collapse row' : 'Expand row'"
                  :aria-expanded="isRowExpanded(row.id)"
                  @click.stop="toggleExpandRow(row.id)"
                >
                  <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
                    <path d="M6 4l4 4-4 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
                  </svg>
                </button>
              </template>
              <template #cell-username="{ row, value }">
                <div class="username-cell">
                  <span class="username-cell__emoji" aria-hidden="true">{{ row.avatar || '👤' }}</span>
                  <span class="username-cell__text">{{ value }}</span>
                  <span v-if="onlineUsernames.has(value)" class="online-dot" title="Online" aria-label="Online" />
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
                <div
                  class="row-actions-cell"
                  @mouseenter="onRowMouseEnter(row.id)"
                  @mouseleave="onRowMouseLeave"
                >
                  <RowQuickActions
                    :user="row"
                    :loading="quickActions.isRowLoading(row.id)"
                    :active-action="quickActions.getRowAction(row.id)"
                    @enable="handleQuickToggleStatus(row)"
                    @disable="handleQuickToggleStatus(row)"
                    @reset-traffic="handleQuickResetTraffic(row.id)"
                    @delete="handleQuickDelete(row)"
                  />
                </div>
              </template>
            </KDataTable>
            <!-- Expanded rows (virtual scroll) -->
            <template v-for="row in visibleTableData" :key="'expand-' + row.id">
              <div v-if="isRowExpanded(row.id)" class="expanded-row-wrapper">
                <ExpandableRow
                  :user="row"
                  :expanded="true"
                  @toggle="toggleExpandRow(row.id)"
                  @row-click="handleRowClick(row)"
                  @edit="handleExpandedRowEdit(row.id)"
                />
              </div>
            </template>
          </div>
        </div>

        <!-- Normal (non-virtual) table when items ≤ 100 -->
        <KDataTable
          v-if="!virtualScrollEnabled"
          :columns="columns"
          :data="tableData"
          :loading="store.loading"
          :page-size="store.pagination.pageSize"
          row-key="id"
          selectable
          @row-click="handleRowClick"
          @selection-change="onSelectionChange"
        >
          <template #cell-expand="{ row }">
            <button
              class="expand-chevron"
              :class="{ 'expand-chevron--expanded': isRowExpanded(row.id) }"
              :aria-label="isRowExpanded(row.id) ? 'Collapse row' : 'Expand row'"
              :aria-expanded="isRowExpanded(row.id)"
              @click.stop="toggleExpandRow(row.id)"
            >
              <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
                <path d="M6 4l4 4-4 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
              </svg>
            </button>
          </template>
          <template #cell-username="{ row, value }">
            <div class="username-cell">
              <span class="username-cell__emoji" aria-hidden="true">{{ row.avatar || '👤' }}</span>
              <span class="username-cell__text">{{ value }}</span>
              <span v-if="onlineUsernames.has(value)" class="online-dot" title="Online" aria-label="Online" />
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
            <div
              class="row-actions-cell"
              @mouseenter="onRowMouseEnter(row.id)"
              @mouseleave="onRowMouseLeave"
            >
              <RowQuickActions
                :user="row"
                :loading="quickActions.isRowLoading(row.id)"
                :active-action="quickActions.getRowAction(row.id)"
                @enable="handleQuickToggleStatus(row)"
                @disable="handleQuickToggleStatus(row)"
                @reset-traffic="handleQuickResetTraffic(row.id)"
                @delete="handleQuickDelete(row)"
              />
            </div>
          </template>
        </KDataTable>
        <!-- Expanded rows (normal mode) -->
        <template v-for="row in tableData" :key="'expand-' + row.id">
          <div v-if="isRowExpanded(row.id)" class="expanded-row-wrapper">
            <ExpandableRow
              :user="row"
              :expanded="true"
              @toggle="toggleExpandRow(row.id)"
              @row-click="handleRowClick(row)"
              @edit="handleExpandedRowEdit(row.id)"
            />
          </div>
        </template>
      </div>
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

    </div><!-- /.customers-view__main -->

    <!-- Detail Panel (slides in from right, Requirements: 2.1, 2.2, 2.3, 2.9) -->
    <UserDetailPanel
      ref="detailPanelRef"
      :user-id="selectedUserId"
      :open="detailPanelOpen"
      @close="closeDetailPanel"
      @edit="onDetailPanelEdit"
      @updated="store.loadCustomers()"
      @top-up="(username: string, balance: number) => openWalletModal('top-up', username, balance)"
      @deduct="(username: string, balance: number) => openWalletModal('deduct', username, balance)"
    />

    <!-- Wallet Actions Modal (Top Up / Deduct — Requirements: 11.2, 11.4, 11.6, 11.8, 11.10) -->
    <WalletActions
      :open="walletModalOpen"
      :mode="walletMode"
      :username="walletUsername"
      :current-balance="walletBalance"
      @close="closeWalletModal"
      @success="onWalletSuccess"
    />

    <!-- Edit Modal (Requirements: 5.1, 5.2, 5.5, 5.6) -->
    <UserEditModal
      :open="editModalOpen"
      :user-id="editModalUserId"
      @close="closeEditModal"
      @saved="onEditModalSaved"
    />

    <!-- ═══════════════ SLIDE-OVERS ═══════════════ -->

    <!-- New User Slide-Over -->
    <KSlideOver :open="showUserSlideOver" :title="t('customers.new_user')" @close="showUserSlideOver = false">
      <form class="slide-form" autocomplete="off" @submit.prevent="handleCreateUser">
        <!-- Row: Username + Password side by side -->
        <div class="slide-form__row">
          <KFormField name="user-username" :label="t('user.username')" required>
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="userForm.username" autocomplete="off" placeholder="username" />
            </template>
          </KFormField>
          <KFormField name="user-password" :label="t('user.password')" required>
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="userForm.password" type="password" autocomplete="new-password" placeholder="••••••" />
            </template>
          </KFormField>
        </div>
        <!-- ProfileFields for the rest (data limit, expiry, protocols, note, billing) -->
        <ProfileFields
          :model-value="newUserProfileData"
          mode="create"
          @update:model-value="newUserProfileData = $event"
        />
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
      <form class="slide-form" autocomplete="off" @submit.prevent="handleResellerSubmit">
        <KFormField name="reseller-username" :label="t('resellers.username')" required>
          <template #default="{ fieldId }">
            <KInput :id="fieldId" v-model="resellerForm.username" autocomplete="off" placeholder="reseller_name" :disabled="!!editingResellerId" />
          </template>
        </KFormField>
        <KFormField name="reseller-password" :label="t('resellers.password')" :required="!editingResellerId">
          <template #default="{ fieldId }">
            <KInput
              :id="fieldId"
              v-model="resellerForm.password"
              type="password"
              autocomplete="new-password"
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

  </div>
</template>

<style scoped>
/* ─── CSS Grid Layout (table + panel side-by-side) ─── */
.customers-view {
  display: grid;
  grid-template-columns: 1fr;
  gap: 0;
  height: 100%;
  overflow: hidden;
  transition: grid-template-columns var(--transition-panel, 280ms ease-out);
}

.customers-view--panel-open {
  grid-template-columns: 1fr auto;
}

.customers-view__main {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  overflow: hidden;
  min-width: 0;
  padding: var(--space-2) var(--space-4);
}

.customers-view__panel {
  width: 480px;
  max-width: 480px;
  border-left: 1px solid var(--color-border);
  background: var(--color-surface);
  overflow-y: auto;
  animation: panel-slide-in 280ms ease-out;
  /* Hidden — panel uses Teleport to body */
  display: none;
}

@keyframes panel-slide-in {
  from { transform: translateX(100%); opacity: 0; }
  to { transform: translateX(0); opacity: 1; }
}

.page-header { display: flex; align-items: center; justify-content: flex-end; gap: var(--space-2); padding: var(--space-1) 0; }
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
  margin-bottom: 0;
}

.main-tab {
  padding: var(--space-2) var(--space-4);
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

/* Inline icon buttons for filters/columns */
.filter-row__inline-actions {
  display: flex;
  gap: var(--space-1);
  flex-shrink: 0;
}

.filter-row__icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  padding: 0;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-muted);
  cursor: pointer;
  transition: all var(--duration-fast);
}

.filter-row__icon-btn:hover {
  color: var(--color-text);
  border-color: var(--color-primary);
  background: rgba(37, 99, 235, 0.06);
}

.filter-row__icon-btn--active {
  color: var(--color-primary);
  border-color: var(--color-primary);
  background: rgba(37, 99, 235, 0.1);
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

/* Username cell with avatar emoji */
.username-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.username-cell__emoji {
  font-size: 16px;
  line-height: 1;
  flex-shrink: 0;
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

/* ─── Virtual Scroll Container ─── */
.table-scroll-container {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.table-scroll-container--virtual {
  overflow-y: auto;
  position: relative;
}

.virtual-scroll-spacer {
  position: relative;
  width: 100%;
}

.virtual-scroll-content {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
}

/* ─── Skeleton Loading ─── */
.skeleton-table {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-3) 0;
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
  min-height: 100%;
}

.slide-form__footer {
  display: flex;
  justify-content: flex-end;
  gap: var(--space-2);
  padding-top: var(--space-4);
  padding-bottom: var(--space-4);
  border-top: 1px solid var(--color-border);
  margin-top: auto;
  position: sticky;
  bottom: 0;
  background: var(--color-surface, #0b1120);
  z-index: 1;
}

.slide-form__row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-3);
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
  .customers-view {
    transition: none;
  }
  .customers-view__panel {
    animation: none;
  }
}

/* Panel responsive: full width overlay on ≤ 1024px (Req 2.7) */
@media (max-width: 1024px) {
  .customers-view--panel-open {
    grid-template-columns: 1fr;
  }

  .customers-view__panel {
    position: fixed;
    top: 0;
    right: 0;
    bottom: 0;
    width: 100%;
    max-width: 100%;
    z-index: 100;
    box-shadow: -4px 0 24px rgba(0, 0, 0, 0.3);
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

/* ─── Expand Chevron (Req 6.1) ─── */
.expand-chevron {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  padding: 0;
  border: none;
  border-radius: var(--radius-sm, 4px);
  background: transparent;
  color: var(--color-muted, #8b98a5);
  cursor: pointer;
  transition: transform var(--transition-row-expand, 200ms ease-out), color 100ms ease-out, background 100ms ease-out;
}

/* Hide expand column on desktop (viewport > 1024px) */
@media (min-width: 1025px) {
  .expand-chevron {
    display: none;
  }
  :deep(th:first-child),
  :deep(td:first-child) {
    display: none;
  }
}

.expand-chevron:hover {
  background: var(--color-surface-2, #1e2630);
  color: var(--color-text, #e6edf3);
}

.expand-chevron:focus-visible {
  outline: 2px solid var(--color-primary, #2563eb);
  outline-offset: 2px;
}

.expand-chevron--expanded {
  transform: rotate(90deg);
  color: var(--color-primary, #2563eb);
}

/* ─── Row Actions Cell (Quick Actions on hover, Req 13.1) ─── */
.row-actions-cell {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  min-width: 180px;
  min-height: 32px;
  position: relative;
}

/* ─── Expanded Row Wrapper ─── */
.expanded-row-wrapper {
  padding: 0 var(--space-4, 16px);
  animation: expand-row-in var(--transition-row-expand, 200ms ease-out);
}

@keyframes expand-row-in {
  from {
    opacity: 0;
    max-height: 0;
    overflow: hidden;
  }
  to {
    opacity: 1;
    max-height: 200px;
    overflow: visible;
  }
}

@media (prefers-reduced-motion: reduce) {
  .expand-chevron {
    transition: none;
  }

  .expanded-row-wrapper {
    animation: none;
  }
}
</style>
