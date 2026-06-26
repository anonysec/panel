import { ref, type Ref } from 'vue'
import { useApi } from '@koris/composables/useApi'
import { useToast } from '@koris/composables/useToast'

/**
 * Options for the useEntityForm composable.
 *
 * @template T - Shape of the form data object
 */
export interface EntityFormOptions<T extends Record<string, any>> {
  /** API endpoint to POST form data to (e.g. '/api/admin/customers') */
  apiEndpoint: string
  /** Initial/default form values — used for reset on success */
  initialValues: T
  /**
   * Validate form before submission.
   * Return null if valid, or an error message string if invalid.
   */
  validate: (form: T) => string | null
  /** Called after a successful submission (e.g. close panel, reload list) */
  onSuccess: () => void
}

/**
 * Return type of useEntityForm composable
 */
export interface EntityFormReturn<T extends Record<string, any>> {
  /** Reactive form data */
  form: Ref<T>
  /** Whether the form is currently submitting */
  submitting: Ref<boolean>
  /** Current validation error message (empty string if none) */
  validationError: Ref<string>
  /** Submit the form: validates, posts to API, handles success/error */
  submit: () => Promise<void>
  /** Reset form to initial values and clear validation error */
  reset: () => void
}

/**
 * Shared composable for entity creation forms.
 *
 * Provides a consistent pattern across all "Add Entity" panels:
 * 1. Client-side validation before submit (blocks API request on invalid)
 * 2. POST to apiEndpoint with form data
 * 3. On success: shows success toast, resets form, calls onSuccess callback
 * 4. On error: shows error toast, preserves form data (panel stays open)
 *
 * @example
 * ```ts
 * const { form, submitting, validationError, submit, reset } = useEntityForm({
 *   apiEndpoint: '/api/admin/customers',
 *   initialValues: { username: '', password: '' },
 *   validate: (f) => !f.username ? 'Username is required' : null,
 *   onSuccess: () => { showPanel.value = false; loadCustomers() }
 * })
 * ```
 */
export function useEntityForm<T extends Record<string, any>>(
  options: EntityFormOptions<T>
): EntityFormReturn<T> {
  const { apiEndpoint, initialValues, validate, onSuccess } = options

  const api = useApi({ baseUrl: '', showErrorToast: false })
  const toast = useToast()

  const form = ref<T>({ ...initialValues }) as Ref<T>
  const submitting = ref(false)
  const validationError = ref('')

  function reset(): void {
    form.value = { ...initialValues } as any
    validationError.value = ''
  }

  async function submit(): Promise<void> {
    // Client-side validation — blocks API request if invalid
    const error = validate(form.value)
    if (error) {
      validationError.value = error
      toast.warning(error)
      return
    }

    validationError.value = ''
    submitting.value = true

    try {
      await api.post(apiEndpoint, form.value)
      toast.success('Created successfully')
      reset()
      onSuccess()
    } catch (err: any) {
      // Preserve form data on error — panel stays open
      const message = err?.message || 'An error occurred'
      toast.error(message)
    } finally {
      submitting.value = false
    }
  }

  return {
    form,
    submitting,
    validationError,
    submit,
    reset,
  }
}
