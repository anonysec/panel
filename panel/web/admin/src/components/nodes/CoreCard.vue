<script setup lang="ts">
import { ref, computed } from 'vue'
import type { CoreInfo } from './types'
import KButton from '@koris/ui/KButton.vue'
import KInput from '@koris/ui/KInput.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'

const props = defineProps<{
  core: CoreInfo
  nodeId: number
}>()

const emit = defineEmits<{
  (e: 'enable', port: number): void
  (e: 'disable'): void
  (e: 'restart', coreType: string): void
}>()

const enablePort = ref(props.core.port || 1194)

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

const icon = computed(() => protocolIcons[props.core.type] || '📦')
const displayName = computed(() => protocolNames[props.core.type] || props.core.type)

/** Map core state to KStatusPill-compatible status string */
const pillStatus = computed(() => {
  if (props.core.state === 'crashed') return 'failed'
  return props.core.state
})

const portValid = computed(() => {
  const p = Number(enablePort.value)
  return Number.isInteger(p) && p >= 1 && p <= 65535
})

function handleEnable() {
  if (portValid.value) {
    emit('enable', Number(enablePort.value))
  }
}

function handleRestart() {
  emit('restart', props.core.type)
}
</script>

<template>
  <div
    class="core-card"
    :class="`core-card--${core.state}`"
    role="region"
    :aria-label="`${displayName} core, status ${core.state}`"
  >
    <div class="core-card__header">
      <span class="core-card__icon" aria-hidden="true">{{ icon }}</span>
      <span class="core-card__name">{{ displayName }}</span>
      <KStatusPill :status="pillStatus" size="sm" />
    </div>

    <!-- Running state -->
    <template v-if="core.state === 'running'">
      <div class="core-card__details">
        <span class="core-card__detail">
          Port: <code>{{ core.port }}</code>
        </span>
        <span class="core-card__detail">
          Sessions: <strong>{{ core.activeSessions }}</strong>
        </span>
      </div>
      <KButton
        variant="danger"
        size="sm"
        aria-label="Disable core"
        @click="emit('disable')"
      >
        Disable
      </KButton>
    </template>

    <!-- Stopped state -->
    <template v-else-if="core.state === 'stopped'">
      <div class="core-card__enable-form">
        <KInput
          v-model="enablePort"
          type="number"
          placeholder="Port"
          aria-label="Port number"
        />
        <KButton
          variant="primary"
          size="sm"
          :disabled="!portValid"
          aria-label="Enable core"
          @click="handleEnable"
        >
          Enable
        </KButton>
      </div>
    </template>

    <!-- Error / Crashed state -->
    <template v-else>
      <p v-if="core.lastError" class="core-card__error-text">
        {{ core.lastError }}
      </p>
      <p v-else class="core-card__error-text">
        Core encountered an error
      </p>
      <div class="core-card__actions">
        <KButton
          variant="primary"
          size="sm"
          aria-label="Restart core"
          @click="handleRestart"
        >
          Restart
        </KButton>
        <KButton
          variant="danger"
          size="sm"
          aria-label="Disable core"
          @click="emit('disable')"
        >
          Disable
        </KButton>
      </div>
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

.core-card--crashed,
.core-card--unknown {
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
  word-break: break-word;
}

.core-card__actions {
  display: flex;
  gap: var(--space-2);
}
</style>
