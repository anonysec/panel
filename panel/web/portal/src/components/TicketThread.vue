<script setup lang="ts">
import type { TicketMessage } from '@/stores/tickets'
import { formatDateTime } from '@koris/composables/useFormatDate'

interface Props {
  messages: TicketMessage[]
}

defineProps<Props>()
</script>
<template>
  <div class="ticket-thread">
    <div
      v-for="msg in messages"
      :key="msg.id"
      class="ticket-thread__message"
      :class="{ 'ticket-thread__message--admin': msg.sender_type === 'admin' }"
    >
      <div class="ticket-thread__header">
        <span class="ticket-thread__sender">{{ msg.sender_name }}</span>
        <span class="ticket-thread__badge" v-if="msg.sender_type === 'admin'">Staff</span>
        <span class="ticket-thread__time">{{ formatDateTime(msg.created_at) }}</span>
      </div>
      <p class="ticket-thread__body">{{ msg.message }}</p>
    </div>

    <div v-if="!messages.length" class="ticket-thread__empty">
      No messages in this ticket yet.
    </div>
  </div>
</template>
<style scoped>
.ticket-thread {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  max-height: 400px;
  overflow-y: auto;
  padding: var(--space-3) 0;
}
.ticket-thread__message {
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-md);
  background: var(--color-bg);
  border: 1px solid var(--color-border);
}
.ticket-thread__message--admin {
  background: rgba(37, 99, 235, 0.04);
  border-color: rgba(37, 99, 235, 0.15);
}
.ticket-thread__header {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin-bottom: var(--space-2);
}
.ticket-thread__sender {
  font-size: var(--text-sm);
  font-weight: 600;
}
.ticket-thread__badge {
  font-size: var(--text-xs);
  padding: 1px var(--space-2);
  border-radius: var(--radius-full);
  background: var(--color-primary);
  color: #fff;
}
.ticket-thread__time {
  font-size: var(--text-xs);
  color: var(--color-muted);
  margin-left: auto;
}
.ticket-thread__body {
  font-size: var(--text-sm);
  line-height: 1.5;
  color: var(--color-text);
  white-space: pre-wrap;
}
.ticket-thread__empty {
  text-align: center;
  padding: var(--space-6);
  color: var(--color-muted);
  font-size: var(--text-sm);
}
</style>
