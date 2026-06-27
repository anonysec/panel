<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useApi } from '@koris/composables/useApi'
import { useToast } from '@koris/composables/useToast'
import KButton from '@koris/ui/KButton.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'
import KSelect from '@koris/ui/KSelect.vue'
import type { KSelectOption } from '@koris/ui/KSelect.vue'

const props = defineProps<{
  coreType: string
  nodeId: number
  initialConfig?: Record<string, any>
}>()

const emit = defineEmits<{
  (e: 'submit', payload: { listenPort: number; extra: Record<string, any> }): void
  (e: 'cancel'): void
}>()

const { post } = useApi()
const toast = useToast()

const saving = ref(false)

// ─── Common field ───────────────────────────────────────────────────────────
const listenPort = ref<number>(props.initialConfig?.listen_port ?? getDefaultPort(props.coreType))

// ─── OpenVPN fields ─────────────────────────────────────────────────────────
const ovpnAuthMode = ref<string>(props.initialConfig?.auth_mode ?? 'userpass')
const ovpnCipher = ref<string>(props.initialConfig?.cipher ?? 'AES-256-GCM')
const ovpnProtocol = ref<string>(props.initialConfig?.protocol ?? 'udp')

// ─── WireGuard fields ───────────────────────────────────────────────────────
const wgSubnet = ref<string>(props.initialConfig?.subnet ?? '10.8.0.0/24')
const wgDns = ref<string>(props.initialConfig?.dns ?? '8.8.8.8,1.1.1.1')

// ─── L2TP fields ────────────────────────────────────────────────────────────
const l2tpAuthType = ref<string>(props.initialConfig?.auth_type ?? 'psk')
const l2tpPsk = ref<string>(props.initialConfig?.psk ?? '')
const l2tpIpPool = ref<string>(props.initialConfig?.ip_pool ?? '10.9.0.0/24')

// ─── IKEv2 fields ───────────────────────────────────────────────────────────
const ikev2Domain = ref<string>(props.initialConfig?.domain ?? '')
const ikev2CertSource = ref<string>(props.initialConfig?.cert_source ?? 'letsencrypt')
const ikev2IpPool = ref<string>(props.initialConfig?.ip_pool ?? '10.10.0.0/24')

// ─── SSH fields ─────────────────────────────────────────────────────────────
const sshPort = ref<number>(props.initialConfig?.port ?? 2222)
const sshMaxConnections = ref<number>(props.initialConfig?.max_connections ?? 100)

// ─── Select options ─────────────────────────────────────────────────────────
const authModeOptions: KSelectOption[] = [
  { label: 'Username/Password', value: 'userpass' },
  { label: 'Certificate', value: 'certificate' },
]

const cipherOptions: KSelectOption[] = [
  { label: 'AES-256-GCM', value: 'AES-256-GCM' },
  { label: 'AES-128-GCM', value: 'AES-128-GCM' },
]

const protocolOptions: KSelectOption[] = [
  { label: 'UDP', value: 'udp' },
  { label: 'TCP', value: 'tcp' },
]

const l2tpAuthOptions: KSelectOption[] = [
  { label: 'Pre-Shared Key', value: 'psk' },
  { label: 'Certificate', value: 'cert' },
]

const certSourceOptions: KSelectOption[] = [
  { label: "Let's Encrypt", value: 'letsencrypt' },
  { label: 'Custom', value: 'custom' },
]

// ─── Default ports ──────────────────────────────────────────────────────────
function getDefaultPort(core: string): number {
  switch (core) {
    case 'openvpn': return 1194
    case 'wireguard': return 51820
    case 'l2tp': return 1701
    case 'ikev2': return 500
    case 'ssh': return 2222
    default: return 1194
  }
}

