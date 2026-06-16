import { ref } from 'vue'
import { defineStore } from 'pinia'
import { useApi } from '@koris/composables/useApi'
import { useAuthStore } from '@/stores/auth'
import router from '@/router'

/**
 * Reseller entity matching the backend data model
 */
export interface Reseller {
  id: number
  username: string
  credit: number
  created_at: string
}

/**
 * Reseller transaction record
 */
export interface ResellerTransaction {
  id: number
  reseller_id: number
  amount: number
  description: string
  created_at: string
}

/**
 * Reseller creation payload matching POST /api/resellers
 */
export interface CreateResellerPayload {
  username: string
  password: string
}

/**
 * API response types matching backend endpoints
 */
interface ResellersListResponse {
  ok: boolean
  resellers: Reseller[]
}

interface ResellerTransactionsResponse {
  ok: boolean
  transactions: ResellerTransaction[]
}

interface ResellerMutationResponse {
  ok: boolean
  id?: number
}

/**
 * Resellers management store (Pinia Composition API style)
 *
 * Manages reseller accounts including CRUD operations and credit adjustments.
 * Uses useApi composable for all API interactions with loading state management.
 *
 * Requirements: 3.1, 3.3, 22.7
 */
export const useResellersStore = defineStore('resellers', () => {
  // ─── State ────────────────────────────────────────────────────────────────
  const list = ref<Reseller[]>([])
  const transactions = ref<ResellerTransaction[]>([])
  const loading = ref(false)

  // ─── API composable ───────────────────────────────────────────────────────
  const { get, post, del, error } = useApi({
    onUnauthorized: () => {
      // On 401, clear auth state and redirect to login
      const auth = useAuthStore()
      auth.user = null
      auth.isAuthenticated = false
      router.push({ name: 'login' })
    },
  })

  // ─── Actions ──────────────────────────────────────────────────────────────

  /**
   * Load all resellers and their transactions from the API.
   * GET /api/resellers → { ok: boolean, resellers: Reseller[] }
   * GET /api/resellers/transactions → { ok: boolean, transactions: ResellerTransaction[] }
   *
   * Sets loading = true before request, false after (success or failure).
   * On error, preserves existing data (Requirement 3.4).
   */
  async function loadResellers(): Promise<void> {
    loading.value = true
    try {
      const [resellersRes, txRes] = await Promise.all([
        get<ResellersListResponse>('/api/resellers'),
        get<ResellerTransactionsResponse>('/api/resellers/transactions'),
      ])
      list.value = resellersRes.resellers || []
      transactions.value = txRes.transactions || []
    } catch {
      // Preserve existing data on error (Requirement 3.4)
    } finally {
      loading.value = false
    }
  }

  /**
   * Create a new reseller account.
   * POST /api/resellers with { username, password }
   *
   * On success, reloads the resellers list.
   * On error, preserves existing data.
   */
  async function createReseller(username: string, password: string): Promise<boolean> {
    loading.value = true
    try {
      await post<ResellerMutationResponse>('/api/resellers', { username, password })
      await loadResellers()
      return true
    } catch {
      // Preserve existing data on error (Requirement 3.4)
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * Adjust a reseller's credit balance.
   * POST /api/resellers/:id/credit with { amount }
   *
   * Positive amount adds credit; negative amount deducts credit.
   * On success, reloads the resellers list.
   * On error, preserves existing data.
   */
  async function adjustCredit(id: number, amount: number): Promise<boolean> {
    loading.value = true
    try {
      await post<ResellerMutationResponse>(`/api/resellers/${id}/credit`, { amount })
      await loadResellers()
      return true
    } catch {
      // Preserve existing data on error (Requirement 3.4)
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * Delete a reseller account.
   * DELETE /api/resellers/:id
   *
   * On success, reloads the resellers list.
   * On error, preserves existing data.
   */
  async function deleteReseller(id: number): Promise<boolean> {
    loading.value = true
    try {
      await del<ResellerMutationResponse>(`/api/resellers/${id}`)
      await loadResellers()
      return true
    } catch {
      // Preserve existing data on error (Requirement 3.4)
      return false
    } finally {
      loading.value = false
    }
  }

  // ─── Expose ───────────────────────────────────────────────────────────────
  return {
    // State
    list,
    transactions,
    loading,

    // API state (from useApi)
    error,

    // Actions
    loadResellers,
    createReseller,
    adjustCredit,
    deleteReseller,
  }
})
