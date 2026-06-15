<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useTicketsStore } from '@/stores/tickets'
import KButton from '@koris/ui/KButton.vue'
import KTextarea from '@koris/ui/KTextarea.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'

const props = defineProps<{ id: string }>()
const router = useRouter()
const store = useTicketsStore()

const replyText = ref('')
const sending = ref(false)

const ticket = computed(() => store.detail)
const messages = computed(() => ticket.value?.messages ?? [])

async function sendReply() {
  if (!replyText.value.trim()) return
  sending.value = true
  await store.replyToTicket(Number(props.id), replyText.value)
  replyText.value = ''
  sending.value = false
}

async function handleClose() {
  await store.closeTicket(Number(props.id))
}

async function handleReopen() {
  await store.openTicket(Number(props.id))
}

function priorityClass(priority: string): string {
  if (priority === 'urgent' || priority === 'high') return 'priority--high'
  if (priority === 'normal') return 'priority--normal'
  return 'priority--low'
}

onMounted(() => {
  store.loadTicketDetail(Number(props.id))
})
</script>

<template>
  <div class="page ticket-detail">
    <!-- Loading -->
    <div v-if="store.loading && !ticket" class="loading-state">
      <KSkeleton variant="rect" :width="'100%'" :height="60" />
      <KSkeleton variant="rect" :width="'100%'" :height="300" />
    </div>

    <template v-else-if="ticket">
      <!-- Ticket Header -->
      <header class="ticket-header">
        <div class="ticket-header__left">
          <KButton variant="ghost" size="sm" @click="router.back()">← Back</KButton>
          <div class="ticket-header__info">
            <h2 class="ticket-header__subject">{{ ticket.subject }}</h2>
            <div class="ticket-header__meta">
              <KStatusPill :status="ticket.status" />
              <span :class="['priority-badge', priorityClass(ticket.priority)]">
                {{ ticket.priority }}
              </span>
              <span class="text-muted">{{ ticket.username }}</span>
              <span class="text-muted">&middot; {{ ticket.created_at?.slice(0, 16) }}</span>
            </div>
          </div>
        </div>
        <div class="ticket-header__actions">
          <KButton
            v-if="ticket.status !== 'closed'"
            variant="danger"
            size="sm"
            @click="handleClose"
          >Close Ticket</KButton>
          <KButton
            v-else
            variant="primary"
            size="sm"
            @click="handleReopen"
          >Reopen</KButton>
        </div>
      </header>

      <!-- Messages Thread -->
      <section class="messages-thread" aria-label="Ticket conversation">
        <div
          v-for="msg in messages"
          :key="msg.id"
          :class="['message', msg.sender_type === 'admin' ? 'message--admin' : 'message--customer']"
        >
          <div class="message__bubble">
            <div class="message__header">
              <span class="message__sender">{{ msg.sender_name }}</span>
              <span class="message__time text-muted">{{ msg.created_at?.slice(0, 16) }}</span>
            </div>
            <p class="message__text">{{ msg.message }}</p>
          </div>
        </div>
        <p v-if="messages.length === 0" class="text-muted text-center">No messages yet.</p>
      </section>

      <!-- Reply Form -->
      <form v-if="ticket.status !== 'closed'" class="reply-form" @submit.prevent="sendReply">
        <KTextarea
          v-model="replyText"
          placeholder="Type your reply..."
          rows="3"
          aria-label="Reply message"
        />
        <div class="reply-form__actions">
          <KButton type="submit" variant="primary" :loading="sending" :disabled="!replyText.trim()">
            Send Reply
          </KButton>
        </div>
      </form>
    </template>

    <!-- Not Found -->
    <div v-else class="empty-state">
      <p class="text-muted">Ticket not found.</p>
      <KButton variant="ghost" @click="router.back()">Go Back</KButton>
    </div>
  </div>
</template>

<style scoped>
.ticket-detail { display: flex; flex-direction: column; gap: var(--space-4); }
.loading-state { display: flex; flex-direction: column; gap: var(--space-4); }

.ticket-header { display: flex; justify-content: space-between; align-items: flex-start; padding: var(--space-4); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); }
.ticket-header__left { display: flex; align-items: flex-start; gap: var(--space-3); }
.ticket-header__info { display: flex; flex-direction: column; gap: var(--space-1); }
.ticket-header__subject { margin: 0; font-size: var(--text-lg); font-weight: var(--font-bold); }
.ticket-header__meta { display: flex; align-items: center; gap: var(--space-2); flex-wrap: wrap; }
.ticket-header__actions { display: flex; gap: var(--space-2); }

.priority-badge { font-size: var(--text-xs); font-weight: var(--font-medium); padding: 2px 8px; border-radius: var(--radius-full); text-transform: capitalize; }
.priority--high { background: rgba(239, 68, 68, 0.1); color: var(--color-danger); }
.priority--normal { background: rgba(37, 99, 235, 0.1); color: var(--color-primary); }
.priority--low { background: rgba(139, 152, 165, 0.1); color: var(--color-muted); }

.messages-thread { display: flex; flex-direction: column; gap: var(--space-3); padding: var(--space-4); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); min-height: 200px; max-height: 500px; overflow-y: auto; }

.message { display: flex; max-width: 75%; }
.message--admin { align-self: flex-end; }
.message--customer { align-self: flex-start; }

.message__bubble { padding: var(--space-3); border-radius: var(--radius-lg); font-size: var(--text-sm); }
.message--admin .message__bubble { background: rgba(37, 99, 235, 0.1); border-bottom-right-radius: var(--radius-sm); }
.message--customer .message__bubble { background: var(--color-surface-2); border-bottom-left-radius: var(--radius-sm); }

.message__header { display: flex; justify-content: space-between; gap: var(--space-3); margin-bottom: var(--space-1); }
.message__sender { font-weight: var(--font-semibold); font-size: var(--text-xs); }
.message__time { font-size: var(--text-xs); }
.message__text { margin: 0; line-height: 1.5; white-space: pre-wrap; }

.reply-form { display: flex; flex-direction: column; gap: var(--space-3); padding: var(--space-4); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); }
.reply-form__actions { display: flex; justify-content: flex-end; }

.text-muted { color: var(--color-muted); }
.text-center { text-align: center; }
.empty-state { text-align: center; padding: var(--space-12); }
</style>
