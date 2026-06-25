<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useNodesStore, type VPNSession } from '@/stores/nodes'
import { useToast } from '@koris/composables/useToast'
import KButton from '@koris/ui/KButton.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'

const props = defineProps<{
  nodeId: number
}>()

const nodesStore = useNodesStore()
const toast = useToast()

const sessions = ref<VPNSession[]>([])
const loading = ref(false)
const disconnecting = ref<string | null>(null)

// ─── Formatting Helpers ─────────────────────────────────────────────────────

function formatDuration(seconds: number): string {
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = seconds % 60
  if (h > 0) return `${h}h ${m}m`
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

// ─── Actions ────────────────────────────────────────────────────────────────

async function loadSessions() {
  loading.value = true
  sessions.value = await nodesStore.listSessions(props.nodeId)
  loading.value = false
}

async function handleDisconnect(username: string) {
  disconnecting.value = username
  const ok = await nodesStore.disconnectUser(props.nodeId, username)
  if (ok) {
    sessions.value = sessions.value.filter(s => s.username !== username)
    toast.success(`Disconnected ${username}`)
  } else {
    toast.error(`Failed to disconnect ${username}`)
  }
  disconnecting.value = null
}

onMounted(loadSessions)
</script>

<template>
  <div class="node-sessions-tab">
    <div class="node-sessions-tab__header">
      <h4 class="node-sessions-tab__title">Active Sessions</h4>
      <KButton variant="ghost" size="sm" :loading="loading" @click="loadSessions">
        Refresh
      </KButton>
    </div>

    <KSkeleton v-if="loading" />

    <div v-else-if="sessions.length === 0" class="node-sessions-tab__empty">
      No active sessions
    </div>

    <div v-else class="node-sessions-tab__table-wrap">
      <table class="node-sessions-tab__table">
        <thead>
          <tr>
            <th>Username</th>
            <th>Core</th>
            <th>Client IP</th>
            <th>Assigned IP</th>
            <th>Duration</th>
            <th>RX</th>
            <th>TX</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="session in sessions" :key="session.username + session.coreType">
            <td class="node-sessions-tab__cell--username">{{ session.username }}</td>
            <td><code>{{ session.coreType }}</code></td>
            <td><code>{{ session.clientIp }}</code></td>
            <td><code>{{ session.assignedIp }}</code></td>
            <td>{{ formatDuration(session.duration) }}</td>
            <td>{{ formatBytes(session.rxBytes) }}</td>
            <td>{{ formatBytes(session.txBytes) }}</td>
            <td>
              <KButton
                variant="danger"
                size="sm"
                :loading="disconnecting === session.username"
                @click="handleDisconnect(session.username)"
              >
                Disconnect
              </KButton>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.node-sessions-tab {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.node-sessions-tab__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.node-sessions-tab__title {
  margin: 0;
  font-size: var(--text-base);
  font-weight: var(--font-semibold);
  color: var(--color-text);
}

.node-sessions-tab__empty {
  padding: var(--space-6);
  text-align: center;
  color: var(--color-muted);
  font-size: var(--text-sm);
}

.node-sessions-tab__table-wrap {
  overflow-x: auto;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
}

.node-sessions-tab__table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--text-sm);
}

.node-sessions-tab__table th {
  text-align: left;
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface);
  border-bottom: 1px solid var(--color-border);
  color: var(--color-muted);
  font-weight: var(--font-medium);
  white-space: nowrap;
}

.node-sessions-tab__table td {
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid var(--color-border);
  color: var(--color-text);
  white-space: nowrap;
}

.node-sessions-tab__table tr:last-child td {
  border-bottom: none;
}

.node-sessions-tab__cell--username {
  font-weight: var(--font-medium);
}

.node-sessions-tab__table code {
  font-family: monospace;
  font-size: var(--text-xs);
}
</style>
