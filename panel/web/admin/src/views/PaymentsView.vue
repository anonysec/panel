<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { usePaymentsStore } from '@/stores/payments'
import { useToast } from '@koris/composables/useToast'
import { useI18n } from '@koris/composables/useI18n'
import KDataTable from '@koris/ui/KDataTable.vue'
import KButton from '@koris/ui/KButton.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'
import KSelect from '@koris/ui/KSelect.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KDrawer from '@koris/ui/KDrawer.vue'

const { t } = useI18n()
const store = usePaymentsStore()
const toast = useToast()
const creatingPayment = ref(false)
const showRecordDrawer = ref(false)
const showMethodDrawer = ref(false)
const savingMethod = ref(false)

const paymentForm = ref({
  username: '',
  amount: '',
  method: '',
  description: '',
})

const methodForm = ref({
  name: '',
  type: '',
  instructions: '',
  is_active: true,
  sort_order: 0,
  wallet_address: '',
  network: '',
  currency: '',
})

const cryptoNetworkOptions = computed(() => [
  { label: 'BTC', value: 'BTC' },
  { label: 'ETH', value: 'ETH' },
  { label: 'TRC20', value: 'TRC20' },
  { label: 'ERC20', value: 'ERC20' },
  { label: 'BEP20', value: 'BEP20' },
])

const cryptoCurrencyOptions = computed(() => [
  { label: 'BTC', value: 'BTC' },
  { label: 'USDT', value: 'USDT' },
  { label: 'ETH', value: 'ETH' },
  { label: 'BNB', value: 'BNB' },
])

const tableColumns = computed(() => [
  { key: 'username', label: t('payments.col_user'), sortable: true },
  { key: 'amount', label: t('payments.col_amount'), sortable: true, align: 'right' as const },
  { key: 'method', label: t('payments.col_method'), sortable: true },
  { key: 'status', label: t('payments.col_status'), sortable: true, filterable: true, filterType: 'select' as const, filterOptions: [
    { label: t('status.pending'), value: 'pending' },
    { label: t('status.approved'), value: 'approved' },
    { label: t('status.rejected'), value: 'rejected' },
  ]},
  { key: 'intent_label', label: t('payments.col_intent') },
  { key: 'created_at', label: t('payments.col_date'), sortable: true },
  { key: 'actions', label: t('payments.col_actions'), align: 'center' as const },
])

const methodTypeOptions = computed(() => [
  { label: t('payments.type_bank_transfer'), value: 'bank_transfer' },
  { label: t('payments.type_crypto'), value: 'crypto' },
  { label: t('payments.type_card'), value: 'card' },
  { label: t('payments.type_other'), value: 'other' },
])

async function handleApprove(id: number) {
  await store.approvePayment(id)
}

async function handleReject(id: number) {
  await store.rejectPayment(id)
}

async function submitPayment() {
  creatingPayment.value = true
  const success = await store.createManualPayment({
    username: paymentForm.value.username,
    amount: Number(paymentForm.value.amount),
    method: paymentForm.value.method,
    description: paymentForm.value.description,
  })
  creatingPayment.value = false
  if (success) {
    paymentForm.value = { username: '', amount: '', method: '', description: '' }
    showRecordDrawer.value = false
    toast.success(t('payments.record_success'))
  } else {
    toast.error(t('payments.record_error'))
  }
}

async function submitMethod() {
  savingMethod.value = true
  let instructions = methodForm.value.instructions
  if (methodForm.value.type === 'crypto') {
    instructions = JSON.stringify({
      wallet_address: methodForm.value.wallet_address,
      network: methodForm.value.network,
      currency: methodForm.value.currency,
      note: methodForm.value.instructions,
    })
  }
  const success = await store.savePaymentMethod({
    name: methodForm.value.name,
    type: methodForm.value.type,
    instructions,
    is_active: methodForm.value.is_active,
    sort_order: Number(methodForm.value.sort_order),
  })
  savingMethod.value = false
  if (success) {
    methodForm.value = { name: '', type: '', instructions: '', is_active: true, sort_order: 0, wallet_address: '', network: '', currency: '' }
    showMethodDrawer.value = false
    toast.success(t('payments.method_create_success'))
  } else {
    toast.error(t('payments.method_create_error'))
  }
}

