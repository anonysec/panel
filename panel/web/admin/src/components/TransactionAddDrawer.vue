<script setup lang="ts">
import { computed } from 'vue'
import { useEntityForm } from '@/composables/useEntityForm'
import { usePaymentsStore } from '@/stores/payments'
import { useI18n } from '@koris/composables/useI18n'
import KSlideOver from '@koris/ui/KSlideOver.vue'
import KButton from '@koris/ui/KButton.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'
import KSelect from '@koris/ui/KSelect.vue'

defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const { t } = useI18n()
const paymentsStore = usePaymentsStore()

const { form, submitting, validationError, submit, reset } = useEntityForm({
  apiEndpoint: '/api/payments',
  initialValues: {
    username: '',
    amount: '' as string | number,
    method: '',
    description: '',
  },
  validate: (f) => {
    if (!f.username.trim()) return t('payments.validation_username')
    if (!f.amount || Number(f.amount) <= 0) return t('payments.validation_amount')
    if (!f.method) return t('payments.validation_method')
    return null
  },
  onSuccess: () => {
    emit('close')
    paymentsStore.loadPayments()
  },
})

const methodOptions = computed(() =>
  paymentsStore.activePaymentMethods.map((m) => ({
    value: m.name,
    label: m.name,
  }))
)

function handleClose() {
  emit('close')
}

async function handleSubmit() {
  const payload = { ...form.value }
  if (payload.amount) payload.amount = Number(payload.amount)
  form.value = payload
  await submit()
}
</script>

<template>
  <KSlideOver :open="open" :title="t('payments.record_payment')" @close="handleClose">
    <form class="entity-form" @submit.prevent="handleSubmit">
      <KFormField name="txn-username" :label="t('payments.form_username')" required :error="validationError && !form.username ? validationError : ''">
        <template #default="{ fieldId }">
          <KInput :id="fieldId" v-model="form.username" placeholder="customer_username" />
        </template>
      </KFormField>

      <KFormField name="txn-amount" :label="t('payments.form_amount')" required :error="validationError && (!form.amount || Number(form.amount) <= 0) ? validationError : ''">
        <template #default="{ fieldId }">
          <KInput :id="fieldId" v-model="form.amount" type="number" placeholder="10.00" />
        </template>
      </KFormField>

      <KFormField name="txn-method" :label="t('payments.form_method')" required :error="validationError && !form.method ? validationError : ''">
        <template #default="{ fieldId }">
          <KSelect
            :id="fieldId"
            v-model="form.method"
            :options="methodOptions"
            :placeholder="t('payments.select_method')"
          />
        </template>
      </KFormField>

      <KFormField name="txn-description" :label="t('payments.form_description')">
        <template #default="{ fieldId }">
          <KInput :id="fieldId" v-model="form.description" :placeholder="t('payments.optional_note')" />
        </template>
      </KFormField>

      <div class="entity-form__actions">
        <KButton type="submit" variant="primary" :loading="submitting" full-width>
          {{ t('payments.record_payment') }}
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
</style>
