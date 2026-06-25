import { ref, computed } from 'vue'
import { defineStore } from 'pinia'
import { useApi } from '@koris/composables/useApi'

/**
 * API response from GET /api/info
 */
interface InfoResponse {
  ok: boolean
  edition: 'full' | 'lite'
  version?: string
}

/**
 * Edition store — detects whether the panel is running in "full" or "lite" mode.
 *
 * Used for conditional rendering of full-only features in the sidebar, views,
 * and components. Defaults to "lite" on failure or unrecognized values for safety.
 *
 * Requirements: 22.1, 22.4, 22.5
 */
export const useEditionStore = defineStore('edition', () => {
  // ─── State ────────────────────────────────────────────────────────────────
  const edition = ref<'full' | 'lite'>('lite')
  const loaded = ref(false)

  // ─── Computed ─────────────────────────────────────────────────────────────
  const isLite = computed(() => edition.value === 'lite')
  const isFull = computed(() => edition.value === 'full')

  // ─── Actions ──────────────────────────────────────────────────────────────

  /**
   * Fetch the panel edition from GET /api/info.
   * Only fetches once — subsequent calls are no-ops if already loaded.
   * Defaults to 'lite' on any error or unrecognized value.
   */
  async function fetchEdition(): Promise<void> {
    if (loaded.value) return

    const { get } = useApi({ showErrorToast: false })
    try {
      const res = await get<InfoResponse>('/api/info')
      if (res?.ok && (res.edition === 'full' || res.edition === 'lite')) {
        edition.value = res.edition
      }
    } catch {
      // Default to lite on failure (Requirement 22.5)
    }
    loaded.value = true
  }

  // ─── Expose ───────────────────────────────────────────────────────────────
  return {
    edition,
    loaded,
    isLite,
    isFull,
    fetchEdition,
  }
})
