<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from '@koris/composables/useI18n'
import { useApi } from '@koris/composables/useApi'
import { useToast } from '@koris/composables/useToast'
import { formatDate } from '@koris/composables/useFormatDate'

const { t } = useI18n()
const api = useApi()
const toast = useToast()
const router = useRouter()

interface TicketItem {
  id: number
  subject: string
  status: string
  created_at: string
  updated_at: string
}

const tickets = ref<TicketItem[]>([])
const loading = ref(true)
const showNew = ref(false)
const newSubject = ref('')
const newMessage = ref('')
const creating = ref(false)

async function loadTickets() {
  loading.value = true
  try {
    const data = await api.get<{ ok: boolean; tickets: TicketItem[] }>('/api/reseller/tickets')
    if (data?.ok) {
      tickets.value = data.tickets
    }
  } finally {
    loading.value = false
  }
}

async function createTicket() {
  if (!newSubject.value.trim() || !newMessage.value.trim()) return
  creating.value = true
  try {
    const data = await api.post<{ ok: boolean; id: number }>('/api/reseller/tickets', {
      subject: newSubject.value.trim(),
      message: newMessage.value.trim(),
    })
    if (data?.ok) {
      toast.success(t('reseller_tickets.created'))
      showNew.value = false
      newSubject.value = ''
      newMessage.value = ''
      await loadTickets()
    }
  } finally {
    creating.value = false
  }
}

function openTicket(id: number) {
  router.push({ name: 'reseller-ticket-detail', params: { id } })
}

onMounted(loadTickets)
</script>

<template>
  <div class="reseller-tickets">
    <div class="page-header">
      <h1 class="page-title">{{ t('reseller_tickets.title') }}</h1>
      <button class="new-btn" @click="showNew = true">{{ t('reseller_tickets.new') }}</button>
    </div>

    <!-- New Ticket Form -->
    <div v-if="showNew" class="new-ticket-form">
      <div class="form-field">
        <label>{{ t('reseller_tickets.subject') }}</label>
        <input v-model="newSubject" type="text" class="form-input" :placeholder="t('reseller_tickets.subject')" />
      </div>
      <div class="form-field">
        <label>{{ t('reseller_tickets.message') }}</label>
        <textarea v-model="newMessage" class="form-textarea" rows="4" :placeholder="t('reseller_tickets.message')" />
      </div>
      <div class="form-actions">
        <button class="cancel-btn" @click="showNew = false">{{ t('btn.cancel') }}</button>
        <button class="submit-btn" :disabled="creating" @click="createTicket">
          {{ creating ? '...' : t('btn.create') }}
        </button>
      </div>
    </div>

    <!-- Tickets List -->
    <div v-if="loading" class="loading-state">
      <div v-for="i in 3" :key="i" class="skeleton-row" />
    </div>

    <div v-else-if="tickets.length === 0 && !showNew" class="empty-state">
      <span class="empty-icon">🎫</span>
      <p>{{ t('reseller_tickets.empty') }}</p>
    </div>

    <div v-else class="tickets-list">
      <div
        v-for="ticket in tickets"
        :key="ticket.id"
        class="ticket-row"
        @click="openTicket(ticket.id)"
      >
        <div class="ticket-info">
          <span class="ticket-subject">{{ ticket.subject }}</span>
          <span class="ticket-date">{{ formatDate(ticket.created_at) }}</span>
        </div>
        <span :class="['ticket-status', ticket.status]">{{ ticket.status }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.reseller-tickets {
  padding: var(--space-6, 24px);
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--space-5, 20px);
}

.page-title {
  font-size: var(--text-2xl, 22px);
  font-weight: var(--font-bold, 700);
  margin: 0;
}

.new-btn {
  padding: 8px 16px;
  border-radius: var(--radius-md, 8px);
  background: var(--color-primary, #2563eb);
  color: #fff;
  border: none;
  font-size: 13px;
  font-weight: var(--font-semibold, 600);
  cursor: pointer;
}

.new-btn:hover {
  opacity: 0.85;
}

.new-ticket-form {
  background: var(--color-surface-2, #1e2630);
  border: 1px solid var(--color-border, #28333f);
  border-radius: var(--radius-lg, 12px);
  padding: var(--space-5, 20px);
  margin-bottom: var(--space-5, 20px);
}

.form-field {
  margin-bottom: var(--space-4, 16px);
}

.form-field label {
  display: block;
  font-size: 12px;
  font-weight: var(--font-semibold, 600);
  color: var(--color-muted, #8b98a5);
  margin-bottom: 6px;
}

.form-input, .form-textarea {
  width: 100%;
  padding: 10px 12px;
  border: 1px solid var(--color-border, #28333f);
  border-radius: var(--radius-sm, 6px);
  background: var(--color-surface, #0b1120);
  color: var(--color-text, #e6edf3);
  font-size: 13px;
  font-family: inherit;
}

.form-input:focus, .form-textarea:focus {
  outline: none;
  border-color: var(--color-primary, #2563eb);
}

.form-textarea {
  resize: vertical;
  min-height: 80px;
}

.form-actions {
  display: flex;
  gap: 10px;
  justify-content: flex-end;
}

.cancel-btn {
  padding: 8px 16px;
  border-radius: var(--radius-sm, 6px);
  background: none;
  border: 1px solid var(--color-border, #28333f);
  color: var(--color-text, #e6edf3);
  font-size: 13px;
  cursor: pointer;
}

.submit-btn {
  padding: 8px 16px;
  border-radius: var(--radius-sm, 6px);
  background: var(--color-primary, #2563eb);
  color: #fff;
  border: none;
  font-size: 13px;
  font-weight: var(--font-semibold, 600);
  cursor: pointer;
}

.submit-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.loading-state {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.skeleton-row {
  height: 56px;
  background: var(--color-surface-2, #1e2630);
  border-radius: var(--radius-md, 8px);
  animation: pulse 1.5s infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

.empty-state {
  text-align: center;
  padding: 60px 20px;
  color: var(--color-muted, #8b98a5);
}

.empty-icon {
  font-size: 40px;
  display: block;
  margin-bottom: 12px;
}

.tickets-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.ticket-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
  background: var(--color-surface-2, #1e2630);
  border: 1px solid var(--color-border, #28333f);
  border-radius: var(--radius-md, 8px);
  cursor: pointer;
  transition: border-color 0.15s;
}

.ticket-row:hover {
  border-color: var(--color-primary, #2563eb);
}

.ticket-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.ticket-subject {
  font-weight: var(--font-semibold, 600);
  font-size: 14px;
  color: var(--color-text, #e6edf3);
}

.ticket-date {
  font-size: 11px;
  color: var(--color-muted, #8b98a5);
}

.ticket-status {
  font-size: 11px;
  padding: 3px 8px;
  border-radius: 10px;
  font-weight: var(--font-semibold, 600);
  text-transform: capitalize;
}

.ticket-status.open {
  background: rgba(34, 197, 94, 0.15);
  color: #22c55e;
}

.ticket-status.closed {
  background: rgba(107, 114, 128, 0.15);
  color: #9ca3af;
}
</style>
