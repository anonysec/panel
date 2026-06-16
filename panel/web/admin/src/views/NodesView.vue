<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useNodesStore } from '@/stores/nodes'
import { useToast } from '@koris/composables/useToast'
import { useConfirm } from '@koris/composables/useConfirm'
import KTabs from '@koris/ui/KTabs.vue'
import KButton from '@koris/ui/KButton.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'
import KSelect from '@koris/ui/KSelect.vue'

const store = useNodesStore()
const toast = useToast()
const { confirm } = useConfirm()
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


// ─── Protocol Defaults & Config State ────────────────────────────────────────
const PROTOCOL_DEFAULTS: Record<string, any> = {
  openvpn: { port: 1194, network: '10.8.0.0/24', enabled: true, mtu: 1500, max_clients: 0, enable_logs: true, conn_limit: 0, extra_json: { transport: 'udp', cipher: 'AES-256-GCM', tls_mode: 'tls-crypt', dns1: '8.8.8.8', dns2: '8.8.4.4' } },
  l2tp: { port: 1701, network: '10.9.0.0/24', enabled: true, mtu: 1500, max_clients: 0, enable_logs: true, conn_limit: 0, extra_json: { ipsec_mode: 'ipsec', psk: '', auth_method: 'CHAP', dns1: '8.8.8.8', dns2: '8.8.4.4' } },
  ikev2: { port: 500, network: '10.10.0.0/24', enabled: true, mtu: 1500, max_clients: 0, enable_logs: true, conn_limit: 0, extra_json: { auth_type: 'psk', psk: '', cert_id: '', dns1: '8.8.8.8', dns2: '8.8.4.4' } },
  ssh: { port: 2222, network: '', enabled: true, max_clients: 0, enable_logs: true, conn_limit: 0, extra_json: { listen_address: '0.0.0.0', key_type: 'ed25519' } },
}

const protocolList = ['openvpn', 'l2tp', 'ikev2', 'ssh'] as const
const protocolIcons: Record<string, string> = {
  openvpn: '🔐',
  l2tp: '🔒',
  ikev2: '🛡️',
  ssh: '🖥️',
}
const protocolLabels: Record<string, string> = {
  openvpn: 'OpenVPN',
  l2tp: 'L2TP',
  ikev2: 'IKEv2',
  ssh: 'SSH',
}

const editingConfig = ref<{ nodeId: number; protocol: string } | null>(null)
const configForm = ref<any>({})
const savingConfig = ref(false)


// ─── Helpers ─────────────────────────────────────────────────────────────────
function formatBps(bps: number): string {
  if (bps < 1000) return `${bps} bps`
  if (bps < 1000000) return `${(bps / 1000).toFixed(1)} Kbps`
  return `${(bps / 1000000).toFixed(1)} Mbps`
}

function getServiceStatus(node: any, protocol: string): string {
  const metrics = node.status_metrics
  if (!metrics) return 'unknown'
  if (protocol === 'openvpn') return metrics.openvpn_status || 'unknown'
  if (protocol === 'l2tp') return metrics.l2tp_status || 'unknown'
  if (protocol === 'ikev2') return metrics.ikev2_status || 'unknown'
  if (protocol === 'ssh') return metrics.ssh_status || 'unknown'
  return 'unknown'
}

function getNodeConfig(nodeId: number, protocol: string) {
  const configs = store.vpnConfigs[nodeId]
  if (!configs) return null
  return configs.find(c => c.protocol === protocol) || null
}

// ─── Node CRUD ───────────────────────────────────────────────────────────────
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

async function handleDeleteNode(id: number, name: string) {
  const confirmed = await confirm({
    title: 'Delete Node',
    message: `Are you sure you want to delete "${name}"? This will remove the node and all related configurations.`,
    variant: 'danger',
    icon: '⚠',
    confirmText: 'Delete',
    cancelText: 'Cancel',
  })
  if (!confirmed) return
  const success = await store.deleteNode(id)
  if (success) {
    toast.success(`Node "${name}" deleted successfully.`)
  } else {
    toast.error(`Failed to delete node "${name}".`)
  }
}


