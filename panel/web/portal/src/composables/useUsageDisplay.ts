import { computed, type Ref, toValue, type MaybeRef } from 'vue'

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
    const now = new Date()
    const expiry = new Date(toValue(expiresAt))
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

/**
 * Formats a byte count into a human-readable string with appropriate units.
 *
 * @param bytes - Number of bytes to format
 * @returns Formatted string (e.g., "2.4 GB", "512.0 MB", "0 B")
 */
export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const k = 1024
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${units[i]}`
}
