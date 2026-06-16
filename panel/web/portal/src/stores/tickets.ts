import { ref, computed } from 'vue'
import { defineStore } from 'pinia'
import { useApi } from '@koris/composables/useApi'
import router from '@/router'
import type { Ticket } from '@koris/types/entities'

/**
 * Ticket message within a conversation thread
 */
export interface TicketMessage {
  id: number
  ticket_id: number
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
 * API response types
 */
interface TicketsListResponse {
  ok: boolean
  tickets: Ticket[]
}

interface TicketDetailResponse {
  ok: boolean
  ticket: TicketDetail
}

interface TicketCreateResponse {
  ok: boolean
  id: number
}

interface TicketReplyResponse {
  ok: boolean
}

/**
 * Portal tickets store (Pinia Composition API style)
 *
 * Manages support ticket list, detail view, creation, and replies.
 * Uses useApi composable for all API interactions.
 *
 * Requirements: 3.2, 3.3, 3.4, 23.5
 */
export const usePortalTicketsStore = defineStore('portal-tickets', () => {
  // ─── State ────────────────────────────────────────────────────────────────
  const list = ref<Ticket[]>([])
  const detail = ref<TicketDetail | null>(null)
  const loading = ref(false)

  // ─── API composable ───────────────────────────────────────────────────────
  const { get, post, error } = useApi({
    onUnauthorized: () => {
      // Clear tickets state and redirect to portal login
      list.value = []
      detail.value = null
      router.push({ name: 'portal-login' })
    },
  })

  // ─── Computed ─────────────────────────────────────────────────────────────
  const openTickets = computed(() =>
    list.value.filter((t) => t.status === 'open' || t.status === 'pending')
  )

  const closedTickets = computed(() =>
    list.value.filter((t) => t.status === 'closed')
  )

  // ─── Actions ──────────────────────────────────────────────────────────────

  /**
   * Load all customer tickets.
   * GET /api/portal/tickets → { ok, tickets }
   */
  async function loadTickets(): Promise<void> {
    loading.value = true
    try {
      const res = await get<TicketsListResponse>('/api/portal/tickets')
      list.value = res.tickets || []
    } catch {
      // Preserve existing data on error (Requirement 3.4)
    } finally {
      loading.value = false
    }
  }

  /**
   * Load a single ticket's detail with messages.
   * GET /api/portal/tickets/:id → { ok, ticket }
   */
  async function loadTicketDetail(id: number): Promise<void> {
    loading.value = true
    try {
      const res = await get<TicketDetailResponse>(`/api/portal/tickets/${id}`)
      detail.value = res.ticket
    } catch {
      // Preserve existing detail on error (Requirement 3.4)
    } finally {
      loading.value = false
    }
  }

  /**
   * Create a new support ticket.
   * POST /api/portal/tickets → { ok, id }
   */
  async function createTicket(params: { subject: string; priority: string; message: string }): Promise<number | null> {
    loading.value = true
    try {
      const res = await post<TicketCreateResponse>('/api/portal/tickets', params)
      await loadTickets()
      return res.id
    } catch {
      return null
    } finally {
      loading.value = false
    }
  }

  /**
   * Reply to an existing ticket.
   * POST /api/portal/tickets/:id/reply → { ok }
   */
  async function replyToTicket(id: number, message: string): Promise<boolean> {
    loading.value = true
    try {
      await post<TicketReplyResponse>(`/api/portal/tickets/${id}/reply`, { message })
      await loadTicketDetail(id)
      return true
    } catch {
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * Close a ticket.
   * POST /api/portal/tickets/:id/close → { ok }
   */
  async function closeTicket(id: number): Promise<boolean> {
    loading.value = true
    try {
      await post<TicketReplyResponse>(`/api/portal/tickets/${id}/close`)
      await loadTicketDetail(id)
      await loadTickets()
      return true
    } catch {
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * Clear the currently selected ticket detail.
   */
  function clearDetail(): void {
    detail.value = null
  }

  // ─── Expose ───────────────────────────────────────────────────────────────
  return {
    // State
    list,
    detail,
    loading,

    // API state
    error,

    // Computed
    openTickets,
    closedTickets,

    // Actions
    loadTickets,
    loadTicketDetail,
    createTicket,
    replyToTicket,
    closeTicket,
    clearDetail,
  }
})
