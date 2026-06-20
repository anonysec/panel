<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from '@koris/composables/useI18n'
import { useApi } from '@koris/composables/useApi'
import { useToast } from '@koris/composables/useToast'

const { t } = useI18n()
const api = useApi()
const toast = useToast()

interface PlanPrice {
  id: number
  name: string
  data_gb: number
  duration_days: number
  wholesale_price: number
  sell_price: number
  editPrice?: number
  saving?: boolean
}

const plans = ref<PlanPrice[]>([])
const loading = ref(true)

async function loadPlans() {
  loading.value = true
  try {
    const data = await api.get<{ ok: boolean; plans: PlanPrice[] }>('/api/reseller/plan-prices')
    if (data?.ok) {
      plans.value = data.plans.map(p => ({ ...p, editPrice: p.sell_price, saving: false }))
    }
  } finally {
    loading.value = false
  }
}

async function saveSellPrice(plan: PlanPrice) {
  plan.saving = true
  try {
    const data = await api.post<{ ok: boolean }>('/api/reseller/plan-prices', {
      plan_id: plan.id,
      sell_price: plan.editPrice ?? 0,
    })
    if (data?.ok) {
      plan.sell_price = plan.editPrice ?? 0
      toast.success(t('reseller_plans.saved'))
    }
  } finally {
    plan.saving = false
  }
}

function profit(plan: PlanPrice): number {
  return (plan.editPrice ?? 0) - plan.wholesale_price
}

onMounted(loadPlans)
</script>

<template>
  <div class="reseller-plans">
    <h1 class="page-title">{{ t('reseller_plans.title') }}</h1>

    <div v-if="loading" class="loading-state">
      <div v-for="i in 3" :key="i" class="skeleton-row" />
    </div>

    <div v-else class="plans-table-wrap">
      <table class="plans-table">
        <thead>
          <tr>
            <th>{{ t('plans.name') }}</th>
            <th>{{ t('plans.data_gb') }}</th>
            <th>{{ t('plans.duration_days') }}</th>
            <th>{{ t('reseller_plans.wholesale') }}</th>
            <th>{{ t('reseller_plans.sell_price') }}</th>
            <th>{{ t('reseller_plans.profit') }}</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="plan in plans" :key="plan.id">
            <td class="plan-name">{{ plan.name }}</td>
            <td>{{ plan.data_gb > 0 ? `${plan.data_gb} GB` : '∞' }}</td>
            <td>{{ plan.duration_days > 0 ? `${plan.duration_days}d` : '∞' }}</td>
            <td class="price-cell">{{ plan.wholesale_price.toLocaleString() }}</td>
            <td>
              <input
                v-model.number="plan.editPrice"
                type="number"
                min="0"
                step="1000"
                class="price-input"
              />
            </td>
            <td :class="['profit-cell', { positive: profit(plan) > 0, negative: profit(plan) < 0 }]">
              {{ profit(plan).toLocaleString() }}
            </td>
            <td>
              <button
                class="save-btn"
                :disabled="plan.saving"
                @click="saveSellPrice(plan)"
              >
                {{ plan.saving ? '...' : t('reseller_plans.save') }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.reseller-plans {
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
  height: 48px;
  background: var(--color-surface-2, #1e2630);
  border-radius: var(--radius-md, 8px);
  animation: pulse 1.5s infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

.plans-table-wrap {
  overflow-x: auto;
  border: 1px solid var(--color-border, #28333f);
  border-radius: var(--radius-lg, 12px);
  background: var(--color-surface-2, #1e2630);
}

.plans-table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--text-sm, 13px);
}

.plans-table th {
  text-align: left;
  padding: 12px 14px;
  font-weight: var(--font-semibold, 600);
  color: var(--color-muted, #8b98a5);
  border-bottom: 1px solid var(--color-border, #28333f);
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.plans-table td {
  padding: 12px 14px;
  border-bottom: 1px solid var(--color-border, #28333f);
  color: var(--color-text, #e6edf3);
}

.plans-table tr:last-child td {
  border-bottom: none;
}

.plan-name {
  font-weight: var(--font-semibold, 600);
}

.price-cell {
  color: var(--color-muted, #8b98a5);
}

.price-input {
  width: 100px;
  padding: 6px 10px;
  border: 1px solid var(--color-border, #28333f);
  border-radius: var(--radius-sm, 6px);
  background: var(--color-surface, #0b1120);
  color: var(--color-text, #e6edf3);
  font-size: 13px;
}

.price-input:focus {
  outline: none;
  border-color: var(--color-primary, #2563eb);
}

.profit-cell {
  font-weight: var(--font-semibold, 600);
}

.profit-cell.positive {
  color: #22c55e;
}

.profit-cell.negative {
  color: #ef4444;
}

.save-btn {
  padding: 6px 14px;
  border-radius: var(--radius-sm, 6px);
  background: var(--color-primary, #2563eb);
  color: #fff;
  border: none;
  font-size: 12px;
  font-weight: var(--font-semibold, 600);
  cursor: pointer;
  transition: opacity 0.15s;
}

.save-btn:hover {
  opacity: 0.85;
}

.save-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
