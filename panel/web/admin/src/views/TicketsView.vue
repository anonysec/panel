<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useTicketsStore } from '@/stores/tickets'
import KButton from '@koris/ui/KButton.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'
import KSelect from '@koris/ui/KSelect.vue'
import KTextarea from '@koris/ui/KTextarea.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'

const router = useRouter()
const store = useTicketsStore()
const creating = ref(false)

const ticketForm = ref({
  username: '',
  subject: '',
  priority: 'normal',
  message: '',
})

function handleRowClick(ticket: any) {
  router.push({ name: 'ticket-detail', params: { id: String(ticket.id) } })
}

async function submitTicket() {
  creating.value = true
  const id = await store.createTicket({
    username: ticketForm.value.username,
    subject: ticketForm.value.subject,
    priority: ticketForm.value.priority,
    message: ticketForm.value.message,
  })
  creating.value = false
  if (id) {
    ticketForm.value = { username: '', subject: '', priority: 'normal', message: '' }
    router.push({ name: 'ticket-detail', params: { id: String(id) } })
  }
}

function priorityClass(priority: string): string {
  if (priority === 'urgent' || priority === 'high') return 'priority--high'
  if (priority === 'normal') return 'priority--normal'
  return 'priority--low'
}

onMounted(() => {
  store.loadTickets()
})
</script>

<template>
  <div class="page tickets-view">
    <header class="page-header">
      <h2 class="page-title">Tickets</h2>
    </header>

    <div class="tickets-layout">
      <!-- Left: Tickets Table -->
      <section class="tickets-table-section">
        <h4 class="section-title">Open Tickets</h4>
        <KEmptyState
          v-if="!store.loading && store.openTickets.length === 0"
          icon="🎫"
          title="No Open Tickets"
          description="All caught up! No tickets need attention."
        />
        <div v-else class="tickets-list">
          <div
            v-for="ticket in store.openTickets"
            :key="ticket.id"
            class="ticket-row"
            role="button"
            tabindex="0"
            @click="handleRowClick(ticket)"
            @keydown.enter="handleRowClick(ticket)"
          >
            <div class="ticket-row__main">
              <span class="ticket-row__subject">{{ ticket.subject }}</span>
              <span class="ticket-row__meta text-muted">
                {{ ticket.username }} &middot; {{ ticket.created_at?.slice(0, 10) }}
              </span>
            </div>
            <div class="ticket-row__right">
              <span :class="['priority-badge', priorityClass(ticket.priority)]">
                {{ ticket.priority }}
              </span>
              <KStatusPill :status="ticket.status" size="sm" />
            </div>
          </div>
        </div>

        <!-- Closed Tickets -->
        <div v-if="store.closedTickets.length > 0" class="closed-section">
          <h4 class="section-title">Closed Tickets ({{ store.closedTickets.length }})</h4>
          <div class="tickets-list tickets-list--closed">
            <div
              v-for="ticket in store.closedTickets.slice(0, 5)"
              :key="ticket.id"
              class="ticket-row ticket-row--closed"
              role="button"
              tabindex="0"
              @click="handleRowClick(ticket)"
              @keydown.enter="handleRowClick(ticket)"
            >
              <div class="ticket-row__main">
                <span class="ticket-row__subject">{{ ticket.subject }}</span>
                <span class="ticket-row__meta text-muted">{{ ticket.username }}</span>
              </div>
              <KStatusPill :status="ticket.status" size="sm" />
            </div>
          </div>
        </div>
      </section>

      <!-- Right: Create Ticket Form -->
      <aside class="tickets-sidebar">
        <div class="panel">
          <h4 class="panel-title">Create Ticket</h4>
          <form class="ticket-form" @submit.prevent="submitTicket">
            <KFormField name="ticket-user" label="Username" required>
              <template #default="{ fieldId }">
                <KInput :id="fieldId" v-model="ticketForm.username" placeholder="customer_username" />
              </template>
            </KFormField>
            <KFormField name="ticket-subject" label="Subject" required>
              <template #default="{ fieldId }">
                <KInput :id="fieldId" v-model="ticketForm.subject" placeholder="Ticket subject" />
              </template>
            </KFormField>
            <KFormField name="ticket-priority" label="Priority">
              <template #default="{ fieldId }">
                <KSelect
                  :id="fieldId"
                  v-model="ticketForm.priority"
                  :options="[
                    { label: 'Low', value: 'low' },
                    { label: 'Normal', value: 'normal' },
                    { label: 'High', value: 'high' },
                    { label: 'Urgent', value: 'urgent' },
                  ]"
                />
              </template>
            </KFormField>
            <KFormField name="ticket-message" label="Message" required>
              <template #default="{ fieldId }">
                <KTextarea :id="fieldId" v-model="ticketForm.message" rows="4" placeholder="Describe the issue..." />
              </template>
            </KFormField>
            <KButton type="submit" variant="primary" :loading="creating" full-width>
              Create Ticket
            </KButton>
          </form>
        </div>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.tickets-view { display: flex; flex-direction: column; gap: var(--space-5); }
.page-header { display: flex; align-items: center; justify-content: space-between; }
.page-title { margin: 0; font-size: var(--text-xl); font-weight: var(--font-bold); }

.tickets-layout { display: grid; grid-template-columns: 1fr 320px; gap: var(--space-5); }

.tickets-table-section { display: flex; flex-direction: column; gap: var(--space-4); }
.section-title { margin: 0; font-size: var(--text-sm); font-weight: var(--font-semibold); color: var(--color-text); }

.tickets-list { display: flex; flex-direction: column; gap: var(--space-1); }
.ticket-row { display: flex; justify-content: space-between; align-items: center; padding: var(--space-3) var(--space-4); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-md); cursor: pointer; transition: all var(--duration-fast); }
.ticket-row:hover { border-color: var(--color-primary); background: var(--color-surface-2); }
.ticket-row:focus-visible { outline: 2px solid var(--color-accent); outline-offset: -2px; }
.ticket-row--closed { opacity: 0.7; }

.ticket-row__main { display: flex; flex-direction: column; gap: var(--space-1); }
.ticket-row__subject { font-weight: var(--font-medium); font-size: var(--text-sm); }
.ticket-row__meta { font-size: var(--text-xs); }
.ticket-row__right { display: flex; align-items: center; gap: var(--space-2); }

.priority-badge { font-size: var(--text-xs); font-weight: var(--font-medium); padding: 2px 8px; border-radius: var(--radius-full); text-transform: capitalize; }
.priority--high { background: rgba(239, 68, 68, 0.1); color: var(--color-danger); }
.priority--normal { background: rgba(37, 99, 235, 0.1); color: var(--color-primary); }
.priority--low { background: rgba(139, 152, 165, 0.1); color: var(--color-muted); }

.closed-section { margin-top: var(--space-4); }
.tickets-list--closed { opacity: 0.8; }

.tickets-sidebar { display: flex; flex-direction: column; }
.panel { padding: var(--space-4); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); }
.panel-title { margin: 0 0 var(--space-3); font-size: var(--text-sm); font-weight: var(--font-semibold); }
.ticket-form { display: flex; flex-direction: column; gap: var(--space-3); }

.text-muted { color: var(--color-muted); }

@media (max-width: 900px) {
  .tickets-layout { grid-template-columns: 1fr; }
}
</style>
