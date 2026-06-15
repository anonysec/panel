<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useResellersStore } from '@/stores/resellers'
import KDataTable from '@koris/ui/KDataTable.vue'
import KButton from '@koris/ui/KButton.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'

const store = useResellersStore()
const showForm = ref(false)
const showCreditForm = ref(false)
const creditTarget = ref<{ id: number; username: string } | null>(null)
const creating = ref(false)
const adjusting = ref(false)

const resellerForm = ref({ username: '', password: '' })
const creditForm = ref({ amount: '' })

const columns = [
  { key: 'username', label: 'Username', sortable: true },
  { key: 'credit', label: 'Balance', sortable: true, align: 'right' as const },
  { key: 'created_at', label: 'Created', sortable: true },
  { key: 'actions', label: 'Actions', align: 'center' as const },
]

const txColumns = [
  { key: 'created_at', label: 'Date', sortable: true },
  { key: 'amount', label: 'Amount', sortable: true, align: 'right' as const },
  { key: 'description', label: 'Description' },
]

async function handleCreate() {
  creating.value = true
  await store.createReseller(resellerForm.value.username, resellerForm.value.password)
  resellerForm.value = { username: '', password: '' }
  creating.value = false
  showForm.value = false
}

function openCreditAdjust(reseller: any) {
  creditTarget.value = { id: reseller.id, username: reseller.username }
  creditForm.value = { amount: '' }
  showCreditForm.value = true
}

async function handleCreditAdjust() {
  if (!creditTarget.value) return
  adjusting.value = true
  await store.adjustCredit(creditTarget.value.id, Number(creditForm.value.amount))
  adjusting.value = false
  showCreditForm.value = false
  creditTarget.value = null
}

async function handleDelete(id: number) {
  await store.deleteReseller(id)
}

onMounted(() => {
  store.loadResellers()
})
</script>

<template>
  <div class="page resellers-view">
    <header class="page-header">
      <h2 class="page-title">Resellers</h2>
      <KButton variant="primary" icon="+" @click="showForm = true">Add Reseller</KButton>
    </header>

    <!-- Create Form -->
    <div v-if="showForm" class="panel">
      <h4 class="panel-title">New Reseller</h4>
      <form class="inline-form" @submit.prevent="handleCreate">
        <KFormField name="reseller-user" label="Username" required>
          <template #default="{ fieldId }">
            <KInput :id="fieldId" v-model="resellerForm.username" placeholder="reseller_name" />
          </template>
        </KFormField>
        <KFormField name="reseller-pass" label="Password" required>
          <template #default="{ fieldId }">
            <KInput :id="fieldId" v-model="resellerForm.password" type="password" placeholder="Secure password" />
          </template>
        </KFormField>
        <div class="form-actions">
          <KButton variant="ghost" @click="showForm = false">Cancel</KButton>
          <KButton type="submit" variant="primary" :loading="creating">Create</KButton>
        </div>
      </form>
    </div>

    <!-- Credit Adjust Form -->
    <div v-if="showCreditForm" class="panel">
      <h4 class="panel-title">Adjust Credit: {{ creditTarget?.username }}</h4>
      <form class="inline-form" @submit.prevent="handleCreditAdjust">
        <KFormField name="credit-amount" label="Amount" hint="Positive to add, negative to deduct" required>
          <template #default="{ fieldId, describedBy }">
            <KInput :id="fieldId" v-model="creditForm.amount" type="number" placeholder="10.00" :aria-describedby="describedBy" />
          </template>
        </KFormField>
        <div class="form-actions">
          <KButton variant="ghost" @click="showCreditForm = false">Cancel</KButton>
          <KButton type="submit" variant="primary" :loading="adjusting">Adjust</KButton>
        </div>
      </form>
    </div>

    <!-- Resellers Table -->
    <KEmptyState
      v-if="!store.loading && store.list.length === 0"
      icon="🤝"
      title="No Resellers"
      description="Create your first reseller to start delegating sales."
    />

    <KDataTable
      v-else
      :columns="columns"
      :data="store.list"
      :loading="store.loading"
      :page-size="20"
      row-key="id"
    >
      <template #cell-credit="{ value }">
        <span class="credit-cell">${{ typeof value === 'number' ? value.toFixed(2) : '0.00' }}</span>
      </template>
      <template #cell-created_at="{ value }">
        {{ value?.slice(0, 10) }}
      </template>
      <template #cell-actions="{ row }">
        <div class="action-btns">
          <KButton variant="ghost" size="sm" @click.stop="openCreditAdjust(row)">Credit</KButton>
          <KButton variant="danger" size="sm" @click.stop="handleDelete(row.id)">Delete</KButton>
        </div>
      </template>
    </KDataTable>

    <!-- Transactions History -->
    <div v-if="store.transactions.length > 0" class="panel">
      <h4 class="panel-title">Recent Transactions</h4>
      <KDataTable
        :columns="txColumns"
        :data="store.transactions.slice(0, 20)"
        :page-size="10"
        row-key="id"
      >
        <template #cell-amount="{ value }">
          <span :class="{ 'text-success': value > 0, 'text-danger': value < 0 }">
            ${{ typeof value === 'number' ? value.toFixed(2) : value }}
          </span>
        </template>
        <template #cell-created_at="{ value }">
          {{ value?.slice(0, 10) }}
        </template>
      </KDataTable>
    </div>
  </div>
</template>

<style scoped>
.resellers-view { display: flex; flex-direction: column; gap: var(--space-5); }
.page-header { display: flex; align-items: center; justify-content: space-between; }
.page-title { margin: 0; font-size: var(--text-xl); font-weight: var(--font-bold); }

.panel { padding: var(--space-4); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); }
.panel-title { margin: 0 0 var(--space-3); font-size: var(--text-sm); font-weight: var(--font-semibold); }

.inline-form { display: flex; flex-direction: column; gap: var(--space-3); max-width: 400px; }
.form-actions { display: flex; justify-content: flex-end; gap: var(--space-2); }

.credit-cell { font-weight: var(--font-semibold); color: var(--color-accent); }
.action-btns { display: flex; gap: var(--space-1); }

.text-success { color: var(--color-success); }
.text-danger { color: var(--color-danger); }
</style>