// ─── Validation ─────────────────────────────────────────────────────────────
const errors = computed(() => {
  const e: Record<string, string> = {}

  const p = Number(listenPort.value)
  if (!Number.isInteger(p) || p < 1 || p > 65535) {
    e.listenPort = 'Port must be 1–65535'
  }

  if (props.coreType === 'ikev2') {
    if (!ikev2Domain.value.trim()) {
      e.domain = 'Domain is required for IKEv2'
    }
  }

  if (props.coreType === 'l2tp' && l2tpAuthType.value === 'psk') {
    if (!l2tpPsk.value.trim()) {
      e.psk = 'Pre-shared key is required'
    }
  }

  if (props.coreType === 'ssh') {
    const sp = Number(sshPort.value)
    if (!Number.isInteger(sp) || sp < 1 || sp > 65535) {
      e.sshPort = 'Port must be 1–65535'
    }
    const mc = Number(sshMaxConnections.value)
    if (!Number.isInteger(mc) || mc < 1) {
      e.maxConnections = 'Must be at least 1'
    }
  }

  return e
})

const isValid = computed(() => Object.keys(errors.value).length === 0)

// ─── Serialization ──────────────────────────────────────────────────────────
function buildExtra(): Record<string, any> {
  switch (props.coreType) {
    case 'openvpn':
      return {
        auth_mode: ovpnAuthMode.value,
        cipher: ovpnCipher.value,
        protocol: ovpnProtocol.value,
      }
    case 'wireguard':
      return {
        subnet: wgSubnet.value,
        dns: wgDns.value,
      }
    case 'l2tp': {
      const extra: Record<string, any> = {
        auth_type: l2tpAuthType.value,
        ip_pool: l2tpIpPool.value,
      }
      if (l2tpAuthType.value === 'psk') {
        extra.psk = l2tpPsk.value
      }
      return extra
    }
    case 'ikev2':
      return {
        domain: ikev2Domain.value.trim(),
        cert_source: ikev2CertSource.value,
        ip_pool: ikev2IpPool.value,
      }
    case 'ssh':
      return {
        port: Number(sshPort.value),
        max_connections: Number(sshMaxConnections.value),
      }
    default:
      return {}
  }
}

// ─── Submit ─────────────────────────────────────────────────────────────────
async function handleSubmit() {
  if (!isValid.value) return

  saving.value = true

  const extra = buildExtra()
  const port = Number(listenPort.value)

  try {
    await post(`/api/admin/knode/nodes/${props.nodeId}/cores/${props.coreType}/enable`, {
      listen_port: port,
      extra,
    })
    toast.success(`${props.coreType} configured successfully`)
    emit('submit', { listenPort: port, extra })
  } catch (err: any) {
    toast.error(err?.message || 'Failed to configure core')
  } finally {
    saving.value = false
  }
}

// Reset fields when coreType changes
watch(() => props.coreType, () => {
  listenPort.value = getDefaultPort(props.coreType)
})
</script>

