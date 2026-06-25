<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useNodesStore, type CertInfo } from '@/stores/nodes'
import { useToast } from '@koris/composables/useToast'
import KButton from '@koris/ui/KButton.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KTextarea from '@koris/ui/KTextarea.vue'
import KSelect from '@koris/ui/KSelect.vue'
import KAlert from '@koris/ui/KAlert.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'

const props = defineProps<{
  nodeId: number
}>()

const nodesStore = useNodesStore()
const toast = useToast()

const certs = ref<CertInfo[]>([])
const loading = ref(false)

// ─── Upload Form ────────────────────────────────────────────────────────────
const uploadCoreType = ref('')
const caPem = ref('')
const certPem = ref('')
const keyPem = ref('')
const submitting = ref(false)

const coreTypeOptions = [
  { label: 'OpenVPN', value: 'openvpn' },
  { label: 'WireGuard', value: 'wireguard' },
  { label: 'L2TP', value: 'l2tp' },
  { label: 'IKEv2', value: 'ikev2' },
  { label: 'SSH', value: 'ssh' },
]

const uploadValid = computed(() => {
  return (
    uploadCoreType.value.length > 0 &&
    caPem.value.trim().length > 0 &&
    certPem.value.trim().length > 0 &&
    keyPem.value.trim().length > 0
  )
})

// ─── Helpers ────────────────────────────────────────────────────────────────

function expiryClass(days: number): string {
  if (days < 7) return 'node-certs-tab__expiry--critical'
  if (days < 30) return 'node-certs-tab__expiry--warning'
  return 'node-certs-tab__expiry--ok'
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString()
}

// ─── Actions ────────────────────────────────────────────────────────────────

async function loadCerts() {
  loading.value = true
  certs.value = await nodesStore.getCertInfo(props.nodeId)
  loading.value = false
}

async function handleUpload() {
  if (!uploadValid.value) return

  submitting.value = true
  const ok = await nodesStore.pushCerts(
    props.nodeId,
    uploadCoreType.value,
    caPem.value,
    certPem.value,
    keyPem.value
  )

  if (ok) {
    toast.success('Certificates uploaded')
    caPem.value = ''
    certPem.value = ''
    keyPem.value = ''
    await loadCerts()
  } else {
    toast.error('Failed to upload certificates')
  }
  submitting.value = false
}

onMounted(loadCerts)
</script>

<template>
  <div class="node-certs-tab">
    <h4 class="node-certs-tab__title">Certificates</h4>

    <KSkeleton v-if="loading" />

    <!-- Cert Info Table -->
    <template v-else>
      <div v-if="certs.length === 0" class="node-certs-tab__empty">
        No certificate information available
      </div>

      <div v-else class="node-certs-tab__table-wrap">
        <table class="node-certs-tab__table">
          <thead>
            <tr>
              <th>Core</th>
              <th>Subject</th>
              <th>Issuer</th>
              <th>Valid From</th>
              <th>Valid Until</th>
              <th>Expires In</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="cert in certs" :key="cert.coreType">
              <td><code>{{ cert.coreType }}</code></td>
              <td>{{ cert.subject }}</td>
              <td>{{ cert.issuer }}</td>
              <td>{{ formatDate(cert.notBefore) }}</td>
              <td>{{ formatDate(cert.notAfter) }}</td>
              <td>
                <span :class="expiryClass(cert.daysUntilExpiry)">
                  <span v-if="cert.daysUntilExpiry < 30" class="node-certs-tab__warning-icon" aria-label="Warning">⚠️</span>
                  {{ cert.daysUntilExpiry }} days
                </span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <KAlert v-if="certs.some(c => c.daysUntilExpiry < 7)" variant="error">
        One or more certificates expire within 7 days. Upload replacements immediately.
      </KAlert>

      <KAlert v-else-if="certs.some(c => c.daysUntilExpiry < 30)" variant="warning">
        One or more certificates expire within 30 days. Consider renewing soon.
      </KAlert>
    </template>

    <!-- Upload Form -->
    <div class="node-certs-tab__upload">
      <h5 class="node-certs-tab__upload-title">Upload Certificates</h5>

      <KFormField name="cert-core-type" label="Core Type">
        <template #default="{ fieldId }">
          <KSelect
            :id="fieldId"
            v-model="uploadCoreType"
            :options="coreTypeOptions"
            placeholder="Select core"
          />
        </template>
      </KFormField>

      <KFormField name="cert-ca" label="CA Certificate (PEM)">
        <template #default="{ fieldId }">
          <KTextarea
            :id="fieldId"
            v-model="caPem"
            :rows="4"
            placeholder="-----BEGIN CERTIFICATE-----"
          />
        </template>
      </KFormField>

      <KFormField name="cert-cert" label="Certificate (PEM)">
        <template #default="{ fieldId }">
          <KTextarea
            :id="fieldId"
            v-model="certPem"
            :rows="4"
            placeholder="-----BEGIN CERTIFICATE-----"
          />
        </template>
      </KFormField>

      <KFormField name="cert-key" label="Private Key (PEM)">
        <template #default="{ fieldId }">
          <KTextarea
            :id="fieldId"
            v-model="keyPem"
            :rows="4"
            placeholder="-----BEGIN PRIVATE KEY-----"
          />
        </template>
      </KFormField>

      <KButton
        variant="primary"
        :loading="submitting"
        :disabled="!uploadValid"
        @click="handleUpload"
      >
        Upload Certificates
      </KButton>
    </div>
  </div>
</template>

<style scoped>
.node-certs-tab {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.node-certs-tab__title {
  margin: 0;
  font-size: var(--text-base);
  font-weight: var(--font-semibold);
  color: var(--color-text);
}

.node-certs-tab__empty {
  padding: var(--space-6);
  text-align: center;
  color: var(--color-muted);
  font-size: var(--text-sm);
}

.node-certs-tab__table-wrap {
  overflow-x: auto;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
}

.node-certs-tab__table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--text-sm);
}

.node-certs-tab__table th {
  text-align: left;
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface);
  border-bottom: 1px solid var(--color-border);
  color: var(--color-muted);
  font-weight: var(--font-medium);
  white-space: nowrap;
}

.node-certs-tab__table td {
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid var(--color-border);
  color: var(--color-text);
  white-space: nowrap;
}

.node-certs-tab__table tr:last-child td {
  border-bottom: none;
}

.node-certs-tab__table code {
  font-family: monospace;
  font-size: var(--text-xs);
}

.node-certs-tab__expiry--ok {
  color: var(--color-success);
}

.node-certs-tab__expiry--warning {
  color: var(--color-warning);
  font-weight: var(--font-medium);
}

.node-certs-tab__expiry--critical {
  color: var(--color-danger);
  font-weight: var(--font-semibold);
}

.node-certs-tab__warning-icon {
  margin-right: var(--space-1);
}

.node-certs-tab__upload {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  max-width: 520px;
}

.node-certs-tab__upload-title {
  margin: 0;
  font-size: var(--text-sm);
  font-weight: var(--font-semibold);
  color: var(--color-text);
}
</style>
