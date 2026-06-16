import { ref, computed } from 'vue'
import { defineStore } from 'pinia'
import { useApi } from '@koris/composables/useApi'
import { useAuthStore } from '@/stores/auth'
import router from '@/router'
import type { Ticket } from '@koris/types'

/**
 * A single message within a ticket conversation thread
 */
export interface TicketMessage {
  id: number
  sender_type: 'admin' | 'customer'
  sender_name: string
  message: string
  created_at: string
}

/**
 * Extended ticket with its message thread
 */
export interface TicketDetail extends Ticket {
  messages: TicketMessage[]
}

/**
 * API response types matching backend endpoints
 */
interface TicketsListResponse {
  ok: boolean
  tickets: Ticket[]
}

interface TicketDetailResponse {
  ok: boolean
  ticket: TicketDetail
}

interface TicketReplyResponse {
  ok: boolean
}

interface TicketStatusResponse {
  ok: boolean
}

interface TicketCreateResponse {
  ok: boolean
  id: number
}

/**
 * Admin tickets store (Pinia Composition API style)
 *
 * Manages support ticket state including listing, detail view,
 * replies, and status changes. Uses useApi composable for all API interactions.
 *
 * Requirements: 3.1, 3.3, 22.6
 */
export const useTicketsStore = defineStore('tickets', () => {
  // ─── State ────────────────────────────────────────────────────────────────
  const list = ref<Ticket[]>([])
  const detail = ref<TicketDetail | null>(null)
  const loading = ref(false)

  // ─── API composable ───────────────────────────────────────────────────────
  const { get, post, error } = useApi({
    onUnauthorized: () => {
      // On 401, clear auth state and redirect to login
      const auth = useAuthStore()
      auth.user = null
      auth.isAuthenticated = false
      router.push({ name: 'login' })
    },
  })

  // ─── Computed ─────────────────────────────────────────────────────────────

  /** All tickets with status 'open' or 'pending' */
  const openTickets = computed(() =>
    list.value.filter((t) => t.status === 'open' || t.status === 'pending')
  )

  /** All tickets with status 'closed' */
  const closedTickets = computed(() =>
    list.value.filter((t) => t.status === 'closed')
  )

  // ─── Actions ──────────────────────────────────────────────────────────────

  /**
   * Load all tickets from the backend.
   * GET /api/tickets → { ok, tickets: Ticket[] }
   *
   * Sets loading = true before request, false after (Requirement 3.3).
   * On error, preserves existing data (Requirement 3.4).
   */
  async function loadTickets(): Promise<void> {
    loading.value = true
    try {
      const res = await get<TicketsListResponse>('/api/tickets')
      list.value = res.tickets || []
    } catch {
      // Preserve existing data on error (Requirement 3.4)
    } finally {
      loading.value = false
    }
  }

  /**
   * Load a single ticket's detail including its messages.
   * GET /api/tickets/:id → { ok, ticket: TicketDetail }
   *
   * On error, preserves existing detail state.
   */
  async function loadTicketDetail(id: number): Promise<void> {
    loading.value = true
    try {
      const res = await get<TicketDetailResponse>(`/api/tickets/${id}`)
      detail.value = res.ticket
    } catch {
      // Preserve existing detail on error (Requirement 3.4)
    } finally {
      loading.value = false
    }
  }

  /**
   * Reply to a ticket.
   * POST /api/tickets/:id/reply → { ok }
   *
   * After successful reply, reloads the ticket detail to include the new message.
   * On error, preserves existing data.
   */
  async function replyToTicket(id: number, message: string): Promise<boolean> {
    loading.value = true
    try {
      await post<TicketReplyResponse>(`/api/tickets/${id}/reply`, { message })
      // Reload detail to include the new message
      await loadTicketDetail(id)
      return true
    } catch {
      // Preserve existing data on error (Requirement 3.4)
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * Close a ticket.
   * POST /api/tickets/:id/close → { ok }
   *
   * After successful close, reloads the ticket detail and ticket list.
   * On error, preserves existing data.
   */
  async function closeTicket(id: number): Promise<boolean> {
    loading.value = true
    try {
      await post<TicketStatusResponse>(`/api/tickets/${id}/close`)
      // Reload detail and list to reflect updated status
      await loadTicketDetail(id).catch(() => null)
      await loadTickets().catch(() => null)
      return true
    } catch {
      // Preserve existing data on error (Requirement 3.4)
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * Reopen a closed ticket.
   * POST /api/tickets/:id/open → { ok }
   *
   * After successful reopen, reloads the ticket detail and ticket list.
   * On error, preserves existing data.
   */
  async function openTicket(id: number): Promise<boolean> {
    loading.value = true
    try {
      await post<TicketStatusResponse>(`/api/tickets/${id}/open`)
      // Reload detail and list to reflect updated status
      await loadTicketDetail(id).catch(() => null)
      await loadTickets().catch(() => null)
      return true
    } catch {
      // Preserve existing data on error (Requirement 3.4)
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * Create an admin-initiated ticket.
   * POST /api/tickets → { ok, id }
   *
   * Returns the new ticket ID on success, or null on failure.
   * Reloads the ticket list after creation.
   */
  async function createTicket(params: {
    username: string
    subject: string
    priority: string
    message: string
  }): Promise<number | null> {
    loading.value = true
    try {
      const res = await post<TicketCreateResponse>('/api/tickets', params)
      // Reload list to include the new ticket
      await loadTickets().catch(() => null)
      return res.id
    } catch {
      // Preserve existing data on error (Requirement 3.4)
      return null
    } finally {
      loading.value = false
    }
  }

  // ─── Expose ───────────────────────────────────────────────────────────────
  return {
    // State
    list,
    detail,
    loading,

    // API state (from useApi)
    error,

    // Computed
    openTickets,
    closedTickets,

    // Actions
    loadTickets,
    loadTicketDetail,
    replyToTicket,
    closeTicket,
    openTicket,
    createTicket,
  }
})