function parseCryptoInstructions(instructions: string): { wallet_address?: string; network?: string; currency?: string; note?: string } | null {
  try {
    const data = JSON.parse(instructions)
    if (data && typeof data === 'object' && data.wallet_address) return data
    return null
  } catch {
    return null
  }
}

onMounted(() => {
  store.loadPayments()
})
</script>

<template>
  <div class="page payments-view">
    <header class="page-header">
      <KButton variant="primary" @click="showRecordDrawer = true">
        {{ t('payments.record_payment') }}
      </KButton>
    </header>

    <!-- Payments Table -->
    <section class="payments-table-section">
      <KDataTable
        :columns="tableColumns"
        :data="store.paginatedList"
        :loading="store.loading"
        :page-size="store.pageSize"
        row-key="id"
      >
        <template #cell-amount="{ value }">
          <span class="amount-cell">${{ typeof value === 'number' ? value.toFixed(2) : value }}</span>
        </template>
        <template #cell-status="{ value }">
          <KStatusPill :status="value" size="sm" />
        </template>
        <template #cell-created_at="{ value }">
          {{ value?.slice(0, 10) }}
        </template>
        <template #cell-actions="{ row }">
          <div v-if="row.status === 'pending'" class="action-btns">
            <KButton variant="primary" size="sm" @click.stop="handleApprove(row.id)">{{ t('payments.approve') }}</KButton>
            <KButton variant="danger" size="sm" @click.stop="handleReject(row.id)">{{ t('payments.reject') }}</KButton>
          </div>
          <span v-else class="text-muted">-</span>
        </template>
      </KDataTable>
    </section>

    <!-- Payment Methods Section (always visible) -->
    <section class="panel">
      <div class="panel-header">
        <h4 class="panel-title">{{ t('payments.payment_methods') }}</h4>
        <KButton variant="ghost" size="sm" @click="showMethodDrawer = true">{{ t('payments.add_method') }}</KButton>
      </div>
      <div class="methods-list">
        <div v-for="method in store.paymentMethods" :key="method.id" class="method-item">
          <div class="method-item__info">
            <span class="method-item__name">{{ method.name }}</span>
            <span class="method-item__type text-muted">{{ method.type }}</span>
            <template v-if="method.type === 'crypto'">
              <span v-if="parseCryptoInstructions(method.instructions)" class="method-item__crypto text-muted">
                {{ parseCryptoInstructions(method.instructions)?.network }} &middot; {{ parseCryptoInstructions(method.instructions)?.currency }} &middot; {{ parseCryptoInstructions(method.instructions)?.wallet_address?.slice(0, 12) }}...
              </span>
            </template>
          </div>
          <KStatusPill :status="method.is_active ? 'active' : 'disabled'" size="sm" />
        </div>
        <p v-if="store.paymentMethods.length === 0" class="text-muted text-sm">{{ t('payments.no_methods') }}</p>
      </div>
    </section>

    <!-- Record Payment Drawer -->
    <KDrawer :open="showRecordDrawer" :title="t('payments.record_payment')" side="right" @close="showRecordDrawer = false">
      <form class="payment-form" @submit.prevent="submitPayment">
        <KFormField name="pay-username" :label="t('payments.form_username')" required>
          <template #default="{ fieldId }">
            <KInput :id="fieldId" v-model="paymentForm.username" placeholder="customer_username" />
          </template>
        </KFormField>
        <KFormField name="pay-amount" :label="t('payments.form_amount')" required>
          <template #default="{ fieldId }">
            <KInput :id="fieldId" v-model="paymentForm.amount" type="number" placeholder="10.00" />
          </template>
        </KFormField>
        <KFormField name="pay-method" :label="t('payments.form_method')" required>
          <template #default="{ fieldId }">
            <KSelect
              :id="fieldId"
              v-model="paymentForm.method"
              :options="store.activePaymentMethods.map(m => ({ label: m.name, value: m.name }))"
              :placeholder="t('payments.select_method')"
            />
          </template>
        </KFormField>
        <KFormField name="pay-desc" :label="t('payments.form_description')">
          <template #default="{ fieldId }">
            <KInput :id="fieldId" v-model="paymentForm.description" :placeholder="t('payments.optional_note')" />
          </template>
        </KFormField>
        <KButton type="submit" variant="primary" :loading="creatingPayment" full-width>
          {{ t('payments.record_payment') }}
        </KButton>
      </form>
    </KDrawer>

    <!-- Add Payment Method Drawer -->
    <KDrawer :open="showMethodDrawer" :title="t('payments.add_payment_method')" side="right" @close="showMethodDrawer = false">
      <form class="payment-form" @submit.prevent="submitMethod">
        <KFormField name="method-name" :label="t('payments.method_name')" required>
          <template #default="{ fieldId }">
            <KInput :id="fieldId" v-model="methodForm.name" :placeholder="t('payments.method_name_placeholder')" />
          </template>
        </KFormField>
        <KFormField name="method-type" :label="t('payments.method_type')" required>
          <template #default="{ fieldId }">
            <KSelect
              :id="fieldId"
              v-model="methodForm.type"
              :options="methodTypeOptions"
              :placeholder="t('payments.select_type')"
            />
          </template>
        </KFormField>
        <!-- Crypto-specific fields -->
        <template v-if="methodForm.type === 'crypto'">
          <KFormField name="method-wallet" :label="t('payments.crypto_wallet')" required>
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="methodForm.wallet_address" :placeholder="t('payments.crypto_wallet_placeholder')" />
            </template>
          </KFormField>
          <KFormField name="method-network" :label="t('payments.crypto_network')" required>
            <template #default="{ fieldId }">
              <KSelect
                :id="fieldId"
                v-model="methodForm.network"
                :options="cryptoNetworkOptions"
                :placeholder="t('payments.crypto_select_network')"
              />
            </template>
          </KFormField>
          <KFormField name="method-currency" :label="t('payments.crypto_currency')" required>
            <template #default="{ fieldId }">
              <KSelect
                :id="fieldId"
                v-model="methodForm.currency"
                :options="cryptoCurrencyOptions"
                :placeholder="t('payments.crypto_select_currency')"
              />
            </template>
          </KFormField>
        </template>
        <KFormField name="method-instructions" :label="t('payments.method_instructions')">
          <template #default="{ fieldId }">
            <KInput :id="fieldId" v-model="methodForm.instructions" :placeholder="t('payments.method_instructions_placeholder')" />
          </template>
        </KFormField>
        <KFormField name="method-sort" :label="t('payments.method_sort_order')">
          <template #default="{ fieldId }">
            <KInput :id="fieldId" v-model="methodForm.sort_order" type="number" placeholder="0" />
          </template>
        </KFormField>
        <div class="toggle-field">
          <label class="toggle-label">
            <input type="checkbox" v-model="methodForm.is_active" />
            <span>{{ t('payments.method_active') }}</span>
          </label>
        </div>
        <KButton type="submit" variant="primary" :loading="savingMethod" full-width>
          {{ t('payments.create_method') }}
        </KButton>
      </form>
    </KDrawer>
  </div>
