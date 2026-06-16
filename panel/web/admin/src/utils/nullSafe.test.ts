import { describe, it, expect } from 'vitest'
import { displayValue, formatDate } from './nullSafe'

describe('displayValue', () => {
  it('returns fallback for null', () => {
    expect(displayValue(null)).toBe('\u2014')
  })

  it('returns fallback for undefined', () => {
    expect(displayValue(undefined)).toBe('\u2014')
  })

  it('returns fallback for empty string', () => {
    expect(displayValue('')).toBe('\u2014')
  })

  it('returns string representation of a string value', () => {
    expect(displayValue('hello')).toBe('hello')
  })

  it('returns string representation of a number', () => {
    expect(displayValue(42)).toBe('42')
  })

  it('returns string representation of zero', () => {
    expect(displayValue(0)).toBe('0')
  })

  it('returns string representation of false', () => {
    expect(displayValue(false)).toBe('false')
  })

  it('uses custom fallback when provided', () => {
    expect(displayValue(null, 'N/A')).toBe('N/A')
  })
})

describe('formatDate', () => {
  it('returns fallback for null', () => {
    expect(formatDate(null)).toBe('\u2014')
  })

  it('returns fallback for undefined', () => {
    expect(formatDate(undefined)).toBe('\u2014')
  })

  it('returns fallback for empty string', () => {
    expect(formatDate('')).toBe('\u2014')
  })

  it('returns fallback for invalid date string', () => {
    expect(formatDate('not-a-date')).toBe('\u2014')
  })

  it('formats a valid ISO date string', () => {
    const result = formatDate('2024-03-15T10:30:00Z')
    // The exact format depends on locale, but it should contain the year and not be the fallback
    expect(result).not.toBe('\u2014')
    expect(result).toContain('2024')
  })

  it('formats a date-only ISO string', () => {
    const result = formatDate('2024-01-01')
    expect(result).not.toBe('\u2014')
    expect(result).toContain('2024')
  })

  it('uses custom fallback when provided', () => {
    expect(formatDate(null, 'N/A')).toBe('N/A')
  })
})
