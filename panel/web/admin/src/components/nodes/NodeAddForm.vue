<script setup lang="ts">
import { ref, computed } from 'vue'
import { useNodesStore, type NodeFormData } from '@/stores/nodes'
import { useToast } from '@koris/composables/useToast'
import KButton from '@koris/ui/KButton.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'
import KTextarea from '@koris/ui/KTextarea.vue'
import KAlert from '@koris/ui/KAlert.vue'

const emit = defineEmits<{
  (e: 'created', nodeId: number): void
}>()

const nodesStore = useNodesStore()
const toast = useToast()

// ─── Form State ─────────────────────────────────────────────────────────────
const name = ref('')
const address = ref('')
const port = ref(62050)
const apiKey = ref('')
const clientCertPem = ref('')
const clientKeyPem = ref('')
const caCertPem = ref('')

const saving = ref(false)
const feedback = ref<{ type: 'success' | 'error'; message: string } | null>(null)

// ─── Validation ─────────────────────────────────────────────────────────────
const errors = computed(() => {
  const e: Record<string, string> = {}
  if (!address.value.trim()) e.address = 'Address is required'
  const p = Number(port.value)
  if (!Number.isInteger(p) || p < 1 || p > 65535) e.port = 'Port must be 1–65535'
  if (!clientCertPem.value.trim()) e.clientCertPem = 'Client certificate is required'
  if (!clientKeyPem.value.trim()) e.clientKeyPem = 'Client key is required'
  if (!caCertPem.value.trim()) e.caCertPem = 'CA certificate is required'
  return e
})

const isValid = computed(() => Object.keys(errors.value).length === 0)

// ─── Actions ────────────────────────────────────────────────────────────────
async function handleSubmit() {
  if (!isValid.value) return

  saving.value = true
  feedback.value = null

  const payload: NodeFormData = {
    name: name.value.trim(),
    address: address.value.trim(),
    port: Number(port.value),
    api_key: apiKey.value,
    client_cert_pem: clientCertPem.value,
    client_key_pem: clientKeyPem.value,
    ca_cert_pem: caCertPem.value,
  }

  const nodeId = await nodesStore.createNode(payload)

  if (nodeId) {
    feedback.value = { type: 'success', message: 'Node created successfully' }
    toast.success('Node created')
    emit('created', nodeId)
    resetForm()
  } else {
    feedback.value = { type: 'error', message: 'Failed to create node. Check connection details.' }
  }

  saving.value = false
}

function resetForm() {
  name.value = ''
  address.value = ''
  port.value = 62050
  apiKey.value = ''
  clientCertPem.value = ''
  clientKeyPem.value = ''
  caCertPem.value = ''
}
</script>

<template>
  <form class="node-add-form" @submit.prevent="handleSubmit">
    <h3 class="node-add-form__title">Add Node</h3>

    <KAlert v-if="feedback" :variant="feedback.type" closable @close="feedback = null">
      {{ feedback.message }}
    </KAlert>

    <KFormField name="node-name" label="Name" hint="Friendly name for this node">
      <template #default="{ fieldId }">
        <KInput :id="fieldId" v-model="name" placeholder="e.g. de-1" />
      </template>
    </KFormField>

    <KFormField name="node-address" label="Address" :error="errors.address">
      <template #default="{ fieldId }">
        <KInput :id="fieldId" v-model="address" placeholder="e.g. 192.168.1.100 or node.example.com" />
      </template>
    </KFormField>

    <KFormField name="node-port" label="Port" :error="errors.port">
      <template #default="{ fieldId }">
        <KInput :id="fieldId" v-model="port" type="number" placeholder="62050" />
      </template>
    </KFormField>

    <KFormField name="node-api-key" label="API Key">
      <template #default="{ fieldId }">
        <KInput :id="fieldId" v-model="apiKey" type="password" placeholder="API key for node authentication" />
      </template>
    </KFormField>

    <KFormField name="node-client-cert" label="Client Certificate (PEM)" :error="errors.clientCertPem">
      <template #default="{ fieldId }">
        <KTextarea :id="fieldId" v-model="clientCertPem" :rows="4" placeholder="-----BEGIN CERTIFICATE-----" />
      </template>
    </KFormField>

    <KFormField name="node-client-key" label="Client Key (PEM)" :error="errors.clientKeyPem">
      <template #default="{ fieldId }">
        <KTextarea :id="fieldId" v-model="clientKeyPem" :rows="4" placeholder="-----BEGIN PRIVATE KEY-----" />
      </template>
    </KFormField>

    <KFormField name="node-ca-cert" label="CA Certificate (PEM)" :error="errors.caCertPem">
      <template #default="{ fieldId }">
        <KTextarea :id="fieldId" v-model="caCertPem" :rows="4" placeholder="-----BEGIN CERTIFICATE-----" />
      </template>
    </KFormField>

    <KButton type="submit" variant="primary" :loading="saving" :disabled="!isValid">
      Test &amp; Save
    </KButton>
  </form>
</template>

<style scoped>
.node-add-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  max-width: 520px;
}

.node-add-form__title {
  margin: 0;
  font-size: var(--text-lg);
  font-weight: var(--font-semibold);
  color: var(--color-text);
}
</style>
