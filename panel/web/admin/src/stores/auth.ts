import { ref, computed } from 'vue'
import { defineStore } from 'pinia'
import { useApi } from '@koris/composables/useApi'
import router from '@/router'

/**
 * Auth user representation
 */
export interface AuthUser {
  username: string
  role: string
  credit: number
}

/**
 * API response types matching backend endpoints
 */
interface SetupStatusResponse {
  ok: boolean
  needs_setup: boolean
  setup_key_required: boolean
}

interface AuthMeResponse {
  ok: boolean
  authenticated?: boolean
  username?: string
  role?: string
  credit?: number
}

interface LoginResponse {
  ok: boolean
  username?: string
  role?: string
  credit?: number
}

interface SetupResponse {
  ok: boolean
  username?: string
  role?: string
}

interface LogoutResponse {
  ok: boolean
}

/**
 * Admin authentication store (Pinia Composition API style)
 *
 * Manages authentication state including setup detection, login/logout,
 * and session validation. Uses useApi composable for all API interactions.
 *
 * Requirements: 2.1, 2.2, 2.3, 3.1, 3.3, 3.4
 */
export const useAuthStore = defineStore('auth', () => {
  // ─── State ────────────────────────────────────────────────────────────────
  const user = ref<AuthUser | null>(null)
  const isAuthenticated = ref(false)
  const setupRequired = ref(false)
  const setupKeyRequired = ref(false)
  const initialized = ref(false)

  // ─── API composable ───────────────────────────────────────────────────────
  const { get, post, loading, error } = useApi({
    onUnauthorized: () => {
      // On 401, clear auth state and redirect to login
      user.value = null
      isAuthenticated.value = false
      router.push({ name: 'login' })
    },
  })

  // ─── Computed ─────────────────────────────────────────────────────────────
  const username = computed(() => user.value?.username ?? '')
  const role = computed(() => user.value?.role ?? '')

  // ─── Actions ──────────────────────────────────────────────────────────────

  /**
   * Check current authentication status.
   * Called by the navigation guard on first route access.
   * - GET /api/setup/status → determines if setup is needed
   * - GET /api/auth/me → determines if user is authenticated
   *
   * On error, preserves existing state and surfaces error via toast.
   */
  async function checkAuth(): Promise<void> {
    try {
      // Check if setup is required
      const setupStatus = await get<SetupStatusResponse>('/api/setup/status')
      if (setupStatus.needs_setup) {
        setupRequired.value = true
        setupKeyRequired.value = setupStatus.setup_key_required
        initialized.value = true
        return
      }

      setupRequired.value = false

      // Check if user is already authenticated
      const me = await get<AuthMeResponse>('/api/auth/me')
      if (me.authenticated) {
        user.value = {
          username: me.username || '',
          role: me.role || 'admin',
          credit: me.credit || 0,
        }
        isAuthenticated.value = true
      } else {
        user.value = null
        isAuthenticated.value = false
      }
    } catch {
      // On error, preserve existing state (Requirement 3.4)
      // The useApi composable already sets error.value with a message
    } finally {
      initialized.value = true
    }
  }

  /**
   * Login with username and password.
   * POST /api/auth/admin with { username, password }
   *
   * On success, sets user and isAuthenticated.
   * On error, preserves existing state — error is surfaced via useApi's error ref.
   */
  async function login(username: string, password: string): Promise<boolean> {
    try {
      // Preflight: ensure CSRF token is available on fresh sessions
      await get<any>('/api/health').catch(() => null)

      const res = await post<LoginResponse>('/api/auth/admin', {
        username,
        password,
      })

      user.value = {
        username: res.username || username,
        role: res.role || 'admin',
        credit: res.credit || 0,
      }
      isAuthenticated.value = true
      setupRequired.value = false
      return true
    } catch {
      // Preserve existing state on error (Requirement 3.4)
      return false
    }
  }

  /**
   * Logout the current user.
   * POST /api/auth/logout
   *
   * Always clears local auth state regardless of API response.
   */
  async function logout(): Promise<void> {
    try {
      await post<LogoutResponse>('/api/auth/logout')
    } catch {
      // Ignore logout errors — always clear local state
    } finally {
      user.value = null
      isAuthenticated.value = false
    }
  }

  /**
   * Complete initial setup (create owner account).
   * POST /api/setup/owner with { username, password, setup_key? }
   *
   * On success, sets user as authenticated owner.
   * On error, preserves existing state.
   */
  async function setup(params: {
    username: string
    password: string
    setup_key?: string
  }): Promise<boolean> {
    try {
      // Preflight: ensure CSRF token is available on fresh sessions
      await get<any>('/api/health').catch(() => null)

      const res = await post<SetupResponse>('/api/setup/owner', params)

      user.value = {
        username: res.username || params.username,
        role: res.role || 'owner',
        credit: 0,
      }
      isAuthenticated.value = true
      setupRequired.value = false
      setupKeyRequired.value = false
      return true
    } catch {
      // Preserve existing state on error (Requirement 3.4)
      return false
    }
  }

  // ─── Expose ───────────────────────────────────────────────────────────────
  return {
    // State
    user,
    isAuthenticated,
    setupRequired,
    setupKeyRequired,
    initialized,

    // Computed
    username,
    role,

    // API state (from useApi)
    loading,
    error,

    // Actions
    checkAuth,
    login,
    logout,
    setup,
  }
})
