<template>
  <div class="transaction-list">
    <h3 v-if="showTitle" class="transaction-list__title">Transactions</h3>

    <div v-if="sortedTransactions.length === 0" class="transaction-list__empty">
      <p class="transaction-list__empty-text">No transactions yet</p>
    </div>

    <ul v-else class="transaction-list__items">
      <li
        v-for="tx in sortedTransactions"
        :key="tx.id"
        class="transaction-list__item"
      >
        <div class="transaction-list__item-left">
          <span
            class="transaction-list__amount"
            :class="amountClass(tx.amount)"
          >
            {{ formatSignedAmount(tx.amount) }}
          </span>
          <span class="transaction-list__type">{{ tx.type }}</span>
        </div>
        <div class="transaction-list__item-right">
          <span
            class="transaction-list__description"
            :title="tx.description && tx.description.length > 100 ? tx.description : undefined"
          >
            {{ truncateDescription(tx.description) }}
          </span>
          <span class="transaction-list__timestamp">{{ formatTimestamp(tx.created_at) }}</span>
        </div>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { WalletTransaction } from '@koris/types/entities'
import { formatCurrency } from '@/utils/formatCurrency'

export interface TransactionListProps {
  transactions: WalletTransaction[]
  currencySymbol?: string
  showTitle?: boolean
}

const props = withDefaults(defineProps<TransactionListProps>(), {
  currencySymbol: '$',
  showTitle: true,
})

const sortedTransactions = computed(() =>
  [...props.transactions].sort(
    (a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
  )
)

function formatSignedAmount(amount: number): string {
  const sign = amount >= 0 ? '+' : ''
  return `${sign}${formatCurrency(amount, props.currencySymbol)}`
}

function amountClass(amount: number): string {
  if (amount > 0) return 'transaction-list__amount--positive'
  if (amount < 0) return 'transaction-list__amount--negative'
  return ''
}

function truncateDescription(description: string): string {
  if (!description) return ''
  if (description.length <= 100) return description
  return description.slice(0, 100) + '…'
}

function formatTimestamp(isoString: string): string {
  const date = new Date(isoString)
  if (isNaN(date.getTime())) return isoString
  return date.toLocaleString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}
</script>

<style scoped>
.transaction-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-3, 12px);
  padding: var(--space-4, 16px);
}

.transaction-list__title {
  margin: 0;
  font-size: var(--text-sm, 0.875rem);
  font-weight: var(--font-semibold, 600);
  color: var(--color-text-secondary, #6b7280);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.transaction-list__empty {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-6, 24px) var(--space-4, 16px);
}

.transaction-list__empty-text {
  margin: 0;
  font-size: var(--text-sm, 0.875rem);
  color: var(--color-text-muted, #9ca3af);
}

.transaction-list__items {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-2, 8px);
}

.transaction-list__item {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-3, 12px);
  padding: var(--space-2, 8px) 0;
  border-bottom: 1px solid var(--color-border-light, #f3f4f6);
}

.transaction-list__item:last-child {
  border-bottom: none;
}

.transaction-list__item-left {
  display: flex;
  flex-direction: column;
  gap: var(--space-1, 4px);
  min-width: 0;
}

.transaction-list__item-right {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: var(--space-1, 4px);
  min-width: 0;
  flex-shrink: 1;
}

.transaction-list__amount {
  font-size: var(--text-sm, 0.875rem);
  font-weight: var(--font-semibold, 600);
  font-family: var(--font-mono, monospace);
  white-space: nowrap;
}

.transaction-list__amount--positive {
  color: var(--color-success, #10b981);
}

.transaction-list__amount--negative {
  color: var(--color-error, #ef4444);
}

.transaction-list__type {
  font-size: var(--text-xs, 0.75rem);
  color: var(--color-text-muted, #9ca3af);
  text-transform: capitalize;
}

.transaction-list__description {
  font-size: var(--text-xs, 0.75rem);
  color: var(--color-text-secondary, #6b7280);
  text-align: right;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 200px;
}

.transaction-list__timestamp {
  font-size: var(--text-xs, 0.75rem);
  color: var(--color-text-muted, #9ca3af);
  white-space: nowrap;
}
</style>