<template>
  <form class="core-config-form" @submit.prevent="handleSubmit">
    <h3 class="core-config-form__title">
      Configure {{ coreType.toUpperCase() }}
    </h3>

    <!-- Common: Listen Port -->
    <KFormField name="listen-port" label="Listen Port" :error="errors.listenPort">
      <template #default="{ fieldId }">
        <KInput :id="fieldId" v-model="listenPort" type="number" placeholder="Port" />
      </template>
    </KFormField>

    <!-- OpenVPN fields -->
    <template v-if="coreType === 'openvpn'">
      <KFormField name="auth-mode" label="Authentication Mode">
        <template #default="{ fieldId }">
          <KSelect :id="fieldId" v-model="ovpnAuthMode" :options="authModeOptions" />
        </template>
      </KFormField>

      <KFormField name="cipher" label="Cipher">
        <template #default="{ fieldId }">
          <KSelect :id="fieldId" v-model="ovpnCipher" :options="cipherOptions" />
        </template>
      </KFormField>

      <KFormField name="protocol" label="Protocol">
        <template #default="{ fieldId }">
          <KSelect :id="fieldId" v-model="ovpnProtocol" :options="protocolOptions" />
        </template>
      </KFormField>
    </template>

    <!-- WireGuard fields -->
    <template v-if="coreType === 'wireguard'">
      <KFormField name="subnet" label="Tunnel Subnet" hint="CIDR notation, e.g. 10.8.0.0/24">
        <template #default="{ fieldId }">
          <KInput :id="fieldId" v-model="wgSubnet" placeholder="10.8.0.0/24" />
        </template>
      </KFormField>

      <KFormField name="dns" label="DNS Servers" hint="Comma-separated">
        <template #default="{ fieldId }">
          <KInput :id="fieldId" v-model="wgDns" placeholder="8.8.8.8,1.1.1.1" />
        </template>
      </KFormField>
    </template>

    <!-- L2TP fields -->
    <template v-if="coreType === 'l2tp'">
      <KFormField name="auth-type" label="Authentication Type">
        <template #default="{ fieldId }">
          <KSelect :id="fieldId" v-model="l2tpAuthType" :options="l2tpAuthOptions" />
        </template>
      </KFormField>

      <KFormField
        v-if="l2tpAuthType === 'psk'"
        name="psk"
        label="Pre-Shared Key"
        :error="errors.psk"
      >
        <template #default="{ fieldId }">
          <KInput :id="fieldId" v-model="l2tpPsk" type="password" placeholder="Enter PSK" />
        </template>
      </KFormField>

      <KFormField name="ip-pool" label="IP Pool" hint="CIDR notation">
        <template #default="{ fieldId }">
          <KInput :id="fieldId" v-model="l2tpIpPool" placeholder="10.9.0.0/24" />
        </template>
      </KFormField>
    </template>

    <!-- IKEv2 fields -->
    <template v-if="coreType === 'ikev2'">
      <KFormField name="domain" label="Domain" :error="errors.domain" hint="Required for SSL certificate">
        <template #default="{ fieldId }">
          <KInput :id="fieldId" v-model="ikev2Domain" placeholder="vpn.example.com" />
        </template>
      </KFormField>

      <KFormField name="cert-source" label="Certificate Source">
        <template #default="{ fieldId }">
          <KSelect :id="fieldId" v-model="ikev2CertSource" :options="certSourceOptions" />
        </template>
      </KFormField>

      <KFormField name="ikev2-ip-pool" label="IP Pool" hint="CIDR notation">
        <template #default="{ fieldId }">
          <KInput :id="fieldId" v-model="ikev2IpPool" placeholder="10.10.0.0/24" />
        </template>
      </KFormField>
    </template>

    <!-- SSH fields -->
    <template v-if="coreType === 'ssh'">
      <KFormField name="ssh-port" label="SSH Port" :error="errors.sshPort">
        <template #default="{ fieldId }">
          <KInput :id="fieldId" v-model="sshPort" type="number" placeholder="2222" />
        </template>
      </KFormField>

      <KFormField name="max-connections" label="Max Connections" :error="errors.maxConnections">
        <template #default="{ fieldId }">
          <KInput :id="fieldId" v-model="sshMaxConnections" type="number" placeholder="100" />
        </template>
      </KFormField>
    </template>

    <!-- Actions -->
    <div class="core-config-form__actions">
      <KButton
        type="submit"
        variant="primary"
        :loading="saving"
        :disabled="!isValid"
      >
        Apply Configuration
      </KButton>
      <KButton variant="ghost" @click="emit('cancel')">
        Cancel
      </KButton>
    </div>
  </form>
</template>

<style scoped>
.core-config-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  max-width: 480px;
}

.core-config-form__title {
  margin: 0;
  font-size: var(--text-lg);
  font-weight: var(--font-semibold);
  color: var(--color-text);
}

.core-config-form__actions {
  display: flex;
  gap: var(--space-2);
  padding-top: var(--space-2);
}
</style>
