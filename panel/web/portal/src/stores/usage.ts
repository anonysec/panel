import { ref, computed } from 'vue'
import { defineStore } from 'pinia'
import { useApi } from '@koris/composables/useApi'

/**
 * A single usage session
 */
export interface UsageSession {
  id: number
  start_time: string
  stop_time: string
  session_seconds: number
  input_bytes: number
  output_bytes: number
  total_bytes: number
  framed_ip: string
  online: boolean
}

/**
 * Bandwidth usage summary from the backend
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
 * API response types
 */
interface UsageResponse {
  ok: boolean
  usage: UsageSummary
}

/**
 * Portal usage store (Pinia Composition API style)
 *
 * Manages bandwidth usage data and session history.
 * Uses useApi composable for all API interactions.
 *
 * Requirements: 3.2, 3.3, 3.4, 23.4
 */
export const useUsageStore = defineStore('portal-usage', () => {
  // ─── State ────────────────────────────────────────────────────────────────
  const usage = ref<UsageSummary | null>(null)
  const loading = ref(false)

  // ─── API composable ───────────────────────────────────────────────────────
  const { get, error } = useApi({
    onUnauthorized: () => {
      // Auth store handles redirect
    },
  })

  // ─── Computed ─────────────────────────────────────────────────────────────
  const isOnline = computed(() => usage.value?.online ?? false)

  const activeSessions = computed(() => usage.value?.active_sessions ?? 0)

  const totalUsageBytes = computed(() => usage.value?.total_usage_bytes ?? 0)

  const maxDataBytes = computed(() => usage.value?.max_data_bytes ?? 0)

  const usagePercent = computed(() => {
    if (!usage.value?.max_data_bytes) return 0
    return Math.min(100, Math.round((usage.value.total_usage_bytes / usage.value.max_data_bytes) * 100))
  })

  const remainingBytes = computed(() => {
    if (!usage.value?.max_data_bytes) return Infinity
    return Math.max(0, usage.value.max_data_bytes - usage.value.total_usage_bytes)
  })

  const sessions = computed(() => usage.value?.sessions ?? [])

  /**
   * Chart data points derived from sessions for bandwidth visualization
   */
  const bandwidthChartData = computed(() => {
    if (!usage.value?.sessions?.length) return []
    return usage.value.sessions
      .slice()
      .reverse()
      .map((s) => ({
        label: new Date(s.start_time).toLocaleDateString('en', { month: 'short', day: 'numeric' }),
        value: Math.round(s.total_bytes / (1024 * 1024)), // MB
      }))
  })

  // ─── Actions ──────────────────────────────────────────────────────────────

  /**
   * Load usage data from the backend.
   * GET /api/portal/usage → { ok, usage }
   */
  async function loadUsage(): Promise<void> {
    loading.value = true
    try {
      const res = await get<UsageResponse>('/api/portal/usage')
      usage.value = res.usage
    } catch {
      // Preserve existing data on error (Requirement 3.4)
    } finally {
      loading.value = false
    }
  }

  // ─── Expose ───────────────────────────────────────────────────────────────
  return {
    // State
    usage,
    loading,

    // API state
    error,

    // Computed
    isOnline,
    activeSessions,
    totalUsageBytes,
    maxDataBytes,
    usagePercent,
    remainingBytes,
    sessions,
    bandwidthChartData,

    // Actions
    loadUsage,
  }
})
