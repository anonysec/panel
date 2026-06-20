import { ref, type Ref } from 'vue'
import { useToast } from './useToast'

/**
 * Error object for API failures
 */
export interface ApiError {
  status: number
  message: string
  url: string
}

/**
 * Options for configuring the useApi composable
 */
export interface UseApiOptions {
  baseUrl?: string
  onUnauthorized?: () => void
  onError?: (error: ApiError) => void
  /** When true, automatically shows a toast notification on API errors. Defaults to true. */
  showErrorToast?: boolean
}

/**
 * Return type of the useApi composable
 */
export interface UseApiReturn {
  get<T>(url: string, options?: RequestInit): Promise<T>
  post<T>(url: string, body?: unknown, options?: RequestInit): Promise<T>
  put<T>(url: string, body?: unknown, options?: RequestInit): Promise<T>
  patch<T>(url: string, body?: unknown, options?: RequestInit): Promise<T>
  del<T>(url: string, options?: RequestInit): Promise<T>
  loading: Ref<boolean>
  error: Ref<string>
}

/** Module-level CSRF token storage (shared across all useApi instances) */
let csrfToken: string = ''

/** Get the current CSRF token value */
export function getCsrfToken(): string {
  return csrfToken
}

/**
 * Composable for making API requests with consistent loading state,
 * error handling, CSRF token management, and authentication failure detection.
 *
 * @param options - Configuration options for the API composable
 * @returns UseApiReturn with typed HTTP methods, loading, and error refs
 *
 * @example
 * ```ts
 * const { get, post, loading, error } = useApi({
 *   baseUrl: '/api',
 *   onUnauthorized: () => router.push('/login')
 * })
 *
 * const customers = await get<Customer[]>('/customers')
 * ```
 */
export function useApi(options: UseApiOptions = {}): UseApiReturn {
  const { baseUrl = '', onUnauthorized, onError, showErrorToast = true } = options
  const toast = useToast()

  const loading: Ref<boolean> = ref(false)
  const error: Ref<string> = ref('')

  /**
   * Core request method that all HTTP methods delegate to.
   * Handles loading state, error handling, content-type, CSRF tokens, and auth failures.
   */
  async function request<T>(
    url: string,
    method: string,
    body?: unknown,
    requestOptions?: RequestInit
  ): Promise<T> {
    loading.value = true
    error.value = ''

    try {
      const headers = new Headers(requestOptions?.headers as HeadersInit | undefined)

      // Automatically set Content-Type for methods that carry a body
      if ((method === 'POST' || method === 'PUT' || method === 'PATCH') && !headers.has('Content-Type')) {
        headers.set('Content-Type', 'application/json')
      }

      // Include CSRF token on state-changing requests
      if (csrfToken && (method === 'POST' || method === 'PUT' || method === 'PATCH' || method === 'DELETE')) {
        headers.set('X-CSRF-Token', csrfToken)
      }

      const fetchOptions: RequestInit = {
        ...requestOptions,
        method,
        headers,
        credentials: 'same-origin',
      }

      // Serialize body to JSON if present
      if (body !== undefined && body !== null) {
        fetchOptions.body = JSON.stringify(body)
      }

      const response = await fetch(`${baseUrl}${url}`, fetchOptions)

      // Capture CSRF token from response
      const newToken = response.headers.get('X-CSRF-Token')
      if (newToken) {
        csrfToken = newToken
      }

      // Handle 401 Unauthorized
      if (response.status === 401) {
        const apiError: ApiError = {
          status: 401,
          message: 'Unauthorized',
          url: `${baseUrl}${url}`,
        }
        error.value = 'Unauthorized'
        if (onUnauthorized) {
          onUnauthorized()
        }
        if (onError) {
          onError(apiError)
        }
        throw apiError
      }

      // Handle other non-2xx responses
      if (!response.ok) {
        let message = `Request failed with status ${response.status}`

        try {
          const errorBody = await response.json()
          if (errorBody.error) {
            message = errorBody.error
          } else if (errorBody.message) {
            message = errorBody.message
          }
        } catch {
          // Response body is not JSON, use default message
        }

        const apiError: ApiError = {
          status: response.status,
          message,
          url: `${baseUrl}${url}`,
        }
        error.value = message
        if (showErrorToast) {
          toast.error(message)
        }
        if (onError) {
          onError(apiError)
        }
        throw apiError
      }

      // Parse successful response
      const data = await response.json() as T
      return data
    } catch (err) {
      // If it's already an ApiError we threw, re-throw it
      if (err && typeof err === 'object' && 'status' in err && 'url' in err) {
        throw err
      }

      // Network errors or other unexpected failures
      const message = err instanceof Error ? err.message : 'An unexpected error occurred'
      error.value = message
      throw err
    } finally {
      loading.value = false
    }
  }

  /**
   * Perform a GET request
   */
  function get<T>(url: string, requestOptions?: RequestInit): Promise<T> {
    return request<T>(url, 'GET', undefined, requestOptions)
  }

  /**
   * Perform a POST request with automatic Content-Type: application/json
   */
  function post<T>(url: string, body?: unknown, requestOptions?: RequestInit): Promise<T> {
    return request<T>(url, 'POST', body, requestOptions)
  }

  /**
   * Perform a PUT request with automatic Content-Type: application/json
   */
  function put<T>(url: string, body?: unknown, requestOptions?: RequestInit): Promise<T> {
    return request<T>(url, 'PUT', body, requestOptions)
  }

  /**
   * Perform a PATCH request with automatic Content-Type: application/json
   */
  function patch<T>(url: string, body?: unknown, requestOptions?: RequestInit): Promise<T> {
    return request<T>(url, 'PATCH', body, requestOptions)
  }

  /**
   * Perform a DELETE request
   */
  function del<T>(url: string, requestOptions?: RequestInit): Promise<T> {
    return request<T>(url, 'DELETE', undefined, requestOptions)
  }

  return {
    get,
    post,
    put,
    patch,
    del,
    loading,
    error,
  }
}
