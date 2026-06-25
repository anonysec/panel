<script setup lang="ts">
import { ref, computed } from 'vue'
import KButton from '@koris/ui/KButton.vue'
import KInput from '@koris/ui/KInput.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'

const props = defineProps<{
  nodeId: number
  coreType: string
  status: 'running' | 'stopped' | 'error'
  port?: number
  sessions?: number
  pid?: number
}>()

const emit = defineEmits<{
  (e: 'enable', port: number): void
  (e: 'disable'): void
}>()

const enablePort = ref(1194)

const protocolIcons: Record<string, string> = {
  openvpn: '🔐',
  wireguard: '🛡️',
  l2tp: '🔗',
  ikev2: '🔑',
  ssh: '💻',
  mtproto: '📡',
  xray: '⚡',
}

const protocolNames: Record<string, string> = {
  openvpn: 'OpenVPN',
  wireguard: 'WireGuard',
  l2tp: 'L2TP',
  ikev2: 'IKEv2',
  ssh: 'SSH',
  mtproto: 'MTProto',
  xray: 'Xray',
}

const icon = computed(() => protocolIcons[props.coreType] || '📦')
const displayName = computed(() => protocolNames[props.coreType] || props.coreType)

const portValid = computed(() => {
  const p = Number(enablePort.value)
  return Number.isInteger(p) && p >= 1 && p <= 65535
})

function handleEnable() {
  if (portValid.value) {
    emit('enable', Number(enablePort.value))
  }
}
</script>

<template>
  <div class="core-card" :class="`core-card--${status}`">
    <div class="core-card__header">
      <span class="core-card__icon" aria-hidden="true">{{ icon }}</span>
      <span class="core-card__name">{{ displayName }}</span>
      <KStatusPill :status="status" size="sm" />
    </div>

    <!-- Running state -->
    <template v-if="status === 'running'">
      <div class="core-card__details">
        <span v-if="port" class="core-card__detail">
          Port: <code>{{ port }}</code>
        </span>
        <span v-if="sessions != null" class="core-card__detail">
          Sessions: <strong>{{ sessions }}</strong>
        </span>
        <span v-if="pid" class="core-card__detail">
          PID: <code>{{ pid }}</code>
        </span>
      </div>
      <KButton variant="danger" size="sm" @click="emit('disable')">
        Disable
      </KButton>
    </template>

    <!-- Stopped state -->
    <template v-else-if="status === 'stopped'">
      <div class="core-card__enable-form">
        <KInput
          v-model="enablePort"
          type="number"
          placeholder="Port"
        />
        <KButton
          variant="primary"
          size="sm"
          :disabled="!portValid"
          @click="handleEnable"
        >
          Enable
        </KButton>
      </div>
    </template>

    <!-- Error state -->
    <template v-else>
      <p class="core-card__error-text">Core encountered an error</p>
      <KButton variant="danger" size="sm" @click="emit('disable')">
        Disable
      </KButton>
    </template>
  </div>
</template>

<style scoped>
.core-card {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-3);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
}

.core-card--running {
  border-left: 3px solid var(--color-success);
}

.core-card--stopped {
  border-left: 3px solid var(--color-muted);
}

.core-card--error {
  border-left: 3px solid var(--color-danger);
}

.core-card__header {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.core-card__icon {
  font-size: 1.2em;
}

.core-card__name {
  font-weight: var(--font-medium);
  color: var(--color-text);
  flex: 1;
}

.core-card__details {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  font-size: var(--text-sm);
  color: var(--color-muted);
}

.core-card__detail code {
  font-family: monospace;
  color: var(--color-text);
}

.core-card__detail strong {
  color: var(--color-text);
}

.core-card__enable-form {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.core-card__enable-form .k-input {
  max-width: 100px;
}

.core-card__error-text {
  margin: 0;
  font-size: var(--text-sm);
  color: var(--color-danger);
}
</style>
