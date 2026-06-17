<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { usePlansStore } from '@/stores/plans'
import { useI18n } from '@koris/composables/useI18n'
import KButton from '@koris/ui/KButton.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'

const { t } = useI18n()
const store = usePlansStore()
const showForm = ref(false)
const editingId = ref<number | null>(null)
const saving = ref(false)

const form = ref({
  name: '',
  billing_type: 'quota' as 'quota' | 'payg',
  data_gb: '',
  speed_mbps: '',
  duration_days: '',
  price: '',
  price_per_gb: '',
  price_per_day: '',
  disconnect_on_zero: true,
})

const isPayg = computed(() => form.value.billing_type === 'payg')

function resetForm() {
  form.value = {
    name: '',
    billing_type: 'quota',
    data_gb: '',
    speed_mbps: '',
    duration_days: '',
    price: '',
    price_per_gb: '',
    price_per_day: '',
    disconnect_on_zero: true,
  }
  editingId.value = null
  showForm.value = false
}

function openCreate() {
  resetForm()
  showForm.value = true
}

function openEdit(plan: any) {
  form.value = {
    name: plan.name,
    billing_type: plan.billing_type || 'quota',
    data_gb: String(plan.data_gb),
    speed_mbps: String(plan.speed_mbps),
    duration_days: String(plan.duration_days),
    price: String(plan.price),
    price_per_gb: String(plan.price_per_gb || 0),
    price_per_day: String(plan.price_per_day || 0),
    disconnect_on_zero: plan.disconnect_on_zero !== false,
  }
  editingId.value = plan.id
  showForm.value = true
}

async function handleSubmit() {
  saving.value = true
  const payload = {
    name: form.value.name,
    billing_type: form.value.billing_type,
    data_gb: Number(form.value.data_gb) || 0,
    speed_mbps: Number(form.value.speed_mbps) || 0,
    duration_days: Number(form.value.duration_days) || 0,
    price: Number(form.value.price) || 0,
    price_per_gb: Number(form.value.price_per_gb) || 0,
    price_per_day: Number(form.value.price_per_day) || 0,
    disconnect_on_zero: form.value.disconnect_on_zero,
    is_active: true,
    sort_order: 0,
  }
  if (editingId.value) {
    await store.updatePlan(editingId.value, payload)
  } else {
    await store.createPlan(payload)
  }
  saving.value = false
  resetForm()
}

async function deactivatePlan(id: number) {
  await store.deletePlan(id)
}

onMounted(() => {
  store.loadPlans()
})
</script>

