<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useNodesStore, type FirewallRule } from '@/stores/nodes'
import { useToast } from '@koris/composables/useToast'
import KButton from '@koris/ui/KButton.vue'
import KInput from '@koris/ui/KInput.vue'
import KSelect from '@koris/ui/KSelect.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'

const props = defineProps<{
  nodeId: number
}>()

const nodesStore = useNodesStore()
const toast = useToast()

const rules = ref<FirewallRule[]>([])
const loading = ref(false)
const closing = ref<string | null>(null)

// ─── Open Port Form ─────────────────────────────────────────────────────────
const newPort = ref<number | ''>('')
const newProtocol = ref('tcp')
const newComment = ref('')
const submitting = ref(false)

const protocolOptions = [
  { label: 'TCP', value: 'tcp' },
  { label: 'UDP', value: 'udp' },
]

const portValid = computed(() => {
  const p = Number(newPort.value)
  return Number.isInteger(p) && p >= 1 && p <= 65535
})

// ─── Actions ────────────────────────────────────────────────────────────────

async function loadRules() {
  loading.value = true
  rules.value = await nodesStore.listFirewallRules(props.nodeId)
  loading.value = false
}

async function handleOpenPort() {
  if (!portValid.value) return

  submitting.value = true
  const ok = await nodesStore.openPort(
    props.nodeId,
    Number(newPort.value),
    newProtocol.value,
    newComment.value.trim()
  )

  if (ok) {
    toast.success(`Port ${newPort.value}/${newProtocol.value} opened`)
    newPort.value = ''
    newComment.value = ''
    await loadRules()
  } else {
    toast.error('Failed to open port')
  }
  submitting.value = false
}

async function handleClose(port: number, protocol: string) {
  const key = `${port}/${protocol}`
  closing.value = key
  const ok = await nodesStore.closePort(props.nodeId, port, protocol)
  if (ok) {
    rules.value = rules.value.filter(r => !(r.port === port && r.protocol === protocol))
    toast.success(`Port ${key} closed`)
  } else {
    toast.error(`Failed to close port ${key}`)
  }
  closing.value = null
}

onMounted(loadRules)
</script>

<template>
  <div class="node-firewall-tab">
    <h4 class="node-firewall-tab__title">Firewall Rules</h4>

    <!-- Open Port Form -->
    <form class="node-firewall-tab__form" @submit.prevent="handleOpenPort">
      <KInput
        v-model="newPort"
        type="number"
        placeholder="Port"
        class="node-firewall-tab__port-input"
      />
      <KSelect
        v-model="newProtocol"
        :options="protocolOptions"
        class="node-firewall-tab__protocol-select"
      />
      <KInput
        v-model="newComment"
        placeholder="Comment (optional)"
        class="node-firewall-tab__comment-input"
      />
      <KButton type="submit" variant="primary" size="sm" :loading="submitting" :disabled="!portValid">
        Open Port
      </KButton>
    </form>

    <KSkeleton v-if="loading" />

    <div v-else-if="rules.length === 0" class="node-firewall-tab__empty">
      No firewall rules configured
    </div>

    <div v-else class="node-firewall-tab__table-wrap">
      <table class="node-firewall-tab__table">
        <thead>
          <tr>
            <th>Port</th>
            <th>Protocol</th>
            <th>Direction</th>
            <th>Action</th>
            <th>Source CIDR</th>
            <th>Comment</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="rule in rules" :key="`${rule.port}-${rule.protocol}`">
            <td><code>{{ rule.port }}</code></td>
            <td>{{ rule.protocol.toUpperCase() }}</td>
            <td>{{ rule.direction }}</td>
            <td>
              <span
                class="node-firewall-tab__action-badge"
                :class="`node-firewall-tab__action-badge--${rule.action}`"
              >
                {{ rule.action }}
              </span>
            </td>
            <td><code>{{ rule.sourceCidr || '*' }}</code></td>
            <td class="node-firewall-tab__cell--comment">{{ rule.comment || '—' }}</td>
            <td>
              <KButton
                variant="danger"
                size="sm"
                :loading="closing === `${rule.port}/${rule.protocol}`"
                @click="handleClose(rule.port, rule.protocol)"
              >
                Close
              </KButton>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.node-firewall-tab {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.node-firewall-tab__title {
  margin: 0;
  font-size: var(--text-base);
  font-weight: var(--font-semibold);
  color: var(--color-text);
}

.node-firewall-tab__form {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex-wrap: wrap;
  padding: var(--space-3);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
}

.node-firewall-tab__port-input {
  max-width: 100px;
}

.node-firewall-tab__protocol-select {
  max-width: 100px;
}

.node-firewall-tab__comment-input {
  flex: 1;
  min-width: 140px;
}

.node-firewall-tab__empty {
  padding: var(--space-6);
  text-align: center;
  color: var(--color-muted);
  font-size: var(--text-sm);
}

.node-firewall-tab__table-wrap {
  overflow-x: auto;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
}

.node-firewall-tab__table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--text-sm);
}

.node-firewall-tab__table th {
  text-align: left;
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface);
  border-bottom: 1px solid var(--color-border);
  color: var(--color-muted);
  font-weight: var(--font-medium);
  white-space: nowrap;
}

.node-firewall-tab__table td {
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid var(--color-border);
  color: var(--color-text);
  white-space: nowrap;
}

.node-firewall-tab__table tr:last-child td {
  border-bottom: none;
}

.node-firewall-tab__table code {
  font-family: monospace;
  font-size: var(--text-xs);
}

.node-firewall-tab__action-badge {
  display: inline-block;
  padding: 1px var(--space-2);
  border-radius: var(--radius-sm);
  font-size: var(--text-xs);
  font-weight: var(--font-medium);
  text-transform: uppercase;
}

.node-firewall-tab__action-badge--allow {
  background: color-mix(in srgb, var(--color-success) 15%, transparent);
  color: var(--color-success);
}

.node-firewall-tab__action-badge--deny {
  background: color-mix(in srgb, var(--color-danger) 15%, transparent);
  color: var(--color-danger);
}

.node-firewall-tab__cell--comment {
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