// ─── Protocol Config Handlers ────────────────────────────────────────────────
function startEdit(nodeId: number, protocol: string, currentConfig: any) {
  editingConfig.value = { nodeId, protocol }
  const defaults = PROTOCOL_DEFAULTS[protocol]
  if (currentConfig) {
    configForm.value = {
      protocol: currentConfig.protocol,
      port: currentConfig.port,
      network: currentConfig.network,
      enabled: currentConfig.enabled,
      mtu: currentConfig.mtu ?? defaults.mtu ?? null,
      max_clients: currentConfig.max_clients ?? defaults.max_clients ?? 0,
      enable_logs: currentConfig.enable_logs ?? defaults.enable_logs ?? true,
      conn_limit: currentConfig.conn_limit ?? defaults.conn_limit ?? 0,
      extra_json: { ...defaults.extra_json, ...(currentConfig.extra_json || {}) },
    }
  } else {
    configForm.value = {
      protocol,
      port: defaults.port,
      network: defaults.network,
      enabled: defaults.enabled,
      mtu: defaults.mtu ?? null,
      max_clients: defaults.max_clients ?? 0,
      enable_logs: defaults.enable_logs ?? true,
      conn_limit: defaults.conn_limit ?? 0,
      extra_json: { ...defaults.extra_json },
    }
  }
}

function cancelEdit() {
  editingConfig.value = null
  configForm.value = {}
}

async function saveConfig() {
  if (!editingConfig.value) return
  savingConfig.value = true
  const { nodeId } = editingConfig.value
  const payload = {
    protocol: configForm.value.protocol,
    port: configForm.value.port,
    network: configForm.value.network,
    enabled: configForm.value.enabled,
    mtu: configForm.value.mtu ?? undefined,
    max_clients: configForm.value.max_clients ?? 0,
    enable_logs: configForm.value.enable_logs ?? true,
    conn_limit: configForm.value.conn_limit ?? 0,
    extra_json: configForm.value.extra_json,
  }
  await store.saveNodeVpnConfig(nodeId, payload)
  await store.loadNodeVpnConfigs(nodeId)
  editingConfig.value = null
  configForm.value = {}
  savingConfig.value = false
  toast.success('Configuration saved')
}

async function toggleProtocol(nodeId: number, protocol: string, currentConfig: any, newEnabled: boolean) {
  const config = currentConfig
    ? { protocol: currentConfig.protocol, port: currentConfig.port, network: currentConfig.network, enabled: newEnabled, extra_json: currentConfig.extra_json }
    : { protocol, ...PROTOCOL_DEFAULTS[protocol], enabled: newEnabled }
  await store.saveNodeVpnConfig(nodeId, config)
  await store.loadNodeVpnConfigs(nodeId)
}

// ─── Load configs when Cores tab is activated ────────────────────────────────
watch(activeTab, (tab) => {
  if (tab === 'cores') {
    store.list.forEach(node => store.loadNodeVpnConfigs(node.id))
  }
})

onMounted(() => {
  store.loadNodes()
})
</script>


