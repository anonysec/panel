import { ref, onMounted, onActivated } from 'vue'

const STALE_THRESHOLD_MS = 30_000 // 30 seconds

export function useFreshData(fetcher: () => Promise<void>) {
  const lastFetchedAt = ref<number>(0)

  async function ensureFresh(): Promise<void> {
    const now = Date.now()
    if (now - lastFetchedAt.value > STALE_THRESHOLD_MS) {
      await fetcher()
      lastFetchedAt.value = Date.now()
    }
  }

  onMounted(ensureFresh)
  onActivated(ensureFresh)

  return { ensureFresh, lastFetchedAt }
}
