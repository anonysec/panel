<script setup lang="ts">
import { ref, computed } from 'vue'
import { useNodesStore, type NodeFormData } from '@/stores/nodes'
import { useEntityForm } from '@/composables/useEntityForm'
import { useI18n } from '@koris/composables/useI18n'
import KSlideOver from '@koris/ui/KSlideOver.vue'
import KButton from '@koris/ui/KButton.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'
import KTextarea from '@koris/ui/KTextarea.vue'
import KAlert from '@koris/ui/KAlert.vue'

const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'created', nodeId: number): void
}>()

const { t } = useI18n()
const nodesStore = useNodesStore()

// ─── Form State (managed by useEntityForm) ──────────────────────────────────
function validate(f: typeof form.value): string | null {
  if (!f.address.trim()) return t('nodes.validation_address')
  const p = Number(f.port)
  if (!Number.isInteger(p) || p < 1 || p > 65535) return t('nodes.validation_port')
  if (!f.api_key.trim()) return t('nodes.validation_api_key')
  if (!f.ca_cert.trim()) return t('nodes.validation_cert')
  // PEM format validation
  const pem = f.ca_cert.trim()
  if (!pem.startsWith('-----BEGIN') || !pem.includes('-----END')) {
    return t('nodes.validation_pem_format') || 'CA certificate must be in PEM format'
  }
  // Name max 100 chars
  if (f.name.length > 100) return 'Name must be 100 characters or fewer'
  return null
}

const { form, submitting, validationError, submit, reset } = useEntityForm({
  apiEndpoint: '/api/admin/nodes',
  initialValues: {
    name: '',
    address: '',
    port: 2083,
    api_key: '',
    ca_cert: '',
  },
  validate,
  onSuccess() {
    emit('close')
    nodesStore.loadNodes()
  },
})

// ─── Inline error for API failures ──────────────────────────────────────────
const apiError = ref('')

async function handleSubmit() {
  apiError.value = ''

  // Trim all string fields before validation
  form.value.address = form.value.address.trim()
  form.value.api_key = form.value.api_key.trim()
  form.value.ca_cert = form.value.ca_cert.trim()
  form.value.name = form.value.name.trim()

  // Run validation
  const error = validate(form.value)
  if (error) {
    validationError.value = error
    return
  }
  validationError.value = ''

  // Build the actual NodeFormData payload for the store
  const payload: NodeFormData = {
    name: form.value.name || form.value.address,
    address: form.value.address,
    port: Number(form.value.port),
    api_key: form.value.api_key,
    client_cert_pem: '',
    client_key_pem: '',
    ca_cert_pem: form.value.ca_cert,
  }

  submitting.value = true
  const nodeId = await nodesStore.createNode(payload)
  submitting.value = false

  if (nodeId) {
    reset()
    emit('created', nodeId)
    emit('close')
    nodesStore.loadNodes()
  } else {
    apiError.value = t('nodes.created_error')
  }
}

function handleClose() {
  emit('close')
}
</script>

<template>
  <KSlideOver :open="open" :title="t('nodes.add_node')" @close="handleClose">
    <form class="slide-form" @submit.prevent="handleSubmit">
      <p class="slide-form__hint">
        {{ t('nodes.add_hint') }}
      </p>

      <KAlert v-if="apiError" variant="error" closable @close="apiError = ''">
        {{ apiError }}
      </KAlert>

      <KAlert v-if="validationError" variant="warning" closable @close="validationError = ''">
        {{ validationError }}
      </KAlert>

      <KFormField name="node-name" :label="t('nodes.node_name')" hint="Optional — defaults to address">
        <template #default="{ fieldId }">
          <KInput :id="fieldId" v-model="form.name" placeholder="e.g. de-1, us-west" />
        </template>
      </KFormField>

      <KFormField name="node-address" :label="t('nodes.address')" required>
        <template #default="{ fieldId }">
          <KInput :id="fieldId" v-model="form.address" placeholder="IP or hostname (e.g. 185.1.2.3)" />
        </template>
      </KFormField>

      <KFormField name="node-port" :label="t('label.port')" required>
        <template #default="{ fieldId }">
          <KInput :id="fieldId" v-model="form.port" type="number" placeholder="2083" />
        </template>
      </KFormField>

      <KFormField name="node-api-key" :label="t('nodes.api_key')" required hint="Shown when knode is installed">
        <template #default="{ fieldId }">
          <KInput :id="fieldId" v-model="form.api_key" type="password" autocomplete="off" placeholder="Paste from knode install output" />
        </template>
      </KFormField>

      <KFormField name="node-ca-cert" :label="t('nodes.certificate')" required hint="CA certificate from knode install output (PEM format)">
        <template #default="{ fieldId }">
          <KTextarea
            :id="fieldId"
            v-model="form.ca_cert"
            :rows="5"
            placeholder="-----BEGIN CERTIFICATE-----&#10;...&#10;-----END CERTIFICATE-----"
          />
        </template>
      </KFormField>

      <div class="slide-form__footer">
        <KButton type="button" variant="ghost" @click="handleClose">{{ t('btn.cancel') }}</KButton>
        <KButton type="submit" variant="primary" :loading="submitting">
          {{ t('nodes.test_and_save') }}
        </KButton>
      </div>
    </form>
  </KSlideOver>
</template>

<style scoped>
.slide-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  padding: var(--space-4) 0;
}

.slide-form__hint {
  margin: 0;
  font-size: var(--text-sm);
  color: var(--color-muted);
  line-height: 1.5;
}

.slide-form__footer {
  display: flex;
  justify-content: flex-end;
  gap: var(--space-2);
  padding-top: var(--space-3);
  border-top: 1px solid var(--color-border);
}
</style>