<template>
  <div class="page nodes-view">
    <header class="page-header">
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
                <KButton variant="danger" size="sm" @click="handleDeleteNode(node.id, node.name)">
                  Delete
                </KButton>
              </div>
            </div>
          </div>
        </div>
      </template>


      <!-- Cores Tab: Protocol Configuration -->
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
                <div
                  v-for="proto in protocolList"
                  :key="proto"
                  class="protocol-card"
                  :class="{ 'protocol-card--disabled': getNodeConfig(node.id, proto)?.enabled === false }"
                >
                  <!-- Protocol Summary Row -->
                  <div class="protocol-card__header">
                    <div class="protocol-card__info">
                      <span class="protocol-card__icon">{{ protocolIcons[proto] }}</span>
                      <span class="protocol-card__name">{{ protocolLabels[proto] }}</span>
                    </div>
                    <div class="protocol-card__meta">
                      <span class="protocol-card__port text-muted">
                        Port {{ getNodeConfig(node.id, proto)?.port ?? PROTOCOL_DEFAULTS[proto].port }}
                      </span>
                      <span v-if="getNodeConfig(node.id, proto)?.network || PROTOCOL_DEFAULTS[proto].network" class="protocol-card__network text-muted">
                        {{ getNodeConfig(node.id, proto)?.network || PROTOCOL_DEFAULTS[proto].network }}
                      </span>
                    </div>
                    <div class="protocol-card__controls">
                      <KStatusPill :status="getServiceStatus(node, proto)" size="sm" />
                      <label class="toggle-switch">
                        <input
                          type="checkbox"
                          :checked="getNodeConfig(node.id, proto)?.enabled ?? PROTOCOL_DEFAULTS[proto].enabled"
                          @change="toggleProtocol(node.id, proto, getNodeConfig(node.id, proto), ($event.target as HTMLInputElement).checked)"
                        />
                        <span class="toggle-switch__slider" />
                      </label>
                      <KButton variant="ghost" size="sm" @click="startEdit(node.id, proto, getNodeConfig(node.id, proto))">
                        Edit
                      </KButton>
                    </div>
                  </div>


                  <!-- Inline Edit Form -->
                  <div
                    v-if="editingConfig && editingConfig.nodeId === node.id && editingConfig.protocol === proto"
                    class="protocol-form"
                  >
                    <div class="protocol-form__grid">
                      <KFormField :name="`${proto}-port`" label="Port">
                        <template #default="{ fieldId }">
                          <KInput :id="fieldId" v-model="configForm.port" type="number" placeholder="Port" />
                        </template>
                      </KFormField>
                      <KFormField :name="`${proto}-network`" label="Network">
                        <template #default="{ fieldId }">
                          <KInput :id="fieldId" v-model="configForm.network" placeholder="10.8.0.0/24" />
                        </template>
                      </KFormField>

                      <!-- OpenVPN specific fields -->
                      <template v-if="proto === 'openvpn'">
                        <KFormField :name="`${proto}-transport`" label="Transport">
                          <template #default="{ fieldId }">
                            <KSelect :id="fieldId" v-model="configForm.extra_json.transport" :options="[{ label: 'UDP', value: 'udp' }, { label: 'TCP', value: 'tcp' }]" />
                          </template>
                        </KFormField>
                        <KFormField :name="`${proto}-cipher`" label="Cipher">
                          <template #default="{ fieldId }">
                            <KSelect :id="fieldId" v-model="configForm.extra_json.cipher" :options="[{ label: 'AES-256-GCM', value: 'AES-256-GCM' }, { label: 'AES-128-GCM', value: 'AES-128-GCM' }, { label: 'CHACHA20-POLY1305', value: 'CHACHA20-POLY1305' }]" />
                          </template>
                        </KFormField>
                        <KFormField :name="`${proto}-tls`" label="TLS Mode">
                          <template #default="{ fieldId }">
                            <KSelect :id="fieldId" v-model="configForm.extra_json.tls_mode" :options="[{ label: 'tls-crypt', value: 'tls-crypt' }, { label: 'tls-auth', value: 'tls-auth' }, { label: 'none', value: 'none' }]" />
                          </template>
                        </KFormField>
                        <KFormField :name="`${proto}-dns1`" label="DNS 1">
                          <template #default="{ fieldId }">
                            <KInput :id="fieldId" v-model="configForm.extra_json.dns1" placeholder="8.8.8.8" />
                          </template>
                        </KFormField>
                        <KFormField :name="`${proto}-dns2`" label="DNS 2">
                          <template #default="{ fieldId }">
                            <KInput :id="fieldId" v-model="configForm.extra_json.dns2" placeholder="8.8.4.4" />
                          </template>
                        </KFormField>
                      </template>


                      <!-- L2TP specific fields -->
                      <template v-if="proto === 'l2tp'">
                        <KFormField :name="`${proto}-ipsec`" label="Mode">
                          <template #default="{ fieldId }">
                            <KSelect :id="fieldId" v-model="configForm.extra_json.ipsec_mode" :options="[{ label: 'L2TP/IPSec', value: 'ipsec' }, { label: 'Plain L2TP', value: 'plain' }]" />
                          </template>
                        </KFormField>
                        <KFormField v-if="configForm.extra_json.ipsec_mode === 'ipsec'" :name="`${proto}-psk`" label="Pre-Shared Key">
                          <template #default="{ fieldId }">
                            <KInput :id="fieldId" v-model="configForm.extra_json.psk" type="password" placeholder="PSK" />
                          </template>
                        </KFormField>
                        <KFormField :name="`${proto}-auth`" label="Auth Method">
                          <template #default="{ fieldId }">
                            <KSelect :id="fieldId" v-model="configForm.extra_json.auth_method" :options="[{ label: 'CHAP', value: 'CHAP' }, { label: 'PAP', value: 'PAP' }, { label: 'MS-CHAPv2', value: 'MS-CHAPv2' }]" />
                          </template>
                        </KFormField>
                        <KFormField :name="`${proto}-dns1`" label="DNS 1">
                          <template #default="{ fieldId }">
                            <KInput :id="fieldId" v-model="configForm.extra_json.dns1" placeholder="8.8.8.8" />
                          </template>
                        </KFormField>
                        <KFormField :name="`${proto}-dns2`" label="DNS 2">
                          <template #default="{ fieldId }">
                            <KInput :id="fieldId" v-model="configForm.extra_json.dns2" placeholder="8.8.4.4" />
                          </template>
                        </KFormField>
                      </template>

                      <!-- IKEv2 specific fields -->
                      <template v-if="proto === 'ikev2'">
                        <KFormField :name="`${proto}-authtype`" label="Auth Type">
                          <template #default="{ fieldId }">
                            <KSelect :id="fieldId" v-model="configForm.extra_json.auth_type" :options="[{ label: 'PSK', value: 'psk' }, { label: 'Certificate', value: 'certificate' }]" />
                          </template>
                        </KFormField>
                        <KFormField v-if="configForm.extra_json.auth_type === 'psk'" :name="`${proto}-psk`" label="Pre-Shared Key">
                          <template #default="{ fieldId }">
                            <KInput :id="fieldId" v-model="configForm.extra_json.psk" type="password" placeholder="PSK" />
                          </template>
                        </KFormField>
                        <KFormField v-if="configForm.extra_json.auth_type === 'certificate'" :name="`${proto}-certid`" label="Certificate ID">
                          <template #default="{ fieldId }">
                            <KInput :id="fieldId" v-model="configForm.extra_json.cert_id" placeholder="Certificate identifier" />
                          </template>
                        </KFormField>
                        <KFormField :name="`${proto}-dns1`" label="DNS 1">
                          <template #default="{ fieldId }">
                            <KInput :id="fieldId" v-model="configForm.extra_json.dns1" placeholder="8.8.8.8" />
                          </template>
                        </KFormField>
                        <KFormField :name="`${proto}-dns2`" label="DNS 2">
                          <template #default="{ fieldId }">
                            <KInput :id="fieldId" v-model="configForm.extra_json.dns2" placeholder="8.8.4.4" />
                          </template>
                        </KFormField>
                      </template>

                      <!-- SSH specific fields -->
                      <template v-if="proto === 'ssh'">
                        <KFormField :name="`${proto}-listen`" label="Listen Address">
                          <template #default="{ fieldId }">
                            <KInput :id="fieldId" v-model="configForm.extra_json.listen_address" placeholder="0.0.0.0" />
                          </template>
                        </KFormField>
                        <KFormField :name="`${proto}-keytype`" label="Key Type">
                          <template #default="{ fieldId }">
                            <KSelect :id="fieldId" v-model="configForm.extra_json.key_type" :options="[{ label: 'ed25519', value: 'ed25519' }, { label: 'rsa', value: 'rsa' }, { label: 'ecdsa', value: 'ecdsa' }]" />
                          </template>
                        </KFormField>
                      </template>
                    </div>

                    <!-- Advanced Settings -->
                    <details class="advanced-settings">
                      <summary class="advanced-settings__title">Advanced Settings</summary>
                      <div class="protocol-form__grid advanced-settings__grid">
                        <!-- MTU: all protocols except SSH -->
                        <KFormField v-if="proto !== 'ssh'" :name="`${proto}-mtu`" label="MTU">
                          <template #default="{ fieldId }">
                            <KInput :id="fieldId" v-model="configForm.mtu" type="number" placeholder="1500" />
                          </template>
                        </KFormField>
                        <!-- Max Clients: all protocols -->
                        <KFormField :name="`${proto}-max-clients`" label="Max Clients (0 = unlimited)">
                          <template #default="{ fieldId }">
                            <KInput :id="fieldId" v-model="configForm.max_clients" type="number" placeholder="0" />
                          </template>
                        </KFormField>
                        <!-- Connection Limit: all protocols -->
                        <KFormField :name="`${proto}-conn-limit`" label="Conn Limit (0 = unlimited)">
                          <template #default="{ fieldId }">
                            <KInput :id="fieldId" v-model="configForm.conn_limit" type="number" placeholder="0" />
                          </template>
                        </KFormField>
                        <!-- Enable Logs: all protocols -->
                        <KFormField :name="`${proto}-enable-logs`" label="Enable Logs">
                          <template #default>
                            <label class="toggle-switch">
                              <input
                                type="checkbox"
                                :checked="configForm.enable_logs"
                                @change="configForm.enable_logs = ($event.target as HTMLInputElement).checked"
                              />
                              <span class="toggle-switch__slider" />
                            </label>
                          </template>
                        </KFormField>
                      </div>
                    </details>

                    <div class="protocol-form__actions">
                      <KButton variant="ghost" size="sm" @click="cancelEdit">Cancel</KButton>
                      <KButton variant="primary" size="sm" :loading="savingConfig" @click="saveConfig">Save</KButton>
                    </div>
                  </div>
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
.page-header { display: flex; align-items: center; justify-content: flex-end; }

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

