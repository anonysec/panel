/**
 * Normalizes a value to a finite number.
 * Returns 0 for null, undefined, NaN, Infinity, -Infinity, or non-numeric values.
 */
export function sanitizeNumber(value: unknown): number {
  const n = Number(value)
  return Number.isFinite(n) ? n : 0
}
