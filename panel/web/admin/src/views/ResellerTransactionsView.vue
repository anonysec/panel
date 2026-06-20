<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from '@koris/composables/useI18n'
import { useApi } from '@koris/composables/useApi'
import { formatDate } from '@koris/composables/useFormatDate'

const { t } = useI18n()
const api = useApi()

interface Transaction {
  id: number
  reseller_username: string
  amount: number
  type: string
  description: string
  actor: string
  created_at: string
}

const transactions = ref<Transaction[]>([])
const loading = ref(true)

async function loadTransactions() {
  loading.value = true
  try {
    const data = await api.get<{ ok: boolean; transactions: Transaction[] }>('/api/reseller/transactions')
    if (data?.ok) {
      transactions.value = data.transactions
    }
  } finally {
    loading.value = false
  }
}

onMounted(loadTransactions)
</script>

<template>
  <div class="reseller-transactions">
    <h1 class="page-title">{{ t('nav.transactions') }}</h1>

    <div v-if="loading" class="loading-state">
      <div v-for="i in 5" :key="i" class="skeleton-row" />
    </div>

    <div v-else-if="transactions.length === 0" class="empty-state">
      <span class="empty-icon">💳</span>
      <p>{{ t('empty.no_payments') }}</p>
    </div>

    <div v-else class="table-wrap">
      <table class="tx-table">
        <thead>
          <tr>
            <th>{{ t('resellers.tx_date') }}</th>
            <th>{{ t('resellers.tx_amount') }}</th>
            <th>{{ t('resellers.tx_description') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="tx in transactions" :key="tx.id">
            <td class="date-cell">{{ formatDate(tx.created_at) }}</td>
            <td :class="['amount-cell', tx.amount >= 0 ? 'positive' : 'negative']">
              {{ tx.amount >= 0 ? '+' : '' }}{{ tx.amount.toLocaleString() }}
            </td>
            <td class="desc-cell">{{ tx.description }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.reseller-transactions {
  padding: var(--space-6, 24px);
}

.page-title {
  font-size: var(--text-2xl, 22px);
  font-weight: var(--font-bold, 700);
  margin: 0 0 var(--space-5, 20px);
}

.loading-state {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.skeleton-row {
  height: 44px;
  background: var(--color-surface-2, #1e2630);
  border-radius: var(--radius-md, 8px);
  animation: pulse 1.5s infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

.empty-state {
  text-align: center;
  padding: 60px 20px;
  color: var(--color-muted, #8b98a5);
}

.empty-icon {
  font-size: 40px;
  display: block;
  margin-bottom: 12px;
}

.table-wrap {
  overflow-x: auto;
  border: 1px solid var(--color-border, #28333f);
  border-radius: var(--radius-lg, 12px);
  background: var(--color-surface-2, #1e2630);
}

.tx-table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--text-sm, 13px);
}

.tx-table th {
  text-align: left;
  padding: 12px 14px;
  font-weight: var(--font-semibold, 600);
  color: var(--color-muted, #8b98a5);
  border-bottom: 1px solid var(--color-border, #28333f);
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.tx-table td {
  padding: 12px 14px;
  border-bottom: 1px solid var(--color-border, #28333f);
  color: var(--color-text, #e6edf3);
}

.tx-table tr:last-child td {
  border-bottom: none;
}

.date-cell {
  color: var(--color-muted, #8b98a5);
  white-space: nowrap;
}

.amount-cell {
  font-weight: var(--font-bold, 700);
  font-variant-numeric: tabular-nums;
}

.amount-cell.positive {
  color: #22c55e;
}

.amount-cell.negative {
  color: #ef4444;
}

.desc-cell {
  max-width: 300px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
