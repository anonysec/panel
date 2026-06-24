import { ref, computed } from 'vue'
import { useApi } from '@koris/composables/useApi'

const edition = ref<'full' | 'lite'>('full')
const loaded = ref(false)

export function useEdition() {
  const { get } = useApi()

  async function fetchEdition() {
    if (loaded.value) return
    try {
      const res = await get<{ ok: boolean; edition: 'full' | 'lite' }>('/api/info')
      if (res?.ok) {
        edition.value = res.edition
      }
    } catch {
      // Default to full if the endpoint is unavailable
    }
    loaded.value = true
  }

  const isLite = computed(() => edition.value === 'lite')
  const isFull = computed(() => edition.value === 'full')

  return { edition, isLite, isFull, loaded, fetchEdition }
}
