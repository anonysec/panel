import { computed, type Ref, toValue, type MaybeRef } from 'vue'
import { formatBytes } from '@koris/composables/formatBytes'

// Re-export formatBytes so existing consumers keep the same import path
export { formatBytes }

/**
 * Composable for calculating and displaying data usage metrics.
 *
 * @param usedBytes - Bytes consumed (reactive ref or plain number)
 * @param capBytes - Total data cap in bytes (reactive ref or plain number)
 * @param expiresAt - ISO date string for subscription expiry (reactive ref or plain string)
 *
 * Validates: Requirements 6.1, 6.2, 6.3, 6.4, 6.5
 */
export function useUsageDisplay(
  usedBytes: MaybeRef<number>,
  capBytes: MaybeRef<number>,
  expiresAt: MaybeRef<string>
) {
  const usedPercent = computed(() => {
    const cap = toValue(capBytes)
    const used = toValue(usedBytes)
    return cap > 0 ? (used / cap) * 100 : 0
  })

  const remainingBytes = computed(() => {
    return Math.max(0, toValue(capBytes) - toValue(usedBytes))
  })

  const progressColor = computed<'green' | 'amber' | 'red'>(() => {
    const remainingPercent = 100 - usedPercent.value
    if (remainingPercent <= 5) return 'red'
    if (remainingPercent <= 20) return 'amber'
    return 'green'
  })

  const daysRemaining = computed(() => {
    const exp = toValue(expiresAt)
    if (!exp) return Infinity
    const now = new Date()
    const expiry = new Date(exp)
    if (isNaN(expiry.getTime())) return Infinity
    const diff = expiry.getTime() - now.getTime()
    return Math.max(0, Math.ceil(diff / (1000 * 60 * 60 * 24)))
  })

  return {
    usedPercent,
    remainingBytes,
    progressColor,
    daysRemaining
  }
}
