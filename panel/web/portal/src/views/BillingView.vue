<script setup lang="ts">
import { ref, computed } from 'vue'
import { usePortalAuthStore } from '@/stores/auth'
import { useBillingStore } from '@/stores/billing'
import { useFreshData } from '@/composables/useFreshData'
import KButton from '@koris/ui/KButton.vue'
import KDataTable from '@koris/ui/KDataTable.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'
import KSelect from '@koris/ui/KSelect.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'

const auth = usePortalAuthStore()
const billing = useBillingStore()

const paymentForm = ref({
  amount: 0,
  method: '',
  receipt: '',
})

const renewPlanId = ref<number>(0)
const notice = ref('')

useFreshData(async () => {
  await billing.loadBillingData()
  if (billing.paymentMethods.length && !paymentForm.value.method) {
    paymentForm.value.method = billing.paymentMethods[0].name
  }
  if (billing.activePlans.length && !renewPlanId.value) {
    renewPlanId.value = billing.activePlans[0].id
  }
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

function formatDate(value: string): string {
  if (!value) return 'N/A'
  return new Intl.DateTimeFormat('en', { year: 'numeric', month: 'short', day: '2-digit' }).format(new Date(value))
}

function formatGB(value: number): string {
  return value > 0 ? `${value} GB` : 'Unlimited'
}

async function handleSubmitPayment() {
  if (paymentForm.value.amount <= 0) return
  notice.value = ''
  const success = await billing.submitPayment({
    amount: paymentForm.value.amount,
    method: paymentForm.value.method,
    receipt: paymentForm.value.receipt,
  })
  if (success) {
    notice.value = 'Payment request submitted. Admin will review it.'
    paymentForm.value = { amount: 0, method: paymentForm.value.method, receipt: '' }
  }
}

async function handleRenew() {
  if (!renewPlanId.value) return
  notice.value = ''
  const res = await billing.renewPlan(renewPlanId.value)
  if (res?.renewed) {
    notice.value = 'Plan activated. Wallet was charged.'
  } else if (res?.payment_required) {
    notice.value = `Payment request #${res.payment_id} created for ${formatMoney(res.required_amount || 0)}.`
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
    </template>
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
</style>