<template>
  <div class="page plans-view">
    <!-- Header -->
    <header class="page-header">
      <KButton variant="primary" icon="+" @click="openCreate">{{ t('plans.create_plan') }}</KButton>
    </header>

    <!-- Create/Edit Form -->
    <div v-if="showForm" class="plan-form-panel">
      <h4 class="form-title">{{ editingId ? t('plans.edit_plan') : t('plans.new_plan') }}</h4>
      <form class="plan-form" @submit.prevent="handleSubmit">
        <!-- Billing Type Selector -->
        <div class="billing-type-selector">
          <label class="billing-type-option" :class="{ active: form.billing_type === 'quota' }">
            <input type="radio" v-model="form.billing_type" value="quota" />
            <span class="billing-type-label">{{ t('plans.type_quota') }}</span>
            <span class="billing-type-desc">{{ t('plans.type_quota_desc') }}</span>
          </label>
          <label class="billing-type-option" :class="{ active: form.billing_type === 'payg' }">
            <input type="radio" v-model="form.billing_type" value="payg" />
            <span class="billing-type-label">{{ t('plans.type_payg') }}</span>
            <span class="billing-type-desc">{{ t('plans.type_payg_desc') }}</span>
          </label>
        </div>

        <div class="form-grid">
          <KFormField name="plan-name" :label="t('plans.name')" required>
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="form.name" :placeholder="t('plans.name_placeholder')" />
            </template>
          </KFormField>
          <KFormField name="plan-speed" :label="t('plans.speed')">
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="form.speed_mbps" type="number" placeholder="100" />
            </template>
          </KFormField>

          <!-- Quota-specific fields -->
          <template v-if="!isPayg">
            <KFormField name="plan-data" :label="t('plans.data_gb')" required>
              <template #default="{ fieldId }">
                <KInput :id="fieldId" v-model="form.data_gb" type="number" placeholder="50" />
              </template>
            </KFormField>
            <KFormField name="plan-duration" :label="t('plans.duration_days')" required>
              <template #default="{ fieldId }">
                <KInput :id="fieldId" v-model="form.duration_days" type="number" placeholder="30" />
              </template>
            </KFormField>
            <KFormField name="plan-price" :label="t('plans.price')" required>
              <template #default="{ fieldId }">
                <KInput :id="fieldId" v-model="form.price" type="number" placeholder="9.99" />
              </template>
            </KFormField>
          </template>

          <!-- PAYG-specific fields -->
          <template v-if="isPayg">
            <KFormField name="plan-price-per-gb" :label="t('plans.price_per_gb')" required>
              <template #default="{ fieldId }">
                <KInput :id="fieldId" v-model="form.price_per_gb" type="number" step="0.01" placeholder="0.50" />
              </template>
            </KFormField>
            <KFormField name="plan-price-per-day" :label="t('plans.price_per_day')" required>
              <template #default="{ fieldId }">
                <KInput :id="fieldId" v-model="form.price_per_day" type="number" step="0.01" placeholder="0.10" />
              </template>
            </KFormField>
            <KFormField name="plan-disconnect" :label="t('plans.disconnect_on_zero')">
              <template #default>
                <label class="toggle-label">
                  <input type="checkbox" v-model="form.disconnect_on_zero" class="toggle-input" />
                  <span class="toggle-text">{{ form.disconnect_on_zero ? t('plans.yes') : t('plans.no') }}</span>
                </label>
              </template>
            </KFormField>
          </template>
        </div>
        <div class="form-actions">
          <KButton variant="ghost" @click="resetForm">{{ t('btn.cancel') }}</KButton>
          <KButton type="submit" variant="primary" :loading="saving">
            {{ editingId ? t('plans.update') : t('btn.create') }}
          </KButton>
        </div>
      </form>
    </div>

    <!-- Loading -->
    <div v-if="store.loading && store.list.length === 0" class="plans-grid">
      <KSkeleton v-for="i in 4" :key="i" variant="rect" :width="'100%'" :height="160" />
    </div>

    <!-- Empty -->
    <KEmptyState
      v-else-if="store.list.length === 0"
      icon="📋"
      :title="t('plans.no_plans')"
      :description="t('plans.no_plans_desc')"
    />

    <!-- Plans Grid -->
    <div v-else class="plans-grid">
      <div v-for="plan in store.list" :key="plan.id" class="plan-card" :class="{ 'plan-card--inactive': !plan.is_active }">
        <div class="plan-card__header">
          <h4 class="plan-card__name">{{ plan.name }}</h4>
          <div class="plan-card__badges">
            <span class="plan-card__type-badge" :class="plan.billing_type === 'payg' ? 'badge--payg' : 'badge--quota'">
              {{ plan.billing_type === 'payg' ? 'PAYG' : t('plans.type_quota') }}
            </span>
            <span v-if="!plan.is_active" class="plan-card__badge">{{ t('plans.inactive') }}</span>
          </div>
        </div>

        <!-- Quota pricing -->
        <div v-if="plan.billing_type !== 'payg'" class="plan-card__price">${{ plan.price }}</div>
        <!-- PAYG pricing -->
        <div v-else class="plan-card__price plan-card__price--payg">
          <span>${{ plan.price_per_gb }}/GB</span>
          <span class="price-separator">+</span>
          <span>${{ plan.price_per_day }}/{{ t('plans.day') }}</span>
        </div>

        <div class="plan-card__specs">
          <template v-if="plan.billing_type !== 'payg'">
            <div class="plan-spec">
              <span class="plan-spec__label">{{ t('plans.data') }}</span>
              <span class="plan-spec__value">{{ plan.data_gb }} GB</span>
            </div>
            <div class="plan-spec">
              <span class="plan-spec__label">{{ t('plans.duration') }}</span>
              <span class="plan-spec__value">{{ plan.duration_days }} {{ t('plans.days') }}</span>
            </div>
          </template>
          <template v-else>
            <div class="plan-spec">
              <span class="plan-spec__label">{{ t('plans.disconnect_on_zero') }}</span>
              <span class="plan-spec__value">{{ plan.disconnect_on_zero ? t('plans.yes') : t('plans.no') }}</span>
            </div>
          </template>
          <div class="plan-spec">
            <span class="plan-spec__label">{{ t('plans.speed_label') }}</span>
            <span class="plan-spec__value">{{ plan.speed_mbps }} Mbps</span>
          </div>
        </div>
        <div class="plan-card__actions">
          <KButton variant="ghost" size="sm" @click="openEdit(plan)">{{ t('btn.edit') }}</KButton>
          <KButton v-if="plan.is_active" variant="danger" size="sm" @click="deactivatePlan(plan.id)">{{ t('plans.deactivate') }}</KButton>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.plans-view { display: flex; flex-direction: column; gap: var(--space-5); }
