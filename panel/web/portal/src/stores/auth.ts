import { ref, computed } from 'vue'
import { defineStore } from 'pinia'
import { useApi } from '@koris/composables/useApi'

/**
 * Portal customer representation
 */
export interface PortalUser {
  id?: number
  username: string
  display_name?: string
  status?: string
  plan?: string
  credit?: number
  max_data_bytes?: number
  sub_token?: string
  subscription?: {
    plan?: string
    status?: string
    expires_at?: string
  }
}

/**
 * API response types
 */
interface AuthMeResponse {
  ok: boolean
  customer: PortalUser
}

interface LoginResponse {
  ok: boolean
  username: string
  totp_required?: boolean
}

interface LogoutResponse {
  ok: boolean
}

/**
 * Portal authentication store (Pinia Composition API style)
 *
 * Manages customer authentication state including TOTP support.
 * Uses useApi composable for all API interactions.
 *
 * Requirements: 3.2, 3.3, 3.4, 23.1
 */
export const usePortalAuthStore = defineStore('portal-auth', () => {
  // ─── State ────────────────────────────────────────────────────────────────
  const user = ref<PortalUser | null>(null)
  const isAuthenticated = ref(false)
  const totpRequired = ref(false)

  // ─── API composable ───────────────────────────────────────────────────────
  const { get, post, loading, error } = useApi({
    onUnauthorized: () => {
      user.value = null
      isAuthenticated.value = false
    },
  })

  // ─── Computed ─────────────────────────────────────────────────────────────
  const username = computed(() => user.value?.username ?? '')
  const displayName = computed(() => user.value?.display_name || user.value?.username || 'Customer')
  const planName = computed(() => user.value?.subscription?.plan || user.value?.plan || 'None')
  const status = computed(() => user.value?.subscription?.status || user.value?.status || 'inactive')
  const credit = computed(() => user.value?.credit ?? 0)

  // ─── Actions ──────────────────────────────────────────────────────────────

  /**
   * Check current authentication status.
   * GET /api/portal/me → { ok, customer }
   */
  async function checkAuth(): Promise<void> {
    try {
      const res = await get<AuthMeResponse>('/api/portal/me')
      user.value = res.customer
      isAuthenticated.value = true
    } catch {
      user.value = null
      isAuthenticated.value = false
    }
  }

  /**
   * Login with username, password, and optional TOTP code.
   * POST /api/auth/customer → { ok, username, totp_required? }
   */
  async function login(params: { username: string; password: string; totp_code?: string }): Promise<boolean> {
    try {
      const res = await post<LoginResponse>('/api/auth/customer', params)

      if (res.totp_required) {
        totpRequired.value = true
        return false
      }

      totpRequired.value = false
      await checkAuth()
      return true
    } catch {
      return false
    }
  }

  /**
   * Logout the current customer.
   * POST /api/auth/customer/logout
   */
  async function logout(): Promise<void> {
    try {
      await post<LogoutResponse>('/api/auth/customer/logout')
    } catch {
      // Ignore logout errors — always clear local state
    } finally {
      user.value = null
      isAuthenticated.value = false
      totpRequired.value = false
    }
  }

  /**
   * Update profile data.
   * PATCH /api/portal/profile → { ok }
   */
  async function updateProfile(params: { display_name?: string; password?: string; current_password?: string }): Promise<boolean> {
    try {
      await post<{ ok: boolean }>('/api/portal/profile', params)
      // Reload user data
      await checkAuth()
      return true
    } catch {
      return false
    }
  }

  // ─── Expose ───────────────────────────────────────────────────────────────
  return {
    // State
    user,
    isAuthenticated,
    totpRequired,

    // Computed
    username,
    displayName,
    planName,
    status,
    credit,

    // API state
    loading,
    error,

    // Actions
    checkAuth,
    login,
    logout,
    updateProfile,
  }
})
