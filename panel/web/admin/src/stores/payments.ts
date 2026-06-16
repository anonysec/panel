import { ref, computed } from 'vue'
import { defineStore } from 'pinia'
import { useApi } from '@koris/composables/useApi'
import { useAuthStore } from '@/stores/auth'
import router from '@/router'
import type { Payment } from '@koris/types'

/**
 * Payment method entity matching backend schema
 */
export interface PaymentMethod {
  id: number
  name: string
  type: string
  instructions: string
  is_active: boolean
  sort_order: number
  created_at: string
}

/**
 * Payload for creating a manual payment (POST /api/payments)
 */
export interface CreateManualPaymentPayload {
  username: string
  amount: number
  method: string
  description: string
}

/**
 * Payload for creating/updating a payment method
 */
export interface PaymentMethodPayload {
  name: string
  type: string
  instructions: string
  is_active: boolean
  sort_order: number
}

/**
 * Filters for the payments list
 */
export interface PaymentFilters {
  status: 'all' | 'pending' | 'approved' | 'rejected'
  search: string
}

/**
 * API response types matching backend endpoints
 */
interface PaymentsListResponse {
  ok: boolean
  payments: Payment[]
}

interface PaymentMethodsListResponse {
  ok: boolean
  methods: PaymentMethod[]
}

interface PaymentMutationResponse {
  ok: boolean
  id?: number
}

/**
 * Payments management store (Pinia Composition API style)
 *
 * Manages payment records, approval/rejection actions, manual payment creation,
 * and payment method management for the admin panel.
 * Uses useApi composable for all API interactions with loading state management.
 *
 * Requirements: 3.1, 3.3, 22.5
 */
