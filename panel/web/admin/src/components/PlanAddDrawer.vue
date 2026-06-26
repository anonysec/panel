<script setup lang="ts">
import { computed } from 'vue'
import { useEntityForm } from '@/composables/useEntityForm'
import { usePlansStore } from '@/stores/plans'
import { useI18n } from '@koris/composables/useI18n'
import KSlideOver from '@koris/ui/KSlideOver.vue'
import KButton from '@koris/ui/KButton.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'

defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const { t } = useI18n()
const plansStore = usePlansStore()

const { form, submitting, validationError, submit, reset } = useEntityForm({
  apiEndpoint: '/api/plans',
  initialValues: {
    name: '',
    billing_type: 'quota' as 'quota' | 'payg',
    data_gb: '' as string | number,
    speed_mbps: '' as string | number,
    duration_days: '' as string | number,
    price: '' as string | number,
    price_per_gb: '' as string | number,
    price_per_day: '' as string | number,
    disconnect_on_zero: true,
    is_active: true,
    sort_order: 0,
  },
  validate: (f) => {
    if (!f.name.trim()) return t('plans.validation_name')
    if (!f.billing_type) return t('plans.validation_billing_type')
    return null
  },
  onSuccess: () => {
    emit('close')
    plansStore.loadPlans()
  },
})

const isPayg = computed(() => form.value.billing_type === 'payg')

function handleClose() {
  emit('close')
}

function setBillingType(type: 'quota' | 'payg') {
  form.value.billing_type = type
}

async function handleSubmit() {
  // Convert numeric fields before submit
  const payload = { ...form.value }
  payload.data_gb = Number(payload.data_gb) || 0
  payload.speed_mbps = Number(payload.speed_mbps) || 0
  payload.duration_days = Number(payload.duration_days) || 0
  payload.price = Number(payload.price) || 0
  payload.price_per_gb = Number(payload.price_per_gb) || 0
  payload.price_per_day = Number(payload.price_per_day) || 0
  form.value = payload
  await submit()
}
</script>

<template>
  <KSlideOver :open="open" :title="t('plans.create_plan')" @close="handleClose">
    <form class="entity-form" @submit.prevent="handleSubmit">
      <KFormField name="plan-name" :label="t('plans.name')" required :error="validationError && !form.name ? validationError : ''">
        <template #default="{ fieldId }">
          <KInput :id="fieldId" v-model="form.name" :placeholder="t('plans.name_placeholder')" />
        </template>
      </KFormField>

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

      <!-- Shared fields -->
      <KFormField name="plan-speed" :label="t('plans.speed')">
        <template #default="{ fieldId }">
          <KInput :id="fieldId" v-model="form.speed_mbps" type="number" placeholder="100" />
        </template>
      </KFormField>

      <!-- Quota-specific fields -->
      <template v-if="!isPayg">
        <KFormField name="plan-data" :label="t('plans.data_limit')">
          <template #default="{ fieldId }">
            <KInput :id="fieldId" v-model="form.data_gb" type="number" placeholder="GB" />
          </template>
        </KFormField>

        <KFormField name="plan-duration" :label="t('plans.duration')">
          <template #default="{ fieldId }">
            <KInput :id="fieldId" v-model="form.duration_days" type="number" placeholder="Days" />
          </template>
        </KFormField>

        <KFormField name="plan-price" :label="t('plans.price')">
          <template #default="{ fieldId }">
            <KInput :id="fieldId" v-model="form.price" type="number" placeholder="$" />
          </template>
        </KFormField>
      </template>

      <!-- PAYG-specific fields -->
      <template v-if="isPayg">
        <KFormField name="plan-price-gb" :label="t('plans.price_per_gb')">
          <template #default="{ fieldId }">
            <KInput :id="fieldId" v-model="form.price_per_gb" type="number" placeholder="$/GB" />
          </template>
        </KFormField>

        <KFormField name="plan-price-day" :label="t('plans.price_per_day')">
          <template #default="{ fieldId }">
            <KInput :id="fieldId" v-model="form.price_per_day" type="number" placeholder="$/day" />
          </template>
        </KFormField>
      </template>

      <!-- Disconnect on zero toggle -->
      <label class="toggle-field">
        <input type="checkbox" v-model="form.disconnect_on_zero" />
        <span>{{ t('plans.disconnect_on_zero') }}</span>
      </label>

      <div class="entity-form__actions">
        <KButton type="submit" variant="primary" :loading="submitting" full-width>
          {{ t('plans.create_plan') }}
        </KButton>
      </div>
    </form>
  </KSlideOver>
</template>

<style scoped>
.entity-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-3, 0.75rem);
  padding: var(--space-4, 1rem);
}

.entity-form__actions {
  display: flex;
  gap: var(--space-2, 0.5rem);
  padding: var(--space-4, 1rem);
}

.billing-type-selector {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-2, 0.5rem);
}

.billing-type-option {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--space-1, 0.25rem);
  padding: var(--space-3, 0.75rem);
  border: 1px solid var(--border, var(--koris-border, #333));
  border-radius: var(--radius, var(--koris-borderRadius, 8px));
  cursor: pointer;
  transition: border-color 0.15s, background 0.15s;
  text-align: center;
}

.billing-type-option.active {
  border-color: var(--brand, var(--koris-primary, #6366f1));
  background: color-mix(in srgb, var(--brand, var(--koris-primary, #6366f1)) 8%, transparent);
}

.billing-type-option input[type="radio"] {
  display: none;
}

.billing-type-label {
  font-weight: 600;
  font-size: var(--text-sm, 0.875rem);
  color: var(--text, var(--koris-text, #fff));
}

.billing-type-desc {
  font-size: var(--text-xs, 0.75rem);
  color: var(--muted, var(--koris-textMuted, #888));
}

.toggle-field {
  display: flex;
  align-items: center;
  gap: var(--space-2, 0.5rem);
  font-size: var(--text-sm, 0.875rem);
  color: var(--text, var(--koris-text, #fff));
  cursor: pointer;
}

.toggle-field input[type="checkbox"] {
  width: 16px;
  height: 16px;
}
</style>
