<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { usePortalTicketsStore } from '@/stores/tickets'
import { formatDate } from '@koris/composables/useFormatDate'
import KButton from '@koris/ui/KButton.vue'
import KDataTable from '@koris/ui/KDataTable.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'
import KSelect from '@koris/ui/KSelect.vue'
import KTextarea from '@koris/ui/KTextarea.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'
import TicketThread from '@/components/TicketThread.vue'

const ticketsStore = usePortalTicketsStore()

const showCreateForm = ref(false)
const ticketForm = ref({
  subject: '',
  priority: 'normal',
  message: '',
})
const replyMessage = ref('')
const notice = ref('')

onMounted(() => {
  ticketsStore.loadTickets()
})

const selectedTicket = computed(() => ticketsStore.detail)

const ticketColumns = [
  { key: 'subject', label: 'Subject' },
  { key: 'priority', label: 'Priority' },
  { key: 'status', label: 'Status', sortable: true },
  { key: 'created_at', label: 'Date', sortable: true },
  { key: 'actions', label: '' },
]

async function handleCreateTicket() {
  if (!ticketForm.value.subject || !ticketForm.value.message) return
  notice.value = ''
  const id = await ticketsStore.createTicket(ticketForm.value)
  if (id) {
    notice.value = 'Ticket created successfully.'
    ticketForm.value = { subject: '', priority: 'normal', message: '' }
    showCreateForm.value = false
    await ticketsStore.loadTicketDetail(id)
  }
}

async function handleViewTicket(id: number) {
  await ticketsStore.loadTicketDetail(id)
}

async function handleReply() {
  if (!selectedTicket.value || !replyMessage.value.trim()) return
  notice.value = ''
  const success = await ticketsStore.replyToTicket(selectedTicket.value.id, replyMessage.value)
  if (success) {
    notice.value = 'Reply sent.'
    replyMessage.value = ''
  }
}

async function handleCloseTicket() {
  if (!selectedTicket.value) return
  const success = await ticketsStore.closeTicket(selectedTicket.value.id)
  if (success) {
    notice.value = 'Ticket closed.'
  }
}

function handleBack() {
  ticketsStore.clearDetail()
}
</script>
<template>
  <div class="support">
    <div class="support__header">
      <h1 class="support__title">Support</h1>
      <KButton
        v-if="!selectedTicket && !showCreateForm"
        variant="primary"
        size="sm"
        @click="showCreateForm = true"
      >
        + New Ticket
      </KButton>
      <KButton
        v-if="selectedTicket"
        variant="ghost"
        size="sm"
        @click="handleBack"
      >
        ← Back to Tickets
      </KButton>
    </div>

    <div v-if="notice" class="support__notice" role="status">{{ notice }}</div>

    <KSkeleton v-if="ticketsStore.loading && !ticketsStore.list.length && !selectedTicket" type="card" :count="2" />

    <!-- Create Ticket Form -->
    <section v-else-if="showCreateForm && !selectedTicket" class="support__section">
      <h2 class="support__section-title">Create New Ticket</h2>
      <form class="support__form" @submit.prevent="handleCreateTicket">
        <KFormField label="Subject" :required="true">
          <KInput v-model="ticketForm.subject" placeholder="Brief description of your issue" />
        </KFormField>

        <KFormField label="Priority">
          <KSelect v-model="ticketForm.priority">
            <option value="low">Low</option>
            <option value="normal">Normal</option>
            <option value="high">High</option>
            <option value="urgent">Urgent</option>
          </KSelect>
        </KFormField>

        <KFormField label="Message" :required="true">
          <KTextarea v-model="ticketForm.message" placeholder="Describe your issue in detail..." :rows="5" />
        </KFormField>

        <div class="support__form-actions">
          <KButton variant="ghost" @click="showCreateForm = false">Cancel</KButton>
          <KButton type="submit" variant="primary" :loading="ticketsStore.loading" :disabled="!ticketForm.subject || !ticketForm.message">
            Create Ticket
          </KButton>
        </div>
      </form>
    </section>

    <!-- Ticket Detail View -->
    <template v-else-if="selectedTicket">
      <section class="support__section">
        <div class="support__ticket-header">
          <h2 class="support__section-title">#{{ selectedTicket.id }}: {{ selectedTicket.subject }}</h2>
          <div class="support__ticket-meta">
            <KStatusPill :status="selectedTicket.status === 'open' ? 'active' : 'disabled'">
              {{ selectedTicket.status }}
            </KStatusPill>
            <span class="support__priority">{{ selectedTicket.priority }}</span>
            <KButton
              v-if="selectedTicket.status === 'open'"
              variant="ghost"
              size="sm"
              @click="handleCloseTicket"
            >
              Close Ticket
            </KButton>
          </div>
        </div>

        <TicketThread :messages="selectedTicket.messages" />

        <form v-if="selectedTicket.status === 'open'" class="support__reply-form" @submit.prevent="handleReply">
          <KFormField label="Reply">
            <KTextarea v-model="replyMessage" placeholder="Type your message..." :rows="3" />
          </KFormField>
          <KButton type="submit" variant="primary" :loading="ticketsStore.loading" :disabled="!replyMessage.trim()">
            Send Reply
          </KButton>
        </form>
      </section>
    </template>

    <!-- Ticket List -->
    <template v-else>
      <section class="support__section">
        <h2 class="support__section-title">My Tickets</h2>

        <KEmptyState
          v-if="!ticketsStore.list.length"
          title="No tickets yet"
          description="Create a ticket if you need help with your account."
          icon="🎫"
        />

        <KDataTable
          v-else
          :columns="ticketColumns"
          :data="ticketsStore.list"
          :loading="ticketsStore.loading"
        >
          <template #cell-priority="{ row }">
            <KStatusPill status="expired">{{ row.priority }}</KStatusPill>
          </template>
          <template #cell-status="{ row }">
            <KStatusPill :status="row.status === 'open' ? 'active' : 'disabled'">
              {{ row.status }}
            </KStatusPill>
          </template>
          <template #cell-created_at="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
          <template #cell-actions="{ row }">
            <KButton variant="ghost" size="sm" @click="handleViewTicket(row.id)">
              View
            </KButton>
          </template>
        </KDataTable>
      </section>
    </template>
  </div>
</template>
<style scoped>
.support__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--space-6);
}
.support__title {
  font-size: var(--text-2xl);
  font-weight: 700;
}
.support__notice {
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-md);
  background: rgba(34, 197, 94, 0.1);
  color: var(--color-success);
  font-size: var(--text-sm);
  margin-bottom: var(--space-4);
  border: 1px solid rgba(34, 197, 94, 0.2);
}
.support__section {
  padding: var(--space-5);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  margin-bottom: var(--space-4);
}
.support__section-title {
  font-size: var(--text-md);
  font-weight: 600;
  margin-bottom: var(--space-4);
}
.support__form {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  max-width: 500px;
}
.support__form-actions {
  display: flex;
  gap: var(--space-3);
  justify-content: flex-end;
}
.support__ticket-header {
  margin-bottom: var(--space-4);
}
.support__ticket-meta {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  margin-top: var(--space-2);
}
.support__priority {
  font-size: var(--text-xs);
  color: var(--color-muted);
}
.support__reply-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  margin-top: var(--space-4);
  padding-top: var(--space-4);
  border-top: 1px solid var(--color-border);
}
</style>
