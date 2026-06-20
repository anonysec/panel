<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useResellersStore } from '@/stores/resellers'
import { usePlansStore } from '@/stores/plans'
import { useConfirm } from '@koris/composables/useConfirm'
import { useToast } from '@koris/composables/useToast'
import { useI18n } from '@koris/composables/useI18n'
import { formatDate } from '@koris/composables/useFormatDate'
import KDataTable from '@koris/ui/KDataTable.vue'
import KButton from '@koris/ui/KButton.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'
import KSelect from '@koris/ui/KSelect.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'

const { t } = useI18n()
const router = useRouter()
const store = useResellersStore()
const plansStore = usePlansStore()
const { confirm } = useConfirm()
const toast = useToast()

const showForm = ref(false)
const showCreditForm = ref(false)
const editingId = ref<number | null>(null)
const creditTarget = ref<{ id: number; username: string } | null>(null)
const saving = ref(false)
const adjusting = ref(false)

const resellerForm = ref({
  username: '',
  password: '',
  plan_id: '' as string | number,
})
const creditForm = ref({ amount: '' })

/** Only show quota plans for reseller user creation */
const quotaPlans = computed(() =>
  plansStore.list.filter((p) => p.is_active && (p.billing_type || 'quota') === 'quota')
)

const planOptions = computed(() =>
  quotaPlans.value.map((p) => ({
    value: String(p.id),
    label: `${p.name} (${p.data_gb}GB / ${p.duration_days}d — $${p.price})`,
  }))
)

const columns = [
  { key: 'username', label: t('resellers.username'), sortable: true },
  { key: 'status', label: t('resellers.status'), sortable: true },
  { key: 'credit', label: t('resellers.credit'), sortable: true, align: 'right' as const },
  { key: 'customer_count', label: t('resellers.customers'), sortable: true, align: 'right' as const },
  { key: 'created_at', label: t('resellers.created'), sortable: true },
  { key: 'actions', label: '', align: 'center' as const, width: '140px' },
]

const txColumns = [
  { key: 'created_at', label: t('resellers.tx_date'), sortable: true },
  { key: 'amount', label: t('resellers.tx_amount'), sortable: true, align: 'right' as const },
  { key: 'description', label: t('resellers.tx_description') },
]

function resetForm() {
  resellerForm.value = { username: '', password: '', plan_id: '' }
  editingId.value = null
  showForm.value = false
}

function openCreate() {
  resetForm()
  showForm.value = true
}

function openEdit(reseller: any) {
  resellerForm.value = {
    username: reseller.username,
    password: '',
    plan_id: reseller.default_plan_id ? String(reseller.default_plan_id) : '',
  }
  editingId.value = reseller.id
  showForm.value = true
}

async function handleSubmit() {
  saving.value = true
  if (editingId.value) {
    await store.updateReseller(editingId.value, {
      password: resellerForm.value.password || undefined,
      default_plan_id: resellerForm.value.plan_id ? Number(resellerForm.value.plan_id) : undefined,
    })
    toast.success(t('resellers.updated'))
  } else {
    const success = await store.createReseller(resellerForm.value.username, resellerForm.value.password)
    if (success) {
      toast.success(t('resellers.created_success'))
    } else {
      toast.error(t('resellers.create_error'))
    }
  }
  saving.value = false
  resetForm()
}

function openCreditAdjust(reseller: any) {
  creditTarget.value = { id: reseller.id, username: reseller.username }
  creditForm.value = { amount: '' }
  showCreditForm.value = true
}

async function handleCreditAdjust() {
  if (!creditTarget.value) return
  adjusting.value = true
  const success = await store.adjustCredit(creditTarget.value.id, Number(creditForm.value.amount))
  adjusting.value = false
  showCreditForm.value = false
  if (success) {
    toast.success(t('resellers.credit_adjusted'))
  }
  creditTarget.value = null
}

async function handleDelete(id: number, username: string) {
  const confirmed = await confirm({
    title: t('resellers.confirm_delete_title'),
    message: t('resellers.confirm_delete_msg').replace('{name}', username),
    variant: 'danger',
    icon: '\u26A0',
    confirmText: t('btn.delete'),
    cancelText: t('btn.cancel'),
  })
  if (!confirmed) return
  const success = await store.deleteReseller(id)
  if (success) {
    toast.success(t('resellers.deleted_success').replace('{name}', username))
  } else {
    toast.error(t('resellers.deleted_error').replace('{name}', username))
  }
}

onMounted(() => {
  store.loadResellers()
  plansStore.loadPlans()
})
</script>

