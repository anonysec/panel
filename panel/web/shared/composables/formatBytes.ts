/**
 * Converts a byte value to a human-readable string.
 * Uses base-1024 with exactly 1 decimal place.
 * Returns "0.0 B" for zero or negative inputs.
 *
 * @param bytes - Number of bytes to format
 * @returns Formatted string (e.g., "2.4 GB", "512.0 MB", "0.0 B")
 */
export function formatBytes(bytes: number): string {
  if (bytes <= 0) return '0.0 B'

  const units = ['B', 'KB', 'MB', 'GB', 'TB'] as const
  const base = 1024

  let unitIndex = 0
  let value = bytes

  while (value >= base && unitIndex < units.length - 1) {
    value /= base
    unitIndex++
  }

  return `${value.toFixed(1)} ${units[unitIndex]}`
}
