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
  apiEndpoint: '/api/payment-methods',
  initialValues: {
    name: '',
    type: '',
    instructions: '',
    is_active: true,
    sort_order: 0,
    // Crypto-specific fields
    wallet_address: '',
    network: '',
    currency: '',
  },
  validate: (f) => {
    if (!f.name.trim()) return t('payments.validation_method_name')
    if (!f.type) return t('payments.validation_method_type')
    if (f.type === 'crypto') {
      if (!f.wallet_address.trim()) return t('payments.validation_wallet')
      if (!f.network) return t('payments.validation_network')
      if (!f.currency) return t('payments.validation_currency')
    }
    return null
  },
  onSuccess: () => {
    emit('close')
    paymentsStore.loadPayments()
  },
})

const methodTypeOptions = computed(() => [
  { label: t('payments.type_bank_transfer'), value: 'bank_transfer' },
  { label: t('payments.type_crypto'), value: 'crypto' },
  { label: t('payments.type_card'), value: 'card' },
  { label: t('payments.type_other'), value: 'other' },
])

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

const isCrypto = computed(() => form.value.type === 'crypto')

function handleClose() {
  emit('close')
}

async function handleSubmit() {
  // For crypto, serialize wallet info into the instructions field before submit
  if (form.value.type === 'crypto') {
    form.value.instructions = JSON.stringify({
      wallet_address: form.value.wallet_address,
      network: form.value.network,
      currency: form.value.currency,
      note: form.value.instructions,
    })
  }
  await submit()
}
</script>

<template>
  <KSlideOver :open="open" :title="t('payments.add_payment_method')" @close="handleClose">
    <form class="entity-form" @submit.prevent="handleSubmit">
      <KFormField name="pm-name" :label="t('payments.method_name')" required :error="validationError && !form.name ? validationError : ''">
        <template #default="{ fieldId }">
          <KInput :id="fieldId" v-model="form.name" :placeholder="t('payments.method_name_placeholder')" />
        </template>
      </KFormField>

      <KFormField name="pm-type" :label="t('payments.method_type')" required :error="validationError && !form.type ? validationError : ''">
        <template #default="{ fieldId }">
          <KSelect
            :id="fieldId"
            v-model="form.type"
            :options="methodTypeOptions"
            :placeholder="t('payments.select_type')"
          />
        </template>
      </KFormField>

      <!-- Crypto-specific fields -->
      <template v-if="isCrypto">
        <KFormField name="pm-wallet" :label="t('payments.crypto_wallet')" required>
          <template #default="{ fieldId }">
            <KInput :id="fieldId" v-model="form.wallet_address" :placeholder="t('payments.crypto_wallet_placeholder')" />
          </template>
        </KFormField>

        <KFormField name="pm-network" :label="t('payments.crypto_network')" required>
          <template #default="{ fieldId }">
            <KSelect
              :id="fieldId"
              v-model="form.network"
              :options="cryptoNetworkOptions"
              :placeholder="t('payments.crypto_select_network')"
            />
          </template>
        </KFormField>

        <KFormField name="pm-currency" :label="t('payments.crypto_currency')" required>
          <template #default="{ fieldId }">
            <KSelect
              :id="fieldId"
              v-model="form.currency"
              :options="cryptoCurrencyOptions"
              :placeholder="t('payments.crypto_select_currency')"
            />
          </template>
        </KFormField>
      </template>

      <!-- General instructions (non-crypto or notes for crypto) -->
      <KFormField name="pm-instructions" :label="isCrypto ? t('payments.crypto_note') : t('payments.method_instructions')">
        <template #default="{ fieldId }">
          <KInput :id="fieldId" v-model="form.instructions" :placeholder="t('payments.method_instructions_placeholder')" />
        </template>
      </KFormField>

      <div class="entity-form__actions">
        <KButton type="submit" variant="primary" :loading="submitting" full-width>
          {{ t('payments.create_method') }}
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
