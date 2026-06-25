<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useNodesStore, type Tunnel, type TunnelConfig } from '@/stores/nodes'
import { useEditionStore } from '@/stores/edition'
import { useToast } from '@koris/composables/useToast'
import KButton from '@koris/ui/KButton.vue'
import KInput from '@koris/ui/KInput.vue'
import KSelect from '@koris/ui/KSelect.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'

const props = defineProps<{
  nodeId: number
}>()

const nodesStore = useNodesStore()
const edition = useEditionStore()
const toast = useToast()

const tunnels = ref<Tunnel[]>([])
const loading = ref(false)
const tearingDown = ref<string | null>(null)

// ─── Setup Tunnel Form ──────────────────────────────────────────────────────
const newProtocol = ref('vless-reality')
const newExitAddress = ref('')
const newExitPort = ref<number | ''>('')
const submitting = ref(false)

const protocolOptions = [
  { label: 'VLESS + Reality', value: 'vless-reality' },
  { label: 'WireGuard', value: 'wireguard' },
  { label: 'SSH', value: 'ssh' },
  { label: 'Rathole', value: 'rathole' },
]

const formValid = computed(() => {
  const p = Number(newExitPort.value)
  return (
    newExitAddress.value.trim().length > 0 &&
    Number.isInteger(p) && p >= 1 && p <= 65535
  )
})

// ─── State mapping for KStatusPill ──────────────────────────────────────────
function tunnelStatusMap(state: string): 'running' | 'stopped' | 'error' {
  if (state === 'active') return 'running'
  if (state === 'inactive') return 'stopped'
  return 'error'
}

// ─── Actions ────────────────────────────────────────────────────────────────

async function loadTunnels() {
  loading.value = true
  tunnels.value = await nodesStore.listTunnels(props.nodeId)
  loading.value = false
}

async function handleSetup() {
  if (!formValid.value) return

  submitting.value = true
  const config: TunnelConfig = {
    protocol: newProtocol.value,
    exitAddress: newExitAddress.value.trim(),
    exitPort: Number(newExitPort.value),
  }

  const ok = await nodesStore.setupTunnel(props.nodeId, config)
  if (ok) {
    toast.success('Tunnel created')
    newExitAddress.value = ''
    newExitPort.value = ''
    await loadTunnels()
  } else {
    toast.error('Failed to setup tunnel')
  }
  submitting.value = false
}

async function handleTeardown(tunnelId: string) {
  tearingDown.value = tunnelId
  const ok = await nodesStore.teardownTunnel(props.nodeId, tunnelId)
  if (ok) {
    tunnels.value = tunnels.value.filter(t => t.id !== tunnelId)
    toast.success('Tunnel torn down')
  } else {
    toast.error('Failed to teardown tunnel')
  }
  tearingDown.value = null
}

onMounted(loadTunnels)
</script>

<template>
  <div v-if="edition.isFull" class="node-tunnels-tab">
    <h4 class="node-tunnels-tab__title">Outbound Tunnels</h4>

    <!-- Setup Tunnel Form -->
    <form class="node-tunnels-tab__form" @submit.prevent="handleSetup">
      <KSelect
        v-model="newProtocol"
        :options="protocolOptions"
        class="node-tunnels-tab__protocol-select"
      />
      <KInput
        v-model="newExitAddress"
        placeholder="Exit address"
        class="node-tunnels-tab__address-input"
      />
      <KInput
        v-model="newExitPort"
        type="number"
        placeholder="Port"
        class="node-tunnels-tab__port-input"
      />
      <KButton type="submit" variant="primary" size="sm" :loading="submitting" :disabled="!formValid">
        Setup Tunnel
      </KButton>
    </form>

    <KSkeleton v-if="loading" />

    <div v-else-if="tunnels.length === 0" class="node-tunnels-tab__empty">
      No tunnels configured
    </div>

    <div v-else class="node-tunnels-tab__table-wrap">
      <table class="node-tunnels-tab__table">
        <thead>
          <tr>
            <th>ID</th>
            <th>Protocol</th>
            <th>Exit Address</th>
            <th>Exit Port</th>
            <th>State</th>
            <th>Created</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="tunnel in tunnels" :key="tunnel.id">
            <td><code>{{ tunnel.id }}</code></td>
            <td>{{ tunnel.protocol }}</td>
            <td><code>{{ tunnel.exitAddress }}</code></td>
            <td><code>{{ tunnel.exitPort }}</code></td>
            <td>
              <KStatusPill :status="tunnelStatusMap(tunnel.state)" size="sm" />
            </td>
            <td>{{ new Date(tunnel.createdAt).toLocaleDateString() }}</td>
            <td>
              <KButton
                variant="danger"
                size="sm"
                :loading="tearingDown === tunnel.id"
                @click="handleTeardown(tunnel.id)"
              >
                Teardown
              </KButton>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.node-tunnels-tab {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.node-tunnels-tab__title {
  margin: 0;
  font-size: var(--text-base);
  font-weight: var(--font-semibold);
  color: var(--color-text);
}

.node-tunnels-tab__form {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex-wrap: wrap;
  padding: var(--space-3);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
}

.node-tunnels-tab__protocol-select {
  max-width: 180px;
}

.node-tunnels-tab__address-input {
  flex: 1;
  min-width: 160px;
}

.node-tunnels-tab__port-input {
  max-width: 100px;
}

.node-tunnels-tab__empty {
  padding: var(--space-6);
  text-align: center;
  color: var(--color-muted);
  font-size: var(--text-sm);
}

.node-tunnels-tab__table-wrap {
  overflow-x: auto;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
}

.node-tunnels-tab__table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--text-sm);
}

.node-tunnels-tab__table th {
  text-align: left;
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface);
  border-bottom: 1px solid var(--color-border);
  color: var(--color-muted);
  font-weight: var(--font-medium);
  white-space: nowrap;
}

.node-tunnels-tab__table td {
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid var(--color-border);
  color: var(--color-text);
  white-space: nowrap;
}

.node-tunnels-tab__table tr:last-child td {
  border-bottom: none;
}

.node-tunnels-tab__table code {
  font-family: monospace;
  font-size: var(--text-xs);
}
</style>
