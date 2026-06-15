<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useNodesStore } from '@/stores/nodes'
import KTabs from '@koris/ui/KTabs.vue'
import KButton from '@koris/ui/KButton.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'

const store = useNodesStore()
const activeTab = ref('nodes')
const showAddForm = ref(false)
const creating = ref(false)
const newToken = ref<string | null>(null)

const tabs = [
  { key: 'nodes', label: 'Nodes' },
  { key: 'cores', label: 'Cores' },
]

const nodeForm = ref({
  name: '',
  public_ip: '',
  domain: '',
})

const protocols = ['OpenVPN', 'L2TP', 'IKEv2', 'SSH'] as const

function formatBps(bps: number): string {
  if (bps < 1000) return `${bps} bps`
  if (bps < 1000000) return `${(bps / 1000).toFixed(1)} Kbps`
  return `${(bps / 1000000).toFixed(1)} Mbps`
}

async function handleCreateNode() {
  creating.value = true
  const token = await store.createNode({
    name: nodeForm.value.name,
    public_ip: nodeForm.value.public_ip,
    domain: nodeForm.value.domain,
  })
  creating.value = false
  if (token) {
    newToken.value = token
    nodeForm.value = { name: '', public_ip: '', domain: '' }
    showAddForm.value = false
  }
}

async function toggleNode(id: number, currentStatus: string) {
  const enable = currentStatus === 'offline'
  await store.updateNode(id, enable)
}

function getServiceStatus(node: any, protocol: string): string {
  const key = protocol.toLowerCase().replace('openvpn', 'openvpn').replace('l2tp', 'l2tp').replace('ikev2', 'ikev2')
  const metrics = node.status_metrics
  if (!metrics) return 'unknown'
  if (key === 'openvpn') return metrics.openvpn_status || 'unknown'
  if (key === 'l2tp') return metrics.l2tp_status || 'unknown'
  if (key === 'ikev2') return metrics.ikev2_status || 'unknown'
  return 'unknown'
}

onMounted(() => {
  store.loadNodes()
})
</script>

<template>
  <div class="page nodes-view">
    <header class="page-header">
      <h2 class="page-title">Nodes</h2>
      <KButton variant="primary" icon="+" @click="showAddForm = true">Add Node</KButton>
    </header>

    <!-- New Token Display -->
    <div v-if="newToken" class="token-banner">
      <p><strong>Node Token:</strong> <code>{{ newToken }}</code></p>
      <p class="text-muted text-sm">Save this token — it won't be shown again.</p>
      <KButton variant="ghost" size="sm" @click="newToken = null">Dismiss</KButton>
    </div>

    <!-- Add Node Form -->
    <div v-if="showAddForm" class="panel">
      <h4 class="panel-title">Add New Node</h4>
      <form class="node-form" @submit.prevent="handleCreateNode">
        <div class="form-grid">
          <KFormField name="node-name" label="Name" required>
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="nodeForm.name" placeholder="node-us-1" />
            </template>
          </KFormField>
          <KFormField name="node-ip" label="Public IP" required>
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="nodeForm.public_ip" placeholder="1.2.3.4" />
            </template>
          </KFormField>
          <KFormField name="node-domain" label="Domain" required>
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="nodeForm.domain" placeholder="us1.example.com" />
            </template>
          </KFormField>
        </div>
        <div class="form-actions">
          <KButton variant="ghost" @click="showAddForm = false">Cancel</KButton>
          <KButton type="submit" variant="primary" :loading="creating">Create Node</KButton>
        </div>
      </form>
    </div>

    <KTabs v-model="activeTab" :tabs="tabs" aria-label="Nodes navigation">
      <!-- Nodes Tab -->
      <template #nodes>
        <div class="nodes-content">
          <div v-if="store.loading && store.list.length === 0" class="nodes-grid">
            <KSkeleton v-for="i in 3" :key="i" variant="rect" :width="'100%'" :height="180" />
          </div>
          <KEmptyState
            v-else-if="store.list.length === 0"
            icon="🖥️"
            title="No Nodes"
            description="Add your first VPN node to get started."
          />
          <div v-else class="nodes-grid">
            <div v-for="node in store.list" :key="node.id" class="node-card">
              <div class="node-card__header">
                <div class="node-card__title">
                  <h4 class="node-card__name">{{ node.name }}</h4>
                  <KStatusPill :status="node.status" size="sm" />
                </div>
                <span class="node-card__ip text-muted">{{ node.public_ip }}</span>
              </div>

              <div class="node-card__metrics">
                <div class="metric-row">
                  <span class="metric-row__label">CPU</span>
                  <div class="metric-row__bar">
                    <div class="metric-row__fill" :style="{ width: `${node.status_metrics?.cpu_percent ?? 0}%` }" />
                  </div>
                  <span class="metric-row__val">{{ node.status_metrics?.cpu_percent ?? 0 }}%</span>
                </div>
                <div class="metric-row">
                  <span class="metric-row__label">RAM</span>
                  <div class="metric-row__bar">
                    <div class="metric-row__fill metric-row__fill--accent" :style="{ width: `${node.status_metrics?.ram_percent ?? 0}%` }" />
                  </div>
                  <span class="metric-row__val">{{ node.status_metrics?.ram_percent ?? 0 }}%</span>
                </div>
                <div class="metric-row">
                  <span class="metric-row__label">Disk</span>
                  <div class="metric-row__bar">
                    <div class="metric-row__fill metric-row__fill--warning" :style="{ width: `${node.status_metrics?.disk_percent ?? 0}%` }" />
                  </div>
                  <span class="metric-row__val">{{ node.status_metrics?.disk_percent ?? 0 }}%</span>
                </div>
                <div class="metric-text">
                  <span class="text-muted">RX:</span> {{ formatBps(node.status_metrics?.rx_bps ?? 0) }}
                  <span class="text-muted" style="margin-left:var(--space-3)">TX:</span> {{ formatBps(node.status_metrics?.tx_bps ?? 0) }}
                </div>
              </div>

              <div class="node-card__actions">
                <KButton variant="ghost" size="sm" @click="toggleNode(node.id, node.status)">
                  {{ node.status === 'online' ? 'Disable' : 'Enable' }}
                </KButton>
              </div>
            </div>
          </div>
        </div>
      </template>

      <!-- Cores Tab -->
      <template #cores>
        <div class="cores-content">
          <KEmptyState
            v-if="store.list.length === 0"
            icon="⚡"
            title="No Nodes"
            description="Add nodes first to view their protocol cores."
          />
          <div v-else class="cores-grid">
            <div v-for="node in store.list" :key="node.id" class="core-node-section">
              <h4 class="core-node-title">{{ node.name }}</h4>
              <div class="protocol-cards">
                <div v-for="proto in protocols" :key="proto" class="protocol-card">
                  <span class="protocol-card__name">{{ proto }}</span>
                  <KStatusPill :status="getServiceStatus(node, proto)" size="sm" />
                </div>
              </div>
            </div>
          </div>
        </div>
      </template>
    </KTabs>
  </div>
