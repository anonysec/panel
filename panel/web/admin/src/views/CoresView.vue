<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useApi } from '@koris/composables/useApi'
import { useToast } from '@koris/composables/useToast'
import KSkeleton from '@koris/ui/KSkeleton.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'

const { get, post } = useApi()
const toast = useToast()

// ═══════════════════════════════════════════════════════════════════════════════
// Types
// ═══════════════════════════════════════════════════════════════════════════════

interface Node {
  id: number
  name: string
  address: string
}

interface ProtocolConfig {
  enabled: boolean
  port: number
  network: string
}

type VpnConfig = Record<string, ProtocolConfig>

interface NodeWithConfig {
  node: Node
  config: VpnConfig
  loading: boolean
}

// ═══════════════════════════════════════════════════════════════════════════════
// Protocol definitions
// ═══════════════════════════════════════════════════════════════════════════════

const protocols = [
  { key: 'openvpn', name: 'OpenVPN', icon: '🔐', defaultPort: 1194 },
  { key: 'wireguard', name: 'WireGuard', icon: '🛡️', defaultPort: 51820 },
  { key: 'l2tp', name: 'L2TP/IPsec', icon: '🔗', defaultPort: 1701 },
  { key: 'ikev2', name: 'IKEv2', icon: '🔑', defaultPort: 500 },
  { key: 'ssh', name: 'SSH Tunnel', icon: '💻', defaultPort: 2222 },
]

// ═══════════════════════════════════════════════════════════════════════════════
// State
// ═══════════════════════════════════════════════════════════════════════════════

const nodes = ref<NodeWithConfig[]>([])
const loading = ref(true)

// ═══════════════════════════════════════════════════════════════════════════════
// Data fetching
// ═══════════════════════════════════════════════════════════════════════════════

onMounted(async () => {
  await fetchNodes()
})

async function fetchNodes() {
  loading.value = true
  try {
    const res = await get('/api/admin/knode/nodes')
    const nodeList: Node[] = res.nodes || []
    nodes.value = nodeList.map(node => ({
      node,
      config: {},
      loading: true,
    }))
    // Fetch VPN config for each node in parallel
    await Promise.allSettled(
      nodes.value.map((entry, idx) => fetchVpnConfig(entry.node.id, idx))
    )
  } catch {
    // error toast handled by useApi
  } finally {
    loading.value = false
  }
}

async function fetchVpnConfig(nodeId: number, idx: number) {
  try {
    const res = await get(`/api/nodes/vpn-config/${nodeId}`)
    // API returns { configs: [{protocol, enabled, port, network, ...}] }
    // Transform to Record<protocolKey, ProtocolConfig>
    const configMap: VpnConfig = {}
    const configs = res.configs || []
    for (const c of configs) {
      configMap[c.protocol] = {
        enabled: c.enabled,
        port: c.port,
        network: c.network || '',
      }
    }
    nodes.value[idx].config = configMap
  } catch {
    // Node might be offline — leave config empty
  } finally {
    nodes.value[idx].loading = false
  }
}

// ═══════════════════════════════════════════════════════════════════════════════
// Toggle protocol
// ═══════════════════════════════════════════════════════════════════════════════

async function toggleProtocol(nodeId: number, idx: number, protocolKey: string) {
  const entry = nodes.value[idx]
  const current = entry.config[protocolKey]
  const newEnabled = !(current?.enabled ?? false)

  // Optimistic update
  if (!entry.config[protocolKey]) {
    const proto = protocols.find(p => p.key === protocolKey)
    entry.config[protocolKey] = {
      enabled: newEnabled,
      port: proto?.defaultPort || 0,
      network: '',
    }
  } else {
    entry.config[protocolKey].enabled = newEnabled
  }

  try {
    await post(`/api/nodes/vpn-config/${nodeId}`, {
      protocol: protocolKey,
      enabled: newEnabled,
    })
    toast.success(`${protocolKey} ${newEnabled ? 'enabled' : 'disabled'}`)
  } catch {
    // Revert on failure
    entry.config[protocolKey].enabled = !newEnabled
  }
}

// ═══════════════════════════════════════════════════════════════════════════════
// Helpers
// ═══════════════════════════════════════════════════════════════════════════════

function getProtoConfig(config: VpnConfig, key: string): ProtocolConfig | null {
  return config[key] || null
}
</script>

