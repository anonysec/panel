/**
 * Null-safety rendering utilities for the Admin Dashboard.
 * These helpers ensure nullable API fields render graceful fallback values
 * instead of "null", "undefined", or blank content.
 */

/**
 * Returns a string representation of the value, or a fallback if the value
 * is null, undefined, or an empty string.
 */
export function displayValue(value: unknown, fallback = '\u2014'): string {
  if (value === null || value === undefined || value === '') {
    return fallback
  }
  return String(value)
}

/**
 * Formats an ISO 8601 date string for display, or returns a fallback
 * if the value is null, undefined, or an empty string.
 * Uses the browser's locale-aware date formatting.
 */
export function formatDate(value: string | null | undefined, fallback = '\u2014'): string {
  if (value === null || value === undefined || value === '') {
    return fallback
  }

  const date = new Date(value)

  if (isNaN(date.getTime())) {
    return fallback
  }

  return date.toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}