.page-header { display: flex; align-items: center; justify-content: flex-end; }

.plan-form-panel { padding: var(--space-4); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); }
.form-title { margin: 0 0 var(--space-3); font-size: var(--text-base); font-weight: var(--font-semibold); }
.plan-form { display: flex; flex-direction: column; gap: var(--space-4); }
.form-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: var(--space-3); }
.form-actions { display: flex; justify-content: flex-end; gap: var(--space-2); }

/* Billing type selector */
.billing-type-selector { display: flex; gap: var(--space-3); }
.billing-type-option {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--space-1);
  padding: var(--space-3);
  border: 2px solid var(--color-border);
  border-radius: var(--radius-lg);
  cursor: pointer;
  transition: border-color var(--duration-fast), background var(--duration-fast);
}
.billing-type-option:hover { border-color: var(--color-primary); }
.billing-type-option.active { border-color: var(--color-primary); background: rgba(59, 130, 246, 0.05); }
.billing-type-option input[type="radio"] { display: none; }
.billing-type-label { font-weight: var(--font-semibold); font-size: var(--text-sm); }
.billing-type-desc { font-size: var(--text-xs); color: var(--color-muted); }

/* Toggle */
.toggle-label { display: flex; align-items: center; gap: var(--space-2); cursor: pointer; }
.toggle-input { width: 16px; height: 16px; accent-color: var(--color-primary); }
.toggle-text { font-size: var(--text-sm); }

.plans-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: var(--space-4); }

.plan-card { padding: var(--space-4); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); display: flex; flex-direction: column; gap: var(--space-3); transition: border-color var(--duration-fast); }
.plan-card:hover { border-color: var(--color-primary); }
.plan-card--inactive { opacity: 0.6; }

.plan-card__header { display: flex; justify-content: space-between; align-items: center; }
.plan-card__name { margin: 0; font-size: var(--text-base); font-weight: var(--font-semibold); }
.plan-card__badges { display: flex; gap: var(--space-1); align-items: center; }
.plan-card__badge { font-size: var(--text-xs); color: var(--color-warning); background: rgba(245, 158, 11, 0.1); padding: 2px 8px; border-radius: var(--radius-full); }
.plan-card__type-badge { font-size: var(--text-xs); padding: 2px 8px; border-radius: var(--radius-full); font-weight: var(--font-medium); }
.badge--quota { color: var(--color-primary); background: rgba(59, 130, 246, 0.1); }
.badge--payg { color: #10b981; background: rgba(16, 185, 129, 0.1); }

.plan-card__price { font-size: var(--text-2xl); font-weight: var(--font-bold); color: var(--color-primary); }
.plan-card__price--payg { font-size: var(--text-lg); display: flex; align-items: baseline; gap: var(--space-1); color: #10b981; }
.price-separator { font-size: var(--text-sm); color: var(--color-muted); font-weight: normal; }

.plan-card__specs { display: flex; flex-direction: column; gap: var(--space-1); }
.plan-spec { display: flex; justify-content: space-between; font-size: var(--text-sm); }
.plan-spec__label { color: var(--color-muted); }
.plan-spec__value { font-weight: var(--font-medium); }

.plan-card__actions { display: flex; gap: var(--space-2); border-top: 1px solid var(--color-border); padding-top: var(--space-3); }
</style>
