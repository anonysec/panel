/**
 * Unit tests for useEntityForm composable
 *
 * Tests the shared entity form pattern:
 * - Validation blocks submission when fields are invalid
 * - Successful submission resets form and calls onSuccess
 * - Failed submission preserves form data and shows error
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useEntityForm } from './useEntityForm'

// Mock fetch globally for API calls
const mockFetch = vi.fn()
globalThis.fetch = mockFetch

beforeEach(() => {
  vi.clearAllMocks()
  mockFetch.mockReset()
})

interface TestForm {
  name: string
  email: string
}

const defaultOptions = () => ({
  apiEndpoint: '/api/admin/test',
  initialValues: { name: '', email: '' } as TestForm,
  validate: (form: TestForm) => {
    if (!form.name) return 'Name is required'
    if (!form.email) return 'Email is required'
    return null
  },
  onSuccess: vi.fn(),
})

describe('useEntityForm', () => {
  describe('initialization', () => {
    it('initializes form with provided initial values', () => {
      const opts = defaultOptions()
      opts.initialValues = { name: 'John', email: 'john@test.com' }
      const { form } = useEntityForm(opts)

      expect(form.value.name).toBe('John')
      expect(form.value.email).toBe('john@test.com')
    })

    it('starts with submitting=false and no validation error', () => {
      const { submitting, validationError } = useEntityForm(defaultOptions())

      expect(submitting.value).toBe(false)
      expect(validationError.value).toBe('')
    })
  })

  describe('validation blocks submission', () => {
    it('does not call API when validation fails', async () => {
      const opts = defaultOptions()
      const { form, submit } = useEntityForm(opts)

      // Leave required fields empty
      form.value.name = ''
      form.value.email = ''

      await submit()

      // fetch should never have been called
      expect(mockFetch).not.toHaveBeenCalled()
      expect(opts.onSuccess).not.toHaveBeenCalled()
    })

    it('sets validationError when validation fails', async () => {
      const opts = defaultOptions()
      const { form, validationError, submit } = useEntityForm(opts)

      form.value.name = ''
      form.value.email = 'test@test.com'

      await submit()

      expect(validationError.value).toBe('Name is required')
    })

    it('does not call API when partial fields are filled', async () => {
      const opts = defaultOptions()
      const { form, submit } = useEntityForm(opts)

      form.value.name = 'John'
      form.value.email = '' // email still empty

      await submit()

      expect(mockFetch).not.toHaveBeenCalled()
      expect(opts.onSuccess).not.toHaveBeenCalled()
    })
  })

  describe('successful submission', () => {
    it('calls API and onSuccess when form is valid', async () => {
      const opts = defaultOptions()
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers(),
        json: () => Promise.resolve({ ok: true }),
      })

      const { form, submit } = useEntityForm(opts)
      form.value.name = 'John'
      form.value.email = 'john@test.com'

      await submit()

      expect(mockFetch).toHaveBeenCalledTimes(1)
      expect(opts.onSuccess).toHaveBeenCalledTimes(1)
    })

    it('resets form to initial values on success', async () => {
      const opts = defaultOptions()
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers(),
        json: () => Promise.resolve({ ok: true }),
      })

      const { form, submit } = useEntityForm(opts)
      form.value.name = 'John'
      form.value.email = 'john@test.com'

      await submit()

      expect(form.value.name).toBe('')
      expect(form.value.email).toBe('')
    })

    it('clears validation error on success', async () => {
      const opts = defaultOptions()
      const { form, validationError, submit } = useEntityForm(opts)

      // First trigger a validation error
      form.value.name = ''
      form.value.email = ''
      await submit()
      expect(validationError.value).toBe('Name is required')

      // Now fix and submit successfully
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers(),
        json: () => Promise.resolve({ ok: true }),
      })
      form.value.name = 'John'
      form.value.email = 'john@test.com'
      await submit()

      expect(validationError.value).toBe('')
    })
  })

  describe('error preserves form state', () => {
    it('preserves form data when API returns error', async () => {
      const opts = defaultOptions()
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        headers: new Headers(),
        json: () => Promise.resolve({ error: 'Server error' }),
      })

      const { form, submit } = useEntityForm(opts)
      form.value.name = 'John'
      form.value.email = 'john@test.com'

      await submit()

      // Form data should be preserved
      expect(form.value.name).toBe('John')
      expect(form.value.email).toBe('john@test.com')
    })

    it('does not call onSuccess when API fails', async () => {
      const opts = defaultOptions()
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        headers: new Headers(),
        json: () => Promise.resolve({ error: 'Bad request' }),
      })

      const { form, submit } = useEntityForm(opts)
      form.value.name = 'John'
      form.value.email = 'john@test.com'

      await submit()

      expect(opts.onSuccess).not.toHaveBeenCalled()
    })

    it('sets submitting back to false after error', async () => {
      const opts = defaultOptions()
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        headers: new Headers(),
        json: () => Promise.resolve({ error: 'Server error' }),
      })

      const { form, submitting, submit } = useEntityForm(opts)
      form.value.name = 'John'
      form.value.email = 'john@test.com'

      await submit()

      expect(submitting.value).toBe(false)
    })
  })

  describe('reset', () => {
    it('resets form to initial values', () => {
      const opts = defaultOptions()
      const { form, reset } = useEntityForm(opts)

      form.value.name = 'Modified'
      form.value.email = 'modified@test.com'

      reset()

      expect(form.value.name).toBe('')
      expect(form.value.email).toBe('')
    })

    it('clears validation error on reset', async () => {
      const opts = defaultOptions()
      const { form, validationError, submit, reset } = useEntityForm(opts)

      form.value.name = ''
      await submit()
      expect(validationError.value).toBe('Name is required')

      reset()
      expect(validationError.value).toBe('')
    })
  })
})