</template>

<style scoped>
.payments-view { display: flex; flex-direction: column; gap: var(--space-5); }
.page-header { display: flex; align-items: center; justify-content: flex-end; }

.panel { padding: var(--space-4); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); }
.panel-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: var(--space-3); }
.panel-title { margin: 0; font-size: var(--text-sm); font-weight: var(--font-semibold); }

.payment-form { display: flex; flex-direction: column; gap: var(--space-3); }

.methods-list { display: flex; flex-direction: column; gap: var(--space-2); }
.method-item { display: flex; justify-content: space-between; align-items: center; padding: var(--space-2) 0; border-bottom: 1px solid var(--color-border); }
.method-item:last-child { border-bottom: none; }
.method-item__info { display: flex; flex-direction: column; }
.method-item__name { font-size: var(--text-sm); font-weight: var(--font-medium); }
.method-item__type { font-size: var(--text-xs); }
.method-item__crypto { font-size: var(--text-xs); font-family: monospace; }

.payments-table-section { min-width: 0; }

.amount-cell { font-weight: var(--font-semibold); color: var(--color-success); }
.action-btns { display: flex; gap: var(--space-1); }

.toggle-field { padding: var(--space-2) 0; }
.toggle-label { display: flex; align-items: center; gap: var(--space-2); font-size: var(--text-sm); color: var(--color-text); cursor: pointer; }
.toggle-label input[type="checkbox"] { width: 1rem; height: 1rem; accent-color: var(--color-primary); }

.text-muted { color: var(--color-muted); }
.text-sm { font-size: var(--text-sm); }
</style>