<template>
  <div class="page resellers-view">
    <header class="page-header">
      <KButton variant="ghost" @click="router.push({ name: 'customers' })">← {{ t('customers.tab_customers') }}</KButton>
      <KButton variant="primary" icon="+" @click="openCreate">{{ t('resellers.add') }}</KButton>
    </header>

    <!-- Create/Edit Form -->
    <div v-if="showForm" class="panel">
      <h4 class="panel-title">{{ editingId ? t('resellers.edit') : t('resellers.new') }}</h4>
      <form class="reseller-form" @submit.prevent="handleSubmit">
        <div class="form-grid">
          <KFormField name="reseller-user" :label="t('resellers.username')" required>
            <template #default="{ fieldId }">
              <KInput
                :id="fieldId"
                v-model="resellerForm.username"
                placeholder="reseller_name"
                :disabled="!!editingId"
              />
            </template>
          </KFormField>
          <KFormField name="reseller-pass" :label="t('resellers.password')" :required="!editingId">
            <template #default="{ fieldId }">
              <KInput
                :id="fieldId"
                v-model="resellerForm.password"
                type="password"
                :placeholder="editingId ? t('resellers.password_unchanged') : t('resellers.password_placeholder')"
              />
            </template>
          </KFormField>
          <KFormField name="reseller-plan" :label="t('resellers.default_plan')" :hint="t('resellers.plan_hint')">
            <template #default="{ fieldId, describedBy }">
              <KSelect
                :id="fieldId"
                v-model="resellerForm.plan_id"
                :options="planOptions"
                :placeholder="t('resellers.select_plan')"
                :aria-describedby="describedBy"
              />
            </template>
          </KFormField>
        </div>
        <div class="form-actions">
          <KButton variant="ghost" @click="resetForm">{{ t('btn.cancel') }}</KButton>
          <KButton type="submit" variant="primary" :loading="saving">
            {{ editingId ? t('btn.save') : t('resellers.create') }}
          </KButton>
        </div>
      </form>
    </div>

    <!-- Credit Adjust Form -->
    <div v-if="showCreditForm" class="panel">
      <h4 class="panel-title">{{ t('resellers.adjust_credit') }}: {{ creditTarget?.username }}</h4>
      <form class="reseller-form" @submit.prevent="handleCreditAdjust">
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
      v-if="!store.loading && store.list.length === 0 && !showForm"
      icon="🤝"
      :title="t('resellers.empty_title')"
      :description="t('resellers.empty_desc')"
    />

    <KDataTable
      v-else-if="store.list.length > 0 || store.loading"
      :columns="columns"
      :data="store.list"
      :loading="store.loading"
      :page-size="20"
      row-key="id"
    >
      <template #cell-status="{ row }">
        <KStatusPill :status="row.is_active ? 'active' : 'disabled'" size="sm" />
      </template>
      <template #cell-credit="{ value }">
        <span class="credit-cell">${{ typeof value === 'number' ? value.toFixed(2) : '0.00' }}</span>
      </template>
      <template #cell-customer_count="{ row }">
        <span class="customer-count">{{ row.customer_count ?? 0 }}</span>
      </template>
      <template #cell-created_at="{ value }">
        {{ formatDate(value) }}
      </template>
      <template #cell-actions="{ row }">
        <div class="action-btns">
          <KButton variant="ghost" size="sm" @click.stop="openEdit(row)">{{ t('btn.edit') }}</KButton>
          <KButton variant="ghost" size="sm" @click.stop="openCreditAdjust(row)">{{ t('resellers.credit') }}</KButton>
          <KButton variant="danger" size="sm" @click.stop="handleDelete(row.id, row.username)">{{ t('btn.delete') }}</KButton>
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
          {{ formatDate(value) }}
        </template>
      </KDataTable>
    </div>
  </div>
</template>

<style scoped>
.resellers-view { display: flex; flex-direction: column; gap: var(--space-5); }
.page-header { display: flex; align-items: center; justify-content: space-between; }

.panel { padding: var(--space-5); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); }
.panel-title { margin: 0 0 var(--space-4); font-size: var(--text-base); font-weight: var(--font-semibold); }

.reseller-form { display: flex; flex-direction: column; gap: var(--space-4); }
.form-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: var(--space-4); }
.form-actions { display: flex; justify-content: flex-end; gap: var(--space-2); padding-top: var(--space-2); }

.credit-cell { font-weight: var(--font-semibold); color: var(--color-accent); }
.customer-count { font-weight: var(--font-medium); color: var(--color-muted); }
.action-btns { display: flex; gap: var(--space-1); }

.text-success { color: var(--color-success, #22c55e); }
.text-danger { color: var(--color-danger, #ef4444); }

@media (max-width: 640px) {
  .page-header { flex-direction: column; align-items: flex-start; gap: var(--space-2); }
  .form-grid { grid-template-columns: 1fr; }
}
</style>