</template>

<style scoped>
.nodes-view { display: flex; flex-direction: column; gap: var(--space-5); }
.page-header { display: flex; align-items: center; justify-content: space-between; }
.page-title { margin: 0; font-size: var(--text-xl); font-weight: var(--font-bold); }

.token-banner { padding: var(--space-3) var(--space-4); background: rgba(34, 211, 238, 0.08); border: 1px solid var(--color-accent); border-radius: var(--radius-lg); }
.token-banner code { background: var(--color-surface-2); padding: 2px 6px; border-radius: var(--radius-sm); font-size: var(--text-sm); word-break: break-all; }

.panel { padding: var(--space-4); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); }
.panel-title { margin: 0 0 var(--space-3); font-size: var(--text-sm); font-weight: var(--font-semibold); }
.node-form { display: flex; flex-direction: column; gap: var(--space-4); }
.form-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: var(--space-3); }
.form-actions { display: flex; justify-content: flex-end; gap: var(--space-2); }

.nodes-content, .cores-content { padding: var(--space-4) 0; }
.nodes-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: var(--space-4); }

.node-card { padding: var(--space-4); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); display: flex; flex-direction: column; gap: var(--space-3); }
.node-card__header { display: flex; flex-direction: column; gap: var(--space-1); }
.node-card__title { display: flex; align-items: center; justify-content: space-between; }
.node-card__name { margin: 0; font-size: var(--text-base); font-weight: var(--font-semibold); }
.node-card__ip { font-size: var(--text-xs); }

.node-card__metrics { display: flex; flex-direction: column; gap: var(--space-2); }
.metric-row { display: flex; align-items: center; gap: var(--space-2); }
.metric-row__label { font-size: var(--text-xs); color: var(--color-muted); width: 32px; }
.metric-row__bar { flex: 1; height: 6px; background: var(--color-border); border-radius: 3px; overflow: hidden; }
.metric-row__fill { height: 100%; background: var(--color-primary); border-radius: 3px; transition: width 0.3s ease; }
.metric-row__fill--accent { background: var(--color-accent); }
.metric-row__fill--warning { background: var(--color-warning); }
.metric-row__val { font-size: var(--text-xs); color: var(--color-muted); width: 36px; text-align: right; }
.metric-text { font-size: var(--text-xs); padding-top: var(--space-1); }

.node-card__actions { border-top: 1px solid var(--color-border); padding-top: var(--space-2); }

.cores-grid { display: flex; flex-direction: column; gap: var(--space-5); }
.core-node-section {}
.core-node-title { margin: 0 0 var(--space-2); font-size: var(--text-sm); font-weight: var(--font-semibold); }
.protocol-cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); gap: var(--space-2); }
.protocol-card { display: flex; justify-content: space-between; align-items: center; padding: var(--space-3); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-md); }
.protocol-card__name { font-size: var(--text-sm); font-weight: var(--font-medium); }

.text-muted { color: var(--color-muted); }
.text-sm { font-size: var(--text-sm); }
</style>
