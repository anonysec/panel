<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useResellersStore } from '@/stores/resellers'
import { useI18n } from '@koris/composables/useI18n'
import KDataTable from '@koris/ui/KDataTable.vue'
import KButton from '@koris/ui/KButton.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'

const { t } = useI18n()
const store = useResellersStore()
const showForm = ref(false)
const showCreditForm = ref(false)
const creditTarget = ref<{ id: number; username: string } | null>(null)
const creating = ref(false)
const adjusting = ref(false)

const resellerForm = ref({ username: '', password: '' })
const creditForm = ref({ amount: '' })

const columns = [
  { key: 'username', label: t('resellers.username'), sortable: true },
  { key: 'status', label: t('resellers.status'), sortable: true },
  { key: 'credit', label: t('resellers.credit'), sortable: true, align: 'right' as const },
  { key: 'customer_count', label: t('resellers.customers'), sortable: true, align: 'right' as const },
  { key: 'created_at', label: t('resellers.created'), sortable: true },
  { key: 'actions', label: '', align: 'center' as const },
]

const txColumns = [
  { key: 'created_at', label: t('resellers.tx_date'), sortable: true },
  { key: 'amount', label: t('resellers.tx_amount'), sortable: true, align: 'right' as const },
  { key: 'description', label: t('resellers.tx_description') },
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
      <h3 class="page-title">{{ t('resellers.title') }}</h3>
      <KButton variant="primary" icon="+" @click="showForm = true">{{ t('resellers.add') }}</KButton>
    </header>

    <!-- Create Form -->
    <div v-if="showForm" class="panel">
      <h4 class="panel-title">{{ t('resellers.new') }}</h4>
      <form class="inline-form" @submit.prevent="handleCreate">
        <KFormField name="reseller-user" :label="t('resellers.username')" required>
          <template #default="{ fieldId }">
            <KInput :id="fieldId" v-model="resellerForm.username" placeholder="reseller_name" />
          </template>
        </KFormField>
        <KFormField name="reseller-pass" :label="t('resellers.password')" required>
          <template #default="{ fieldId }">
            <KInput :id="fieldId" v-model="resellerForm.password" type="password" placeholder="Secure password" />
          </template>
        </KFormField>
        <div class="form-actions">
          <KButton variant="ghost" @click="showForm = false">{{ t('btn.cancel') }}</KButton>
          <KButton type="submit" variant="primary" :loading="creating">{{ t('resellers.create') }}</KButton>
        </div>
      </form>
    </div>

    <!-- Credit Adjust Form -->
    <div v-if="showCreditForm" class="panel">
      <h4 class="panel-title">{{ t('resellers.adjust_credit') }}: {{ creditTarget?.username }}</h4>
      <form class="inline-form" @submit.prevent="handleCreditAdjust">
        <KFormField name="credit-amount" :label="t('resellers.credit')" :hint="t('resellers.credit_hint')" required>
          <template #default="{ fieldId, describedBy }">
            <KInput :id="fieldId" v-model="creditForm.amount" type="number" placeholder="10.00" :aria-describedby="describedBy" />
          </template>
        </KFormField>
        <div class="form-actions">
          <KButton variant="ghost" @click="showCreditForm = false">{{ t('btn.cancel') }}</KButton>
          <KButton type="submit" variant="primary" :loading="adjusting">{{ t('resellers.adjust_credit') }}</KButton>
        </div>
      </form>
    </div>

    <!-- Resellers Table -->
    <KEmptyState
      v-if="!store.loading && store.list.length === 0"
      icon="🤝"
      :title="t('resellers.empty_title')"
      :description="t('resellers.empty_desc')"
    />

    <KDataTable
      v-else
      :columns="columns"
      :data="store.list"
      :loading="store.loading"
      :page-size="20"
      row-key="id"
    >
      <template #cell-status="{ row }">
        <KStatusPill :status="row.status || 'active'" size="sm" />
      </template>
      <template #cell-credit="{ value }">
        <span class="credit-cell">${{ typeof value === 'number' ? value.toFixed(2) : '0.00' }}</span>
      </template>
      <template #cell-customer_count="{ row }">
        <span class="customer-count">{{ row.customer_count ?? 0 }}</span>
      </template>
      <template #cell-created_at="{ value }">
        {{ value?.slice(0, 10) }}
      </template>
      <template #cell-actions="{ row }">
        <div class="action-btns">
          <KButton variant="ghost" size="sm" @click.stop="openCreditAdjust(row)">{{ t('resellers.credit') }}</KButton>
          <KButton variant="danger" size="sm" @click.stop="handleDelete(row.id)">{{ t('btn.delete') }}</KButton>
        </div>
      </template>
    </KDataTable>

    <!-- Transactions History -->
    <div v-if="store.transactions.length > 0" class="panel">
      <h4 class="panel-title">{{ t('resellers.transactions') }}</h4>
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
.page-title { margin: 0; font-size: var(--text-lg); font-weight: var(--font-semibold); }

.panel { padding: var(--space-4); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); }
.panel-title { margin: 0 0 var(--space-3); font-size: var(--text-sm); font-weight: var(--font-semibold); }

.inline-form { display: flex; flex-direction: column; gap: var(--space-3); max-width: 400px; }
.form-actions { display: flex; justify-content: flex-end; gap: var(--space-2); }

.credit-cell { font-weight: var(--font-semibold); color: var(--color-accent); }
.customer-count { font-weight: var(--font-medium); color: var(--color-muted); }
.action-btns { display: flex; gap: var(--space-1); }

.text-success { color: var(--color-success, #22c55e); }
.text-danger { color: var(--color-danger, #ef4444); }

@media (max-width: 640px) {
  .page-header { flex-direction: column; align-items: flex-start; gap: var(--space-2); }
  .inline-form { max-width: 100%; }
}
</style>
