<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { usePortalAuthStore } from '@/stores/auth'
import { useBillingStore } from '@/stores/billing'
import { useFreshData } from '@koris/composables/useFreshData'
import { formatDate } from '@koris/composables/useFormatDate'
import { useApi } from '@koris/composables/useApi'
import { useToast } from '@koris/composables/useToast'
import KButton from '@koris/ui/KButton.vue'
import KDataTable from '@koris/ui/KDataTable.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'
import KModal from '@koris/ui/KModal.vue'
import KSelect from '@koris/ui/KSelect.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'

interface WalletTransaction {
  id: number
  amount: number
  type: string
  description: string
  created_at: string
}

interface WalletTransactionsResponse {
  ok: boolean
  transactions: WalletTransaction[]
}

const auth = usePortalAuthStore()
const billing = useBillingStore()
const { get } = useApi()
const toast = useToast()

const paymentForm = ref({
  amount: 0,
  method: '',
  receipt: '',
})

const renewPlanId = ref<number>(0)
const notice = ref('')

// Modal state
const showRenewalModal = ref(false)
const showTopupModal = ref(false)
const confirming = ref(false)

// Wallet transactions state
const walletTransactions = ref<WalletTransaction[]>([])
const walletTransactionsLoading = ref(false)

const walletTransactionColumns = [
  { key: 'amount', label: 'Amount', sortable: true },
  { key: 'type', label: 'Type' },
  { key: 'description', label: 'Description' },
  { key: 'created_at', label: 'Date', sortable: true },
]

async function fetchWalletTransactions(): Promise<void> {
  walletTransactionsLoading.value = true
  try {
    const res = await get<WalletTransactionsResponse>('/api/portal/wallet-transactions')
    walletTransactions.value = res.transactions || []
  } catch {
    toast.error('Failed to load wallet transactions')
  } finally {
    walletTransactionsLoading.value = false
  }
}

useFreshData(async () => {
  await billing.loadBillingData()
  if (billing.paymentMethods.length && !paymentForm.value.method) {
    paymentForm.value.method = billing.paymentMethods[0].name
  }
  if (billing.activePlans.length && !renewPlanId.value) {
    renewPlanId.value = billing.activePlans[0].id
  }
})

onMounted(() => {
  fetchWalletTransactions()
})

const walletCredit = computed(() => auth.credit)
const selectedPlan = computed(() => billing.plans.find((p) => p.id === renewPlanId.value))
const selectedPaymentMethod = computed(() =>
  billing.paymentMethods.find((m) => m.name === paymentForm.value.method)
)
const requiredTopup = computed(() =>
  Math.max(0, (selectedPlan.value?.price || 0) - walletCredit.value)
)

const paymentColumns = [
  { key: 'amount', label: 'Amount', sortable: true },
  { key: 'method', label: 'Method' },
  { key: 'status', label: 'Status', sortable: true },
  { key: 'created_at', label: 'Date', sortable: true },
]

function formatMoney(value: number): string {
  return `${new Intl.NumberFormat('en', { maximumFractionDigits: 0 }).format(value)} IRT`
}

function formatTransactionAmount(value: number): string {
  const formatted = formatMoney(Math.abs(value))
  return value >= 0 ? `+${formatted}` : `-${formatted}`
}

function formatGB(value: number): string {
  return value > 0 ? `${value} GB` : 'Unlimited'
}

async function handleSubmitPayment() {
  if (paymentForm.value.amount <= 0) return
  showTopupModal.value = true
}

async function confirmTopup() {
  confirming.value = true
  try {
    const success = await billing.submitPayment({
      amount: paymentForm.value.amount,
      method: paymentForm.value.method,
      receipt: paymentForm.value.receipt,
    })
    if (success) {
      showTopupModal.value = false
      notice.value = 'Payment request submitted. Admin will review it.'
      paymentForm.value = { amount: 0, method: paymentForm.value.method, receipt: '' }
    } else {
      toast.error('Payment submission failed. Please try again.')
    }
  } catch {
    toast.error('Payment submission failed. Please try again.')
  } finally {
    confirming.value = false
  }
}

async function handleRenew() {
  if (!renewPlanId.value) return
  showRenewalModal.value = true
}