<template>
  <div class="page cores-view">
    <header class="page-header">
      <h1>VPN Cores</h1>
      <p class="subtitle">Protocol configurations across all nodes</p>
    </header>

    <!-- Loading state -->
    <div v-if="loading" class="loading-grid">
      <KSkeleton v-for="i in 3" :key="i" height="200px" />
    </div>

    <!-- Empty state -->
    <div v-else-if="nodes.length === 0" class="empty-state">
      <p>No nodes found. Add nodes to manage VPN protocols.</p>
    </div>

    <!-- Node sections -->
    <section v-for="(entry, idx) in nodes" :key="entry.node.id" class="node-section">
      <div class="node-header">
        <h3>{{ entry.node.name }}</h3>
        <span class="node-address">{{ entry.node.address }}</span>
      </div>

      <!-- Per-node loading -->
      <div v-if="entry.loading" class="protocol-grid">
        <KSkeleton v-for="i in 5" :key="i" height="120px" />
      </div>

      <!-- Protocol grid -->
      <div v-else class="protocol-grid">
        <div
          v-for="proto in protocols"
          :key="proto.key"
          class="protocol-card"
          :class="{ enabled: getProtoConfig(entry.config, proto.key)?.enabled }"
        >
          <div class="proto-icon">{{ proto.icon }}</div>
          <div class="proto-info">
            <span class="proto-name">{{ proto.name }}</span>
            <span class="proto-port">
              Port: {{ getProtoConfig(entry.config, proto.key)?.port || proto.defaultPort }}
            </span>
            <span v-if="getProtoConfig(entry.config, proto.key)?.network" class="proto-network">
              {{ getProtoConfig(entry.config, proto.key)?.network }}
            </span>
          </div>
          <div class="proto-status">
            <KStatusPill
              :status="getProtoConfig(entry.config, proto.key)?.enabled ? 'active' : 'inactive'"
              :label="getProtoConfig(entry.config, proto.key)?.enabled ? 'Enabled' : 'Disabled'"
            />
          </div>
          <button
            class="toggle-btn"
            :class="{ active: getProtoConfig(entry.config, proto.key)?.enabled }"
            :title="getProtoConfig(entry.config, proto.key)?.enabled ? 'Disable' : 'Enable'"
            @click="toggleProtocol(entry.node.id, idx, proto.key)"
          >
            <span class="toggle-track">
              <span class="toggle-thumb" />
            </span>
          </button>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.cores-view {
  padding: var(--space-6, 24px);
  max-width: 1200px;
}

.page-header {
  margin-bottom: var(--space-6, 24px);
}

.page-header h1 {
  font-size: var(--text-2xl, 24px);
  font-weight: var(--font-bold, 700);
  margin: 0;
}

.page-header .subtitle {
  color: var(--color-muted, #8b98a5);
  font-size: var(--text-sm, 13px);
  margin: var(--space-1, 4px) 0 0;
}

.loading-grid {
  display: grid;
  gap: var(--space-4, 16px);
}

.empty-state {
  text-align: center;
  padding: var(--space-10, 60px) var(--space-4, 16px);
  color: var(--color-muted, #8b98a5);
}

/* Node sections */
.node-section {
  margin-bottom: var(--space-8, 32px);
  background: var(--color-surface, #161b22);
  border: 1px solid var(--color-border, #28333f);
  border-radius: var(--radius-lg, 12px);
  padding: var(--space-5, 20px);
}

.node-header {
  display: flex;
  align-items: baseline;
  gap: var(--space-3, 12px);
  margin-bottom: var(--space-4, 16px);
}

.node-header h3 {
  font-size: var(--text-lg, 16px);
  font-weight: var(--font-semibold, 600);
  margin: 0;
}

.node-address {
  font-size: var(--text-sm, 13px);
  color: var(--color-muted, #8b98a5);
  font-family: var(--font-mono, monospace);
}

/* Protocol grid */
.protocol-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: var(--space-3, 12px);
}

.protocol-card {
  background: var(--color-surface-2, #1e2630);
  border: 1px solid var(--color-border, #28333f);
  border-radius: var(--radius-md, 8px);
  padding: var(--space-4, 16px);
  display: flex;
  flex-direction: column;
  gap: var(--space-2, 8px);
  transition: border-color var(--duration-normal, 0.15s);
}

.protocol-card.enabled {
  border-color: var(--color-success, #22c55e);
}

.proto-icon {
  font-size: 24px;
}

.proto-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex: 1;
}

.proto-name {
  font-weight: var(--font-semibold, 600);
  font-size: var(--text-base, 14px);
}

.proto-port {
  font-size: var(--text-xs, 11px);
  color: var(--color-muted, #8b98a5);
  font-family: var(--font-mono, monospace);
}

.proto-network {
  font-size: var(--text-xs, 11px);
  color: var(--color-muted, #8b98a5);
  font-family: var(--font-mono, monospace);
}

.proto-status {
  margin-top: var(--space-1, 4px);
}

/* Toggle button */
.toggle-btn {
  align-self: flex-end;
  background: none;
  border: none;
  cursor: pointer;
  padding: 0;
}

.toggle-track {
  display: block;
  width: 36px;
  height: 20px;
  border-radius: 10px;
  background: var(--color-border, #28333f);
  position: relative;
  transition: background var(--duration-normal, 0.15s);
}

.toggle-btn.active .toggle-track {
  background: var(--color-success, #22c55e);
}

.toggle-thumb {
  display: block;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: #fff;
  position: absolute;
  top: 2px;
  left: 2px;
  transition: transform var(--duration-normal, 0.15s);
}

.toggle-btn.active .toggle-thumb {
  transform: translateX(16px);
}
</style>
