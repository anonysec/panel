<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useApi } from '@koris/composables/useApi'
import ConnectionCard from '@/components/ConnectionCard.vue'
import KUsageBar from '@koris/ui/KUsageBar.vue'
import { sanitizeNumber } from '@/utils/sanitizeNumber'

interface Connection {
  protocol: string
  nodeName: string
  assignedIp: string
  duration: number
  rxBytes: number
  txBytes: number
}

interface UsageData {
  used: number
  limit: number
}

const { get } = useApi()
const connections = ref<Connection[]>([])
const usage = ref<UsageData | null>(null)
const loading = ref(false)

async function loadConnections() {
  loading.value = true
  try {
    const res = await get<{ ok: boolean; connections: any[]; usage?: { used_bytes?: unknown; limit_bytes?: unknown } }>('/api/portal/connections')
    if (res?.ok) {
      connections.value = (res.connections || []).map((c: any) => ({
        protocol: c.protocol || c.core_type || '',
        nodeName: c.node_name || '',
        assignedIp: c.assigned_ip || c.framed_ip || '',
        duration: c.duration || 0,
        rxBytes: c.rx_bytes || c.input_bytes || 0,
        txBytes: c.tx_bytes || c.output_bytes || 0,
      }))
      if (res.usage) {
        usage.value = {
          used: sanitizeNumber(res.usage.used_bytes),
          limit: sanitizeNumber(res.usage.limit_bytes),
        }
      } else {
        usage.value = null
      }
    }
  } catch {
    // Silently fail
  } finally {
    loading.value = false
  }
}

onMounted(loadConnections)
</script>

<template>
  <div class="connections-view">
    <h2 class="page-title">Active Connections</h2>

    <!-- Usage Progress -->
    <KUsageBar
      v-if="usage"
      :used="usage.used"
      :limit="usage.limit"
    />

    <!-- Loading State -->
    <div v-if="loading" class="loading-text">Loading...</div>

    <!-- Empty State -->
    <div v-else-if="connections.length === 0" class="empty-state">
      <p>No active connections</p>
    </div>

    <!-- Connections Grid -->
    <div v-else class="connections-grid">
      <ConnectionCard
        v-for="(conn, idx) in connections"
        :key="idx"
        :protocol="conn.protocol"
        :node-name="conn.nodeName"
        :assigned-ip="conn.assignedIp"
        :duration="conn.duration"
        :rx-bytes="conn.rxBytes"
        :tx-bytes="conn.txBytes"
      />
    </div>
  </div>
</template>

<style scoped>
.connections-view {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
  padding: var(--space-5);
}

.page-title {
  font-size: var(--text-2xl);
  font-weight: var(--font-semibold);
  color: var(--color-text);
  margin: 0;
}

.loading-text {
  font-size: var(--text-sm);
  color: var(--color-muted);
}

.empty-state {
  text-align: center;
  padding: var(--space-8);
  color: var(--color-muted);
}

.connections-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: var(--space-4);
}
</style>
