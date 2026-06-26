<script setup lang="ts">
import { computed } from 'vue'
import { useEntityForm } from '@/composables/useEntityForm'
import { useCustomersStore } from '@/stores/customers'
import { usePlansStore } from '@/stores/plans'
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
const customersStore = useCustomersStore()
const plansStore = usePlansStore()

const { form, submitting, validationError, submit, reset } = useEntityForm({
  apiEndpoint: '/api/customers',
  initialValues: {
    username: '',
    password: '',
    display_name: '',
    plan_id: '' as string | number,
    data_gb: '' as string | number,
    speed_mbps: '' as string | number,
    days: '' as string | number,
  },
  validate: (f) => {
    if (!f.username.trim()) return t('customers.validation_username')
    if (!f.password.trim()) return t('customers.validation_password')
    return null
  },
  onSuccess: () => {
    emit('close')
    customersStore.loadCustomers()
  },
})

const planOptions = computed(() =>
  plansStore.activePlans.map((p) => ({
    value: String(p.id),
    label: `${p.name} (${p.data_gb}GB / ${p.duration_days}d)`,
  }))
)

function handleClose() {
  emit('close')
}

async function handleSubmit() {
  // Convert numeric fields before submit
  const payload = { ...form.value }
  if (payload.plan_id) payload.plan_id = Number(payload.plan_id)
  if (payload.data_gb) payload.data_gb = Number(payload.data_gb)
  if (payload.speed_mbps) payload.speed_mbps = Number(payload.speed_mbps)
  if (payload.days) payload.days = Number(payload.days)
  form.value = payload
  await submit()
}
</script>

<template>
  <KSlideOver :open="open" :title="t('customers.new_user')" @close="handleClose">
    <form class="entity-form" @submit.prevent="handleSubmit">
      <KFormField name="user-username" :label="t('user.username')" required :error="validationError && !form.username ? validationError : ''">
        <template #default="{ fieldId }">
          <KInput :id="fieldId" v-model="form.username" autocomplete="off" :placeholder="t('user.username')" />
        </template>
      </KFormField>

      <KFormField name="user-password" :label="t('user.password')" required :error="validationError && !form.password ? validationError : ''">
        <template #default="{ fieldId }">
          <KInput :id="fieldId" v-model="form.password" type="password" autocomplete="new-password" :placeholder="t('user.password')" />
        </template>
      </KFormField>

      <KFormField name="user-display-name" :label="t('user.display_name')">
        <template #default="{ fieldId }">
          <KInput :id="fieldId" v-model="form.display_name" :placeholder="t('user.display_name')" />
        </template>
      </KFormField>

      <KFormField name="user-plan" :label="t('user.plan')">
        <template #default="{ fieldId }">
          <KSelect :id="fieldId" v-model="form.plan_id" :options="planOptions" :placeholder="t('plans.select_plan')" />
        </template>
      </KFormField>

      <KFormField name="user-data" :label="t('user.data_limit')">
        <template #default="{ fieldId }">
          <KInput :id="fieldId" v-model="form.data_gb" type="number" placeholder="GB" />
        </template>
      </KFormField>

      <KFormField name="user-speed" :label="t('user.speed_limit')">
        <template #default="{ fieldId }">
          <KInput :id="fieldId" v-model="form.speed_mbps" type="number" placeholder="Mbps" />
        </template>
      </KFormField>

      <KFormField name="user-duration" :label="t('user.duration')">
        <template #default="{ fieldId }">
          <KInput :id="fieldId" v-model="form.days" type="number" placeholder="Days" />
        </template>
      </KFormField>

      <div class="entity-form__actions">
        <KButton type="submit" variant="primary" :loading="submitting" full-width>
          {{ t('customers.create_user') }}
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