async function confirmRenewal() {
  confirming.value = true
  try {
    const res = await billing.renewPlan(renewPlanId.value)
    if (res?.renewed) {
      showRenewalModal.value = false
      notice.value = 'Plan activated. Wallet was charged.'
    } else if (res?.payment_required) {
      showRenewalModal.value = false
      notice.value = `Payment request #${res.payment_id} created for ${formatMoney(res.required_amount || 0)}.`
    } else {
      toast.error('Plan renewal failed. Please try again.')
    }
  } catch {
    toast.error('Plan renewal failed. Please try again.')
  } finally {
    confirming.value = false
  }
}
</script>
<template>
  <div class="billing">
    <h1 class="billing__title">Billing</h1>

    <div v-if="notice" class="billing__notice" role="status">{{ notice }}</div>

    <KSkeleton v-if="billing.loading && !billing.payments.length" type="card" :count="2" />

    <template v-else>
      <!-- Balance Card -->
      <div class="billing__balance">
        <div class="balance-card">
          <div class="balance-card__label">Wallet Balance</div>
          <div class="balance-card__value">{{ formatMoney(walletCredit) }}</div>
        </div>
      </div>

      <!-- Renew Plan Section -->
      <section class="billing__section">
        <h2 class="billing__section-title">Renew Plan</h2>
        <form class="billing__form" @submit.prevent="handleRenew">
          <KFormField label="Select Plan">
            <KSelect v-model="renewPlanId">
              <option v-for="plan in billing.activePlans" :key="plan.id" :value="plan.id">
                {{ plan.name }} — {{ formatGB(plan.data_gb) }} · {{ plan.duration_days }}d · {{ formatMoney(plan.price) }}
              </option>
            </KSelect>
          </KFormField>

          <div v-if="selectedPlan && requiredTopup > 0" class="billing__warning">
            Insufficient balance. A payment request will be created for {{ formatMoney(requiredTopup) }}.
          </div>

          <KButton type="submit" variant="primary" :loading="billing.loading" :disabled="!renewPlanId">
            Activate Plan
          </KButton>
        </form>
      </section>

      <!-- Top-up Wallet Section -->
      <section class="billing__section">
        <h2 class="billing__section-title">Top-up Wallet</h2>
        <form class="billing__form" @submit.prevent="handleSubmitPayment">
          <KFormField label="Amount" :required="true">
            <KInput v-model.number="paymentForm.amount" type="number" :min="1" placeholder="Amount" />
          </KFormField>

          <KFormField label="Payment Method">
            <KSelect v-model="paymentForm.method">
              <option v-for="m in billing.paymentMethods" :key="m.id" :value="m.name">
                {{ m.name }}
              </option>
            </KSelect>
          </KFormField>

          <div v-if="selectedPaymentMethod?.instructions" class="billing__instructions">
            {{ selectedPaymentMethod.instructions }}
          </div>

          <KFormField label="Receipt / Reference">
            <KInput v-model="paymentForm.receipt" placeholder="Transfer reference or receipt ID" />
          </KFormField>

          <KButton type="submit" variant="primary" :loading="billing.loading" :disabled="paymentForm.amount <= 0">
            Submit Payment
          </KButton>
        </form>
      </section>

      <!-- Payment History -->
      <section class="billing__section">
        <h2 class="billing__section-title">Payment History</h2>

        <KEmptyState
          v-if="!billing.payments.length"
          title="No payments yet"
          description="Your payment history will appear here."
          icon="💳"
        />

        <KDataTable
          v-else
          :columns="paymentColumns"
          :data="billing.payments"
          :loading="billing.loading"
        >
          <template #cell-amount="{ row }">
            <strong>{{ formatMoney(row.amount) }}</strong>
          </template>
          <template #cell-status="{ row }">
            <KStatusPill :status="row.status === 'approved' ? 'active' : row.status === 'rejected' ? 'disabled' : 'expired'">
              {{ row.status }}
            </KStatusPill>
          </template>
          <template #cell-created_at="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </KDataTable>
      </section>

      <!-- Wallet Transactions -->
      <section class="billing__section">
        <h2 class="billing__section-title">Wallet Transactions</h2>

        <KEmptyState
          v-if="!walletTransactionsLoading && !walletTransactions.length"
          title="No transactions yet"
          description="Your wallet transaction history will appear here."
          icon="📒"
        />

        <KDataTable
          v-else
          :columns="walletTransactionColumns"
          :data="walletTransactions"
          :loading="walletTransactionsLoading"
        >
          <template #cell-amount="{ row }">
            <span :class="row.amount >= 0 ? 'billing__credit' : 'billing__debit'">
              {{ formatTransactionAmount(row.amount) }}
            </span>
          </template>
          <template #cell-type="{ row }">
            {{ row.type }}
          </template>
          <template #cell-description="{ row }">
            {{ row.description }}
          </template>
          <template #cell-created_at="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </KDataTable>
      </section>
    </template>

    <!-- Renewal Confirmation Modal -->
    <KModal
      :open="showRenewalModal"
      title="Confirm Plan Renewal"
      @close="showRenewalModal = false"
    >
      <div class="billing__modal-details">
        <dl class="billing__modal-dl">
          <div class="billing__modal-row">
            <dt>Plan</dt>
            <dd>{{ selectedPlan?.name || '—' }}</dd>
          </div>
          <div class="billing__modal-row">
            <dt>Price</dt>
            <dd>{{ formatMoney(selectedPlan?.price || 0) }}</dd>
          </div>
          <div class="billing__modal-row">
            <dt>Wallet Balance</dt>
            <dd>{{ formatMoney(walletCredit) }}</dd>
          </div>
          <div class="billing__modal-row">
            <dt>Deduction</dt>
            <dd>{{ formatMoney(Math.min(selectedPlan?.price || 0, walletCredit)) }}</dd>
          </div>
        </dl>
      </div>
      <template #footer>
        <div class="billing__modal-actions">
          <KButton variant="ghost" @click="showRenewalModal = false">Cancel</KButton>
          <KButton
            variant="primary"
            :loading="confirming"
            :disabled="confirming"
            @click="confirmRenewal"
          >
            Confirm Renewal
          </KButton>
        </div>
      </template>
    </KModal>

    <!-- Top-up Confirmation Modal -->
    <KModal
      :open="showTopupModal"
      title="Confirm Payment"
      @close="showTopupModal = false"
    >
      <div class="billing__modal-details">
        <dl class="billing__modal-dl">
          <div class="billing__modal-row">
            <dt>Amount</dt>
            <dd>{{ formatMoney(paymentForm.amount) }}</dd>
          </div>
          <div class="billing__modal-row">
            <dt>Payment Method</dt>
            <dd>{{ paymentForm.method || '—' }}</dd>
          </div>
          <div class="billing__modal-row">
            <dt>Receipt Reference</dt>
            <dd>{{ paymentForm.receipt || '—' }}</dd>
          </div>
        </dl>
      </div>
      <template #footer>
        <div class="billing__modal-actions">
          <KButton variant="ghost" @click="showTopupModal = false">Cancel</KButton>
          <KButton
            variant="primary"
            :loading="confirming"
            :disabled="confirming"
            @click="confirmTopup"
          >
            Confirm Payment
          </KButton>
        </div>
      </template>
    </KModal>
  </div>
