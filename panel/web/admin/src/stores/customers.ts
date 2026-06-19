import { ref, computed } from 'vue'
import { defineStore } from 'pinia'
import { useApi } from '@koris/composables/useApi'
import type { Customer, CustomerDetail } from '@koris/types'

/**
 * Deleted customer — extends Customer with a deleted_at timestamp
 */
export interface DeletedCustomer extends Customer {
  deleted_at: string
}

/**
 * Usage session representing an active or historical VPN connection
 */
export interface UsageSession {
  id: number
  username: string
  start_time: string
  update_time: string
  stop_time: string
  session_seconds: number
  input_bytes: number
  output_bytes: number
  total_bytes: number
  framed_ip: string
  calling_station_id: string
  terminate_cause: string
  online: boolean
}

/**
 * Summary of a customer's usage data
 */
export interface UsageSummary {
  online: boolean
  active_sessions: number
  total_input_bytes: number
  total_output_bytes: number
  total_usage_bytes: number
  max_data_bytes: number
  remaining_bytes?: number
  last_connected_at: string
  last_disconnected_at: string
  sessions: UsageSession[]
}

/**
 * Customer creation payload matching POST /api/customers
 */
export interface CreateCustomerPayload {
  username: string
  password: string
  display_name: string
  plan_id: number
  data_gb: number
  speed_mbps: number
  days: number
  template_id?: number
}

/**
 * Customer update payload matching PATCH /api/customers/:id
 */
export interface UpdateCustomerPayload {
  display_name?: string
  status?: string
  plan_id?: number
  notes?: string
  data_gb?: number
  speed_mbps?: number
  days?: number
  allowed_protocols?: string[]
}

/**
 * Bulk action request payload matching POST /api/customers/bulk
 * Requirements: 2.2, 2.3, 2.4, 2.5
 */
export interface BulkActionRequest {
  customer_ids: number[]
  action: 'enable' | 'disable' | 'delete' | 'traffic_reset'
}

/**
 * Bulk action response from POST /api/customers/bulk
 * Requirements: 2.7
 */
export interface BulkActionResponse {
  ok: boolean
  succeeded: number[]
  failed: { customer_id: number; error: string }[]
}

/**
 * Filters for the customer list
 */
export interface CustomerFilters {
  search: string
  status: 'all' | 'active' | 'archived' | 'online' | 'limited' | 'disabled' | 'expired'
  sortBy: string
  sortDir: 'asc' | 'desc'
}

/**
 * Pagination state
 */
export interface CustomerPagination {
  page: number
  pageSize: number
  total: number
}

/**
 * API response types matching backend endpoints
 */
interface CustomersListResponse {
  ok: boolean
  customers: Customer[]
}

interface DeletedCustomersListResponse {
  ok: boolean
  customers: DeletedCustomer[]
}

interface CustomerDetailResponse {
  ok: boolean
  customer: CustomerDetail
}

interface CustomerUsageResponse {
  ok: boolean
  usage: UsageSummary
}

interface CustomerMutationResponse {
  ok: boolean
  id?: number
}

interface TrafficResetResponse {
  ok: boolean
}

interface ConnectionLimitResponse {
  ok: boolean
  connection_limit: number
}

/**
 * Customers management store (Pinia Composition API style)
 *
 * Manages customer list, detail, usage, and CRUD operations.
 * Implements client-side filtering, search, and sorting via computed `filteredList`.
 * Uses useApi composable for all API interactions with loading state management.
 *
 * Requirements: 3.1, 3.3, 3.4, 22.2, 22.3
 */
