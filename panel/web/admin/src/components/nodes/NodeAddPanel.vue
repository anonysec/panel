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

// ─── Paste All-in-One ───────────────────────────────────────────────────────
const showPasteMode = ref(false)
const pasteCode = ref('')
const pasteError = ref('')

/**
 * Parses the knode install output to extract address, port, api_key, and certificate.
 * Supports formats like:
 *   Address:  185.1.2.3
 *   Port:     2083
 *   API Key:  kn_abc123...
 *   Certificate:
 *   -----BEGIN CERTIFICATE-----
 *   ...
 *   -----END CERTIFICATE-----
 *
 * Also supports compact one-line format: ip:port:apikey:cert_base64
 */
function parsePasteCode(text: string): { address: string; port: number; api_key: string; ca_cert: string } | null {
  const lines = text.trim().split('\n')

  // Try key:value format (knode install output)
  let address = ''
  let port = 2083
  let apiKey = ''
  let certLines: string[] = []
  let inCert = false

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i].trim()

    if (inCert) {
      certLines.push(lines[i].trimEnd())
      if (line.includes('-----END')) {
        inCert = false
      }
      continue
    }

    // Match "Address:" or "IP:" patterns
    const addrMatch = line.match(/^(?:address|ip|host|server)\s*[:=]\s*(.+)$/i)
    if (addrMatch) {
      address = addrMatch[1].trim()
      continue
    }

    // Match "Port:" pattern
    const portMatch = line.match(/^port\s*[:=]\s*(\d+)/i)
    if (portMatch) {
      port = parseInt(portMatch[1], 10)
      continue
    }

    // Match "API Key:" or "Token:" or "api_key:" patterns
    const keyMatch = line.match(/^(?:api[_\s]?key|token|key)\s*[:=]\s*(.+)$/i)
    if (keyMatch) {
      apiKey = keyMatch[1].trim()
      continue
    }

    // Match "Certificate:" or "Cert:" header
    const certHeaderMatch = line.match(/^(?:certificate|cert|ca[_\s]?cert)\s*[:=]?\s*$/i)
    if (certHeaderMatch) {
      inCert = true
      continue
    }

    // Detect PEM start directly
    if (line.startsWith('-----BEGIN')) {
      inCert = true
      certLines.push(lines[i].trimEnd())
      continue
    }
  }

  const caCert = certLines.join('\n').trim()

  if (address && apiKey) {
    return { address, port, api_key: apiKey, ca_cert: caCert }
  }

  return null
}

function applyPasteCode() {
  pasteError.value = ''
  const parsed = parsePasteCode(pasteCode.value)
  if (!parsed) {
    pasteError.value = 'Could not parse the pasted text. Expected format:\nAddress: <ip>\nPort: <port>\nAPI Key: <key>\nCertificate:\n-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----'
    return
  }

  form.value.address = parsed.address
  form.value.port = parsed.port
  form.value.api_key = parsed.api_key
  if (parsed.ca_cert) {
    form.value.ca_cert = parsed.ca_cert
  }

  showPasteMode.value = false
  pasteCode.value = ''
}

// ─── Form State (managed by useEntityForm) ──────────────────────────────────
interface NodeFormValues {
  name: string
  address: string
  port: number
  api_key: string
  ca_cert: string
}

function validate(f: NodeFormValues): string | null {
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

      <!-- Paste All-in-One Section -->
      <div class="paste-section">
        <KButton
          type="button"
          variant="ghost"
          size="sm"
          @click="showPasteMode = !showPasteMode"
        >
          {{ showPasteMode ? 'Manual Entry' : '📋 Paste Install Output' }}
        </KButton>

        <div v-if="showPasteMode" class="paste-area">
          <p class="paste-hint">
            Paste the output from knode installation. The fields will be auto-filled.
          </p>
          <KTextarea
            v-model="pasteCode"
            :rows="10"
            placeholder="Address:  185.1.2.3&#10;Port:     2083&#10;&#10;API Key:&#10;kn_abc123...&#10;&#10;Certificate:&#10;-----BEGIN CERTIFICATE-----&#10;...&#10;-----END CERTIFICATE-----"
          />
          <KAlert v-if="pasteError" variant="error" closable @close="pasteError = ''">
            {{ pasteError }}
          </KAlert>
          <KButton type="button" variant="primary" size="sm" @click="applyPasteCode">
            Apply
          </KButton>
        </div>
      </div>

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

/* Paste All-in-One */
.paste-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-3);
  background: var(--color-surface-2, rgba(0,0,0,0.05));
  border-radius: var(--radius-md);
  border: 1px dashed var(--color-border);
}

.paste-area {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.paste-hint {
  margin: 0;
  font-size: var(--text-xs);
  color: var(--color-muted);
}
</style>