</template>
<style scoped>
.billing__title {
  font-size: var(--text-2xl);
  font-weight: 700;
  margin-bottom: var(--space-6);
}
.billing__notice {
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-md);
  background: rgba(34, 197, 94, 0.1);
  color: var(--color-success);
  font-size: var(--text-sm);
  margin-bottom: var(--space-4);
  border: 1px solid rgba(34, 197, 94, 0.2);
}
.billing__balance {
  margin-bottom: var(--space-6);
}
.balance-card {
  padding: var(--space-5);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  display: inline-block;
}
.balance-card__label {
  font-size: var(--text-xs);
  color: var(--color-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: var(--space-2);
}
.balance-card__value {
  font-size: var(--text-2xl);
  font-weight: 700;
}
.billing__section {
  margin-bottom: var(--space-8);
  padding: var(--space-5);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
}
.billing__section-title {
  font-size: var(--text-md);
  font-weight: 600;
  margin-bottom: var(--space-4);
}
.billing__form {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  max-width: 400px;
}
.billing__warning {
  font-size: var(--text-xs);
  color: var(--color-warning);
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-sm);
  background: rgba(245, 158, 11, 0.1);
}
.billing__instructions {
  font-size: var(--text-xs);
  color: var(--color-muted);
  padding: var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  white-space: pre-wrap;
}
.billing__credit {
  color: var(--color-success);
  font-weight: 600;
}
.billing__debit {
  color: var(--color-error, #ef4444);
  font-weight: 600;
}
.billing__modal-details {
  font-size: var(--text-sm);
}
.billing__modal-dl {
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.billing__modal-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--space-2) 0;
  border-bottom: 1px solid var(--color-border);
}
.billing__modal-row:last-child {
  border-bottom: none;
}
.billing__modal-row dt {
  color: var(--color-muted);
  font-weight: 500;
}
.billing__modal-row dd {
  margin: 0;
  font-weight: 600;
  color: var(--color-text);
}
.billing__modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--space-3);
}
</style>