.node-card__actions { border-top: 1px solid var(--color-border); padding-top: var(--space-2); display: flex; gap: var(--space-2); }

/* ─── Cores / Protocol Cards ─────────────────────────────────────────────── */
.cores-grid { display: flex; flex-direction: column; gap: var(--space-5); }
.core-node-title { margin: 0 0 var(--space-3); font-size: var(--text-base); font-weight: var(--font-semibold); }
.protocol-cards { display: flex; flex-direction: column; gap: var(--space-3); }

.protocol-card {
  padding: var(--space-4);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  transition: opacity 0.2s ease;
}
.protocol-card--disabled { opacity: 0.5; }


.protocol-card__header {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  flex-wrap: wrap;
}
.protocol-card__info {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  min-width: 120px;
}
.protocol-card__icon { font-size: var(--text-lg); }
.protocol-card__name { font-size: var(--text-sm); font-weight: var(--font-semibold); }
.protocol-card__meta {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  flex: 1;
}
.protocol-card__port { font-size: var(--text-xs); }
.protocol-card__network { font-size: var(--text-xs); }
.protocol-card__controls {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin-left: auto;
}

/* Toggle Switch */
.toggle-switch { position: relative; display: inline-block; width: 36px; height: 20px; cursor: pointer; }
.toggle-switch input { opacity: 0; width: 0; height: 0; }
.toggle-switch__slider {
  position: absolute; top: 0; left: 0; right: 0; bottom: 0;
  background: var(--color-border);
  border-radius: 10px;
  transition: background 0.2s ease;
}
.toggle-switch__slider::before {
  content: '';
  position: absolute; top: 2px; left: 2px;
  width: 16px; height: 16px;
  background: white;
  border-radius: 50%;
  transition: transform 0.2s ease;
}
.toggle-switch input:checked + .toggle-switch__slider { background: var(--color-primary); }
.toggle-switch input:checked + .toggle-switch__slider::before { transform: translateX(16px); }


/* Protocol Form */
.protocol-form {
  margin-top: var(--space-4);
  padding-top: var(--space-4);
  border-top: 1px solid var(--color-border);
}
.protocol-form__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: var(--space-3);
}
.protocol-form__actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--space-2);
  margin-top: var(--space-4);
}

/* Advanced Settings */
.advanced-settings {
  margin-top: var(--space-4);
  border-top: 1px solid var(--color-border);
  padding-top: var(--space-3);
}
.advanced-settings__title {
  font-size: var(--text-sm);
  font-weight: var(--font-semibold);
  color: var(--color-muted);
  cursor: pointer;
  user-select: none;
  padding: var(--space-1) 0;
}
.advanced-settings__grid {
  margin-top: var(--space-3);
}

.text-muted { color: var(--color-muted); }
.text-sm { font-size: var(--text-sm); }
</style>
