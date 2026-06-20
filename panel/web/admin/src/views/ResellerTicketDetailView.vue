<script setup lang="ts">
import { ref, onMounted, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from '@koris/composables/useI18n'
import { useApi } from '@koris/composables/useApi'
import { useToast } from '@koris/composables/useToast'
import { formatDate } from '@koris/composables/useFormatDate'
import { useAuthStore } from '@/stores/auth'

const props = defineProps<{ id: string | number }>()

const { t } = useI18n()
const api = useApi()
const toast = useToast()
const router = useRouter()
const auth = useAuthStore()

interface Message {
  id: number
  sender: string
  message: string
  created_at: string
}

interface TicketDetail {
  ok: boolean
  id: number
  subject: string
  status: string
  created_at: string
  updated_at: string
  messages: Message[]
}

const ticket = ref<TicketDetail | null>(null)
const loading = ref(true)
const replyText = ref('')
const replying = ref(false)
const messagesEl = ref<HTMLElement | null>(null)

async function loadTicket() {
  loading.value = true
  try {
    const data = await api.get<TicketDetail>(`/api/reseller/tickets/${props.id}`)
    if (data?.ok) {
      ticket.value = data
      await nextTick()
      scrollToBottom()
    }
  } finally {
    loading.value = false
  }
}

async function sendReply() {
  if (!replyText.value.trim()) return
  replying.value = true
  try {
    const data = await api.post<{ ok: boolean }>(`/api/reseller/tickets/${props.id}/reply`, {
      message: replyText.value.trim(),
    })
    if (data?.ok) {
      replyText.value = ''
      await loadTicket()
    }
  } finally {
    replying.value = false
  }
}

async function closeTicket() {
  const data = await api.post<{ ok: boolean }>(`/api/reseller/tickets/${props.id}/close`, {})
  if (data?.ok) {
    toast.success(t('reseller_tickets.closed'))
    if (ticket.value) ticket.value.status = 'closed'
  }
}

function scrollToBottom() {
  if (messagesEl.value) {
    messagesEl.value.scrollTop = messagesEl.value.scrollHeight
  }
}

function isOwnMessage(sender: string): boolean {
  return sender === auth.user?.username
}

onMounted(loadTicket)
</script>

<template>
  <div class="ticket-detail">
    <div class="ticket-header">
      <button class="back-btn" @click="router.push({ name: 'reseller-tickets' })">← {{ t('tickets.back') }}</button>
      <div v-if="ticket" class="header-right">
        <span :class="['status-badge', ticket.status]">{{ ticket.status }}</span>
        <button
          v-if="ticket.status === 'open'"
          class="close-btn"
          @click="closeTicket"
        >
          {{ t('reseller_tickets.close') }}
        </button>
      </div>
    </div>

    <div v-if="loading" class="loading-state">
      <div class="skeleton-row" style="height: 32px; width: 60%" />
      <div class="skeleton-row" style="height: 200px" />
    </div>

    <template v-else-if="ticket">
      <h2 class="ticket-subject">{{ ticket.subject }}</h2>
      <span class="ticket-date">{{ formatDate(ticket.created_at) }}</span>

      <div ref="messagesEl" class="messages-container">
        <div
          v-for="msg in ticket.messages"
          :key="msg.id"
          :class="['message-bubble', { own: isOwnMessage(msg.sender) }]"
        >
          <div class="msg-header">
            <span class="msg-sender">{{ msg.sender }}</span>
            <span class="msg-time">{{ formatDate(msg.created_at) }}</span>
          </div>
          <p class="msg-text">{{ msg.message }}</p>
        </div>
      </div>

      <div v-if="ticket.status === 'open'" class="reply-area">
        <textarea
          v-model="replyText"
          class="reply-input"
          rows="3"
          :placeholder="t('reseller_tickets.reply')"
          @keydown.ctrl.enter="sendReply"
        />
        <button class="send-btn" :disabled="replying || !replyText.trim()" @click="sendReply">
          {{ replying ? '...' : t('reseller_tickets.reply') }}
        </button>
      </div>
    </template>
  </div>
</template>

<style scoped>
.ticket-detail {
  padding: var(--space-6, 24px);
  max-width: 800px;
}

.ticket-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--space-4, 16px);
}

.back-btn {
  background: none;
  border: none;
  color: var(--color-primary, #2563eb);
  font-size: 13px;
  cursor: pointer;
  padding: 4px 0;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 10px;
}

.status-badge {
  font-size: 11px;
  padding: 3px 8px;
  border-radius: 10px;
  font-weight: var(--font-semibold, 600);
  text-transform: capitalize;
}

.status-badge.open {
  background: rgba(34, 197, 94, 0.15);
  color: #22c55e;
}

.status-badge.closed {
  background: rgba(107, 114, 128, 0.15);
  color: #9ca3af;
}

.close-btn {
  padding: 6px 12px;
  border-radius: var(--radius-sm, 6px);
  background: rgba(239, 68, 68, 0.12);
  color: #ef4444;
  border: none;
  font-size: 12px;
  cursor: pointer;
}

.close-btn:hover {
  background: rgba(239, 68, 68, 0.2);
}

.ticket-subject {
  font-size: var(--text-xl, 18px);
  font-weight: var(--font-bold, 700);
  margin: 0 0 4px;
}

.ticket-date {
  font-size: 12px;
  color: var(--color-muted, #8b98a5);
  display: block;
  margin-bottom: var(--space-5, 20px);
}

.loading-state {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.skeleton-row {
  background: var(--color-surface-2, #1e2630);
  border-radius: var(--radius-md, 8px);
  animation: pulse 1.5s infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

.messages-container {
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-height: 500px;
  overflow-y: auto;
  padding: var(--space-3, 12px) 0;
}

.message-bubble {
  max-width: 85%;
  padding: 12px 16px;
  border-radius: 12px;
  background: var(--color-surface-2, #1e2630);
  border: 1px solid var(--color-border, #28333f);
}

.message-bubble.own {
  align-self: flex-end;
  background: rgba(37, 99, 235, 0.1);
  border-color: rgba(37, 99, 235, 0.25);
}

.msg-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 6px;
}

.msg-sender {
  font-size: 11px;
  font-weight: var(--font-semibold, 600);
  color: var(--color-primary, #2563eb);
}

.msg-time {
  font-size: 10px;
  color: var(--color-muted, #8b98a5);
}

.msg-text {
  margin: 0;
  font-size: 13px;
  color: var(--color-text, #e6edf3);
  line-height: 1.5;
  white-space: pre-wrap;
}

.reply-area {
  margin-top: var(--space-4, 16px);
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.reply-input {
  width: 100%;
  padding: 12px 14px;
  border: 1px solid var(--color-border, #28333f);
  border-radius: var(--radius-md, 8px);
  background: var(--color-surface, #0b1120);
  color: var(--color-text, #e6edf3);
  font-size: 13px;
  font-family: inherit;
  resize: vertical;
  min-height: 60px;
}

.reply-input:focus {
  outline: none;
  border-color: var(--color-primary, #2563eb);
}

.send-btn {
  align-self: flex-end;
  padding: 8px 20px;
  border-radius: var(--radius-sm, 6px);
  background: var(--color-primary, #2563eb);
  color: #fff;
  border: none;
  font-size: 13px;
  font-weight: var(--font-semibold, 600);
  cursor: pointer;
}

.send-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
