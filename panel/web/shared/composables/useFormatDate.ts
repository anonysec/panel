/**
 * Shared date formatting utilities.
 *
 * formatDate  - date only (year, short month, 2-digit day)
 * formatDateTime - date + time (short month, 2-digit day, hour, minute)
 */

export function formatDate(value: string | null | undefined, fallback = '--'): string {
  if (!value) return fallback
  return new Intl.DateTimeFormat('en', {
    year: 'numeric',
    month: 'short',
    day: '2-digit',
  }).format(new Date(value))
}

export function formatDateTime(value: string | null | undefined, fallback = '--'): string {
  if (!value) return fallback
  return new Intl.DateTimeFormat('en', {
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value))
}