export const useCustomersStore = defineStore('customers', () => {
  // ─── State ────────────────────────────────────────────────────────────────
  const list = ref<Customer[]>([])
  const deleted = ref<DeletedCustomer[]>([])
  const detail = ref<CustomerDetail | null>(null)
  const usage = ref<UsageSummary | null>(null)
  const loading = ref(false)
  const detailLoading = ref(false)

  const filters = ref<CustomerFilters>({
    search: '',
    status: 'all',
    sortBy: 'created_at',
    sortDir: 'desc',
  })

  const pagination = ref<CustomerPagination>({
    page: 1,
    pageSize: 20,
    total: 0,
  })

  // ─── API composable ───────────────────────────────────────────────────────
  // No onUnauthorized handler — the router guard handles auth redirects.
  // This prevents race conditions where a 401 during initial data load
  // would clear auth state and cause a redirect loop after login.
  const { get, post, patch, del, error } = useApi()

  // ─── Computed ─────────────────────────────────────────────────────────────

  /**
   * Filtered and sorted customer list.
   * Applies search (username/display_name), status filter, and sorting.
   *
   * Requirements: 22.2 (filterable, searchable)
   */
  const filteredList = computed(() => {
    let result: Customer[]

    // Status filter: select the appropriate source list
    if (filters.value.status === 'archived') {
      result = [...deleted.value]
    } else if (filters.value.status === 'all') {
      result = [...list.value]
    } else {
      result = list.value.filter((c) => c.status === filters.value.status)
    }

    // Search filter: match against username, display_name, plan, status, id, credit
    const query = filters.value.search.trim().toLowerCase()
    if (query) {
      result = result.filter(
        (c) =>
          c.username.toLowerCase().includes(query) ||
          c.display_name.toLowerCase().includes(query) ||
          (c.plan ?? '').toLowerCase().includes(query) ||
          c.status.toLowerCase().includes(query) ||
          String(c.id).includes(query) ||
          String(c.credit).includes(query)
      )
    }

    // Sorting
    const { sortBy, sortDir } = filters.value
    const direction = sortDir === 'asc' ? 1 : -1

    result.sort((a, b) => {
      const aVal = a[sortBy as keyof Customer]
      const bVal = b[sortBy as keyof Customer]

      if (aVal == null && bVal == null) return 0
      if (aVal == null) return 1
      if (bVal == null) return -1

      if (typeof aVal === 'string' && typeof bVal === 'string') {
        return aVal.localeCompare(bVal) * direction
      }

      if (typeof aVal === 'number' && typeof bVal === 'number') {
        return (aVal - bVal) * direction
      }

      return String(aVal).localeCompare(String(bVal)) * direction
    })

    // Update total count for pagination
    pagination.value.total = result.length

    return result
  })

  /**
   * Paginated slice of the filteredList for display.
   */
  const paginatedList = computed(() => {
    const start = (pagination.value.page - 1) * pagination.value.pageSize
    return filteredList.value.slice(start, start + pagination.value.pageSize)
  })

  // ─── Actions ──────────────────────────────────────────────────────────────

  /**
   * Load all customers (active and deleted) from the API.
   * GET /api/customers → { ok: boolean, customers: Customer[] }
   * GET /api/deleted/customers → { ok: boolean, customers: DeletedCustomer[] }
   *
   * Sets loading = true before request, false after (success or failure).
   * On error, preserves existing data (Requirement 3.4).
   */
  async function loadCustomers(): Promise<void> {
    loading.value = true
    try {
      const [customersRes, deletedRes] = await Promise.all([
        get<CustomersListResponse>('/api/customers'),
        get<DeletedCustomersListResponse>('/api/deleted/customers'),
      ])
      list.value = customersRes.customers || []
      deleted.value = deletedRes.customers || []
    } catch {
      // Preserve existing data on error (Requirement 3.4)
    } finally {
      loading.value = false
    }
  }

  /**
   * Load a specific customer's detail and usage data.
   * GET /api/customers/:id → { ok: boolean, customer: CustomerDetail }
   * GET /api/customers/:id/usage → { ok: boolean, usage: UsageSummary }
   *
   * Sets detailLoading = true before request, false after.
   * On error, preserves existing data (Requirement 3.4).
   */
  async function loadDetail(id: number): Promise<void> {
    detailLoading.value = true
    try {
      const [detailRes, usageRes] = await Promise.all([
        get<CustomerDetailResponse>(`/api/customers/${id}`),
        get<CustomerUsageResponse>(`/api/customers/${id}/usage`),
      ])
      detail.value = detailRes.customer
      usage.value = usageRes.usage
    } catch {
      // Preserve existing data on error (Requirement 3.4)
    } finally {
      detailLoading.value = false
    }
  }

  /**
   * Create a new customer.
   * POST /api/customers with { username, password, display_name, plan_id, data_gb, speed_mbps, days }
   *
   * On success, reloads the customers list.
   * On error, preserves existing data.
   */
  async function createCustomer(payload: CreateCustomerPayload): Promise<boolean> {
    loading.value = true
    try {
      await post<CustomerMutationResponse>('/api/customers', payload)
      await loadCustomers()
      return true
    } catch {
      // Preserve existing data on error (Requirement 3.4)
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * Update an existing customer.
   * PATCH /api/customers/:id with partial customer fields
   *
   * On success, reloads the customer detail.
   * On error, preserves existing data.
   */
  async function updateCustomer(id: number, payload: UpdateCustomerPayload): Promise<boolean> {
    loading.value = true
    try {
      await patch<CustomerMutationResponse>(`/api/customers/${id}`, payload)
      // Reload detail if we're viewing this customer
      if (detail.value?.id === id) {
        await loadDetail(id)
      }
      await loadCustomers()
      return true
    } catch {
      // Preserve existing data on error (Requirement 3.4)
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * Delete (permanently) a customer.
   * Note: In KorisPanel, DELETE /api/customers/:id is actually an archive operation.
   * This action is a hard delete concept — if the API supports it.
   *
   * On success, reloads the customers list.
   * On error, preserves existing data.
   */
  async function deleteCustomer(id: number): Promise<boolean> {
    loading.value = true
    try {
      await del<CustomerMutationResponse>(`/api/customers/${id}`)
      // If we were viewing this customer's detail, clear it
      if (detail.value?.id === id) {
        detail.value = null
        usage.value = null
      }
      await loadCustomers()
      return true
    } catch {
      // Preserve existing data on error (Requirement 3.4)
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * Archive a customer (soft delete — revokes VPN access, customer can be restored).
   * DELETE /api/customers/:id
   *
   * On success, reloads the customers list.
   * On error, preserves existing data.
   */
  async function archiveCustomer(id: number): Promise<boolean> {
    loading.value = true
    try {
      await del<CustomerMutationResponse>(`/api/customers/${id}`)
      // Clear detail if archived customer was being viewed
      if (detail.value?.id === id) {
        detail.value = null
        usage.value = null
      }
      await loadCustomers()
      return true
    } catch {
      // Preserve existing data on error (Requirement 3.4)
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * Execute a bulk action on multiple customers.
   * POST /api/customers/bulk with { customer_ids, action }
   *
   * On success, reloads the customers list.
   * Returns the BulkActionResponse with succeeded/failed arrays, or null on error.
   *
   * Requirements: 2.2, 2.3, 2.4, 2.5, 2.7
   */
  async function bulkAction(request: BulkActionRequest): Promise<BulkActionResponse | null> {
    loading.value = true
    try {
      const response = await post<BulkActionResponse>('/api/customers/bulk', request)
      await loadCustomers()
      return response
    } catch {
      return null
    } finally {
      loading.value = false
    }
  }

  /**
   * Reset traffic counters for a single customer.
   * POST /api/customers/:id/traffic-reset
   *
   * On success, reloads the customer detail.
   * Returns true on success, false on error.
   *
   * Requirements: 3.4
   */
  async function trafficReset(customerId: number): Promise<boolean> {
    detailLoading.value = true
    try {
      await post<TrafficResetResponse>(`/api/customers/${customerId}/traffic-reset`, {})
      // Refetch customer detail to reflect updated counters and status
      await loadDetail(customerId)
      return true
    } catch {
      return false
    } finally {
      detailLoading.value = false
    }
  }

  /**
   * Set the connection limit for a customer.
   * POST /api/customers/:id/connection-limit with { limit }
   *
   * If limit == 0, the backend removes the Simultaneous-Use RADIUS attribute (unlimited).
   * If limit > 0, the backend sets/replaces the Simultaneous-Use attribute.
   *
   * On success, reloads the customer detail.
   * Returns true on success, false on error.
   *
   * Requirements: 4.3
   */
  async function setConnectionLimit(customerId: number, limit: number): Promise<boolean> {
    detailLoading.value = true
    try {
      await post<ConnectionLimitResponse>(`/api/customers/${customerId}/connection-limit`, { limit })
      // Refetch customer detail to reflect updated connection limit
      await loadDetail(customerId)
      return true
    } catch {
      return false
    } finally {
      detailLoading.value = false
    }
  }

  // ─── Expose ───────────────────────────────────────────────────────────────
  return {
    // State
    list,
    deleted,
    detail,
    usage,
    loading,
    detailLoading,
    filters,
    pagination,

    // Computed
    filteredList,
    paginatedList,

    // API state (from useApi)
    error,

    // Actions
    loadCustomers,
    loadDetail,
    createCustomer,
    updateCustomer,
    deleteCustomer,
    archiveCustomer,
    bulkAction,
    trafficReset,
    setConnectionLimit,
  }
})