export const usePaymentsStore = defineStore('payments', () => {
  // ─── State ────────────────────────────────────────────────────────────────
  const list = ref<Payment[]>([])
  const paymentMethods = ref<PaymentMethod[]>([])
  const loading = ref(false)
  const filters = ref<PaymentFilters>({
    status: 'all',
    search: '',
  })

  // ─── Pagination ───────────────────────────────────────────────────────────
  const page = ref(1)
  const pageSize = ref(20)

  // ─── API composable ───────────────────────────────────────────────────────
  const { get, post, patch, del, error } = useApi({
    onUnauthorized: () => {
      // On 401, clear auth state and redirect to login
      const auth = useAuthStore()
      auth.user = null
      auth.isAuthenticated = false
      router.push({ name: 'login' })
    },
  })

  // ─── Computed ─────────────────────────────────────────────────────────────

  /**
   * Filtered payments list based on current filters (status, search)
   */
  const filteredList = computed(() => {
    let result = list.value

    // Filter by status
    if (filters.value.status !== 'all') {
      result = result.filter((p) => p.status === filters.value.status)
    }

    // Filter by search (matches username or method)
    if (filters.value.search.trim()) {
      const query = filters.value.search.trim().toLowerCase()
      result = result.filter(
        (p) =>
          p.username.toLowerCase().includes(query) ||
          p.method.toLowerCase().includes(query)
      )
    }

    return result
  })

  /**
   * Total number of items after filtering (for pagination)
   */
  const totalFiltered = computed(() => filteredList.value.length)

  /**
   * Paginated subset of the filtered list for the current page
   */
  const paginatedList = computed(() => {
    const start = (page.value - 1) * pageSize.value
    const end = start + pageSize.value
    return filteredList.value.slice(start, end)
  })

  /**
   * Active payment methods filtered by is_active === true
   */
  const activePaymentMethods = computed(() =>
    paymentMethods.value.filter((m) => m.is_active === true)
  )

  // ─── Actions ──────────────────────────────────────────────────────────────

  /**
   * Load all payments from the API.
   * GET /api/payments → { ok: boolean, payments: Payment[] }
   *
   * Also loads payment methods from GET /api/payment-methods.
   * Sets loading = true before request, false after (success or failure).
   * On error, preserves existing data (Requirement 3.4).
   */
  async function loadPayments(): Promise<void> {
    loading.value = true
    try {
      const [paymentsRes, methodsRes] = await Promise.all([
        get<PaymentsListResponse>('/api/payments'),
        get<PaymentMethodsListResponse>('/api/payment-methods'),
      ])
      list.value = paymentsRes.payments || []
      paymentMethods.value = methodsRes.methods || []
    } catch {
      // Preserve existing data on error (Requirement 3.4)
    } finally {
      loading.value = false
    }
  }

  /**
   * Approve a pending payment.
   * POST /api/payments/:id/approve
   *
   * On success, reloads the payments list.
   * On error, preserves existing data.
   */
  async function approvePayment(id: number): Promise<boolean> {
    loading.value = true
    try {
      await post<PaymentMutationResponse>(`/api/payments/${id}/approve`)
      await loadPayments()
      return true
    } catch {
      // Preserve existing data on error (Requirement 3.4)
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * Reject a pending payment.
   * POST /api/payments/:id/reject
   *
   * On success, reloads the payments list.
   * On error, preserves existing data.
   */
  async function rejectPayment(id: number): Promise<boolean> {
    loading.value = true
    try {
      await post<PaymentMutationResponse>(`/api/payments/${id}/reject`)
      await loadPayments()
      return true
    } catch {
      // Preserve existing data on error (Requirement 3.4)
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * Create a manual payment record.
   * POST /api/payments with { username, amount, method, description }
   *
   * On success, reloads the payments list.
   * On error, preserves existing data.
   */
  async function createManualPayment(data: CreateManualPaymentPayload): Promise<boolean> {
    loading.value = true
    try {
      await post<PaymentMutationResponse>('/api/payments', data)
      await loadPayments()
      return true
    } catch {
      // Preserve existing data on error (Requirement 3.4)
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * Save (create or update) a payment method.
   * POST /api/payment-methods for new methods.
   * PATCH /api/payment-methods/:id for existing methods.
   *
   * On success, reloads the payments list (which includes methods).
   * On error, preserves existing data.
   */
  async function savePaymentMethod(
    payload: PaymentMethodPayload,
    id?: number
  ): Promise<boolean> {
    loading.value = true
    try {
      if (id) {
        await patch<PaymentMutationResponse>(`/api/payment-methods/${id}`, payload)
      } else {
        await post<PaymentMutationResponse>('/api/payment-methods', payload)
      }
      await loadPayments()
      return true
    } catch {
      // Preserve existing data on error (Requirement 3.4)
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * Deactivate (delete) a payment method.
   * DELETE /api/payment-methods/:id
   *
   * On success, reloads the payments list (which includes methods).
   * On error, preserves existing data.
   */
  async function deactivatePaymentMethod(id: number): Promise<boolean> {
    loading.value = true
    try {
      await del<PaymentMutationResponse>(`/api/payment-methods/${id}`)
      await loadPayments()
      return true
    } catch {
      // Preserve existing data on error (Requirement 3.4)
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * Set the current page for pagination.
   * Resets to page 1 if out of bounds.
   */
  function setPage(newPage: number): void {
    const maxPage = Math.max(1, Math.ceil(totalFiltered.value / pageSize.value))
    page.value = Math.max(1, Math.min(newPage, maxPage))
  }

  /**
   * Update filters and reset to page 1.
   */
  function setFilters(newFilters: Partial<PaymentFilters>): void {
    if (newFilters.status !== undefined) {
      filters.value.status = newFilters.status
    }
    if (newFilters.search !== undefined) {
      filters.value.search = newFilters.search
    }
    page.value = 1
  }

  // ─── Expose ───────────────────────────────────────────────────────────────
  return {
    // State
    list,
    paymentMethods,
    loading,
    filters,

    // Pagination
    page,
    pageSize,

    // Computed
    filteredList,
    totalFiltered,
    paginatedList,
    activePaymentMethods,

    // API state (from useApi)
    error,

    // Actions
    loadPayments,
    approvePayment,
    rejectPayment,
    createManualPayment,
    savePaymentMethod,
    deactivatePaymentMethod,
    setPage,
    setFilters,
  }
})
