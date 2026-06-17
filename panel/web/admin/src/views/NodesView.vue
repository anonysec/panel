<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useNodesStore } from '@/stores/nodes'
import { useToast } from '@koris/composables/useToast'
import { useConfirm } from '@koris/composables/useConfirm'
import { useI18n } from '@koris/composables/useI18n'
import KTabs from '@koris/ui/KTabs.vue'
import KButton from '@koris/ui/KButton.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'
import KSelect from '@koris/ui/KSelect.vue'
import KTextarea from '@koris/ui/KTextarea.vue'

const { t } = useI18n()
const store = useNodesStore()
const toast = useToast()
const { confirm } = useConfirm()
const activeTab = ref('nodes')
const showAddForm = ref(false)
const creating = ref(false)
const newToken = ref<string | null>(null)

// ─── Edit Node State ─────────────────────────────────────────────────────────
const editingNodeId = ref<number | null>(null)
const editNodeForm = ref({ name: '', public_ip: '', domain: '' })
const savingNode = ref(false)

const tabs = computed(() => [
  { key: 'nodes', label: t('nodes.nodes') },
  { key: 'cores', label: t('nodes.cores') },
])

const nodeForm = ref({
  name: '',
  public_ip: '',
  domain: '',
})


// ─── Protocol Defaults & Config State ────────────────────────────────────────
const PROTOCOL_DEFAULTS: Record<string, any> = {
  openvpn: {
    port: 1194, network: '10.8.0.0/24', enabled: true, mtu: 1500, max_clients: 0, enable_logs: true, conn_limit: 0,
    extra_json: {
      transport: 'udp', cipher: 'AES-256-GCM', tls_mode: 'tls-crypt', dns1: '8.8.8.8', dns2: '8.8.4.4',
      comp_lzo: false, push_routes: '', fragment: 0, mssfix: 0, keepalive: '10 120', topology: 'subnet', verb: 3, custom_directives: '',
      outbound: { enabled: false, type: 'vless', address: '', uuid: '', tls: true, path: '', sni: '' },
    },
  },
  l2tp: {
    port: 1701, network: '10.9.0.0/24', enabled: true, mtu: 1500, max_clients: 0, enable_logs: true, conn_limit: 0,
    extra_json: {
      ipsec_mode: 'ipsec', psk: '', auth_method: 'CHAP', dns1: '8.8.8.8', dns2: '8.8.4.4',
      refuse_chap: false, refuse_pap: true, lcp_echo_interval: 30, lcp_echo_failure: 4, idle_timeout: 0, require_mschap_v2: true,
      outbound: { enabled: false, type: 'vless', address: '', uuid: '', tls: true, path: '', sni: '' },
    },
  },
  ikev2: {
    port: 500, network: '10.10.0.0/24', enabled: true, mtu: 1500, max_clients: 0, enable_logs: true, conn_limit: 0,
    extra_json: {
      auth_type: 'psk', psk: '', cert_id: '', dns1: '8.8.8.8', dns2: '8.8.4.4',
      dpd_interval: 30, dpd_timeout: 150, rekey_time: '4h', ike_proposals: 'aes256-sha256-modp2048', esp_proposals: 'aes256-sha256', left_id: '', right_id: '%any', fragment_size: 0,
      outbound: { enabled: false, type: 'vless', address: '', uuid: '', tls: true, path: '', sni: '' },
    },
  },
  ssh: {
    port: 2222, network: '', enabled: true, max_clients: 0, enable_logs: true, conn_limit: 0,
    extra_json: {
      listen_address: '0.0.0.0', key_type: 'ed25519',
      max_sessions: 10, idle_timeout: 0, shell_access: false, allowed_keys: '',
      outbound: { enabled: false, type: 'vless', address: '', uuid: '', tls: true, path: '', sni: '' },
    },
  },
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
  // Check node_services array first (SSH is only stored there)
  if (node.services && Array.isArray(node.services)) {
    const svc = node.services.find((s: any) => s.service === protocol)
    if (svc && svc.status) return svc.status
  }
  // Fall back to status_metrics for openvpn/l2tp/ikev2
  if (!metrics) return 'unknown'
  if (protocol === 'openvpn') return metrics.openvpn_status || 'unknown'
  if (protocol === 'l2tp') return metrics.l2tp_status || 'unknown'
  if (protocol === 'ikev2') return metrics.ikev2_status || 'unknown'
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
    title: t('nodes.confirm_delete_title'),
    message: t('nodes.confirm_delete_msg').replace('{name}', name),
    variant: 'danger',
    icon: '⚠',
    confirmText: t('btn.delete'),
    cancelText: t('btn.cancel'),
  })
  if (!confirmed) return
  const success = await store.deleteNode(id)
  if (success) {
    toast.success(t('nodes.deleted_success').replace('{name}', name))
  } else {
    toast.error(t('nodes.deleted_error').replace('{name}', name))
  }
}

// ─── Edit Node Handlers ──────────────────────────────────────────────────────
function startEditNode(node: any) {
  editingNodeId.value = node.id
  editNodeForm.value = {
    name: node.name || '',
    public_ip: node.public_ip || '',
    domain: node.domain || '',
  }
}

function cancelEditNode() {
  editingNodeId.value = null
  editNodeForm.value = { name: '', public_ip: '', domain: '' }
}

async function handleEditNode() {
  if (!editingNodeId.value) return
  savingNode.value = true
  // Build delta payload: only include fields that actually changed
  const originalNode = store.list.find((n: any) => n.id === editingNodeId.value)
  const payload: Record<string, string> = {}
  if (editNodeForm.value.name !== (originalNode?.name || '')) {
    payload.name = editNodeForm.value.name
  }
  if (editNodeForm.value.public_ip !== (originalNode?.public_ip || '')) {
    payload.public_ip = editNodeForm.value.public_ip
  }
  if (editNodeForm.value.domain !== (originalNode?.domain || '')) {
    payload.domain = editNodeForm.value.domain
  }
  // If nothing changed, just close the form
  if (Object.keys(payload).length === 0) {
    savingNode.value = false
    cancelEditNode()
    return
  }
  const success = await store.editNode(editingNodeId.value, payload)
  savingNode.value = false
  if (success) {
    toast.success(t('nodes.edit_success'))
    cancelEditNode()
  } else {
    toast.error(t('nodes.edit_error'))
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

// ─── Validation Helpers ──────────────────────────────────────────────────────
function isPortValid(port: number | string | null | undefined): boolean {
  const num = Number(port)
  return Number.isInteger(num) && num >= 1 && num <= 65535
}

function isCidrValid(cidr: string | null | undefined): boolean {
  if (!cidr) return true // empty is ok for SSH
  const match = cidr.match(/^(\d{1,3}\.){3}\d{1,3}\/(\d{1,2})$/)
  if (!match) return false
  const parts = cidr.split('/')[0].split('.')
  const mask = Number(cidr.split('/')[1])
  return parts.every(p => Number(p) >= 0 && Number(p) <= 255) && mask >= 0 && mask <= 32
}

// ─── Config Preview ──────────────────────────────────────────────────────────
const showConfigPreview = ref(false)

function getConfigPreview(): string[] {
  if (!editingConfig.value) return []
  const lines: string[] = []
  const f = configForm.value
  lines.push(`protocol=${f.protocol}`)
  lines.push(`port=${f.port}`)
  if (f.network) lines.push(`network=${f.network}`)
  lines.push(`enabled=${f.enabled}`)
  if (f.mtu) lines.push(`mtu=${f.mtu}`)
  if (f.max_clients) lines.push(`max_clients=${f.max_clients}`)
  if (f.conn_limit) lines.push(`conn_limit=${f.conn_limit}`)
  lines.push(`enable_logs=${f.enable_logs}`)
  if (f.extra_json) {
    for (const [key, val] of Object.entries(f.extra_json)) {
      if (val !== '' && val !== null && val !== undefined) {
        lines.push(`${key}=${val}`)
      }
    }
  }
  return lines
}

// ─── Chip Array Helpers ──────────────────────────────────────────────────────
const newRouteInput = ref('')
const newKeyInput = ref('')

function getPushRoutesArray(): string[] {
  const raw = configForm.value?.extra_json?.push_routes || ''
  if (!raw) return []
  return raw.split(',').map((r: string) => r.trim()).filter(Boolean)
}

function addPushRoute() {
  const val = newRouteInput.value.trim()
  if (!val) return
  const current = getPushRoutesArray()
  if (!current.includes(val)) {
    current.push(val)
    configForm.value.extra_json.push_routes = current.join(', ')
  }
  newRouteInput.value = ''
}

function removePushRoute(index: number) {
  const current = getPushRoutesArray()
  current.splice(index, 1)
  configForm.value.extra_json.push_routes = current.join(', ')
}

function getAllowedKeysArray(): string[] {
  const raw = configForm.value?.extra_json?.allowed_keys || ''
  if (!raw) return []
  return raw.split('\n').map((k: string) => k.trim()).filter(Boolean)
}

function addAllowedKey() {
  const val = newKeyInput.value.trim()
  if (!val) return
  const current = getAllowedKeysArray()
  if (!current.includes(val)) {
    current.push(val)
    configForm.value.extra_json.allowed_keys = current.join('\n')
  }
  newKeyInput.value = ''
}

function removeAllowedKey(index: number) {
  const current = getAllowedKeysArray()
  current.splice(index, 1)
  configForm.value.extra_json.allowed_keys = current.join('\n')
}

// ─── Proxy Helpers ────────────────────────────────────────────────────────
function initOutbound() {
  if (!configForm.value.extra_json.outbound) {
    configForm.value.extra_json.outbound = { enabled: false, type: 'vless', address: '', uuid: '', tls: true, path: '', sni: '' }
  }
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

// ─── Reset to Defaults ───────────────────────────────────────────────────────
function resetToDefaults() {
  if (!editingConfig.value) return
  const proto = editingConfig.value.protocol
  const defaults = PROTOCOL_DEFAULTS[proto]
  if (!defaults) {
    toast.error(t('nodes.no_defaults'))
    return
  }
  configForm.value = {
    protocol: proto,
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

// ─── Restart Service ─────────────────────────────────────────────────────────
const restartingService = ref<string | null>(null)

async function restartService(nodeId: number, protocol: string) {
  restartingService.value = `${nodeId}-${protocol}`
  try {
    const success = await store.createNodeTask({
      node_id: nodeId,
      action: 'restart_service',
      payload_json: { protocol },
    })
    if (success) {
      toast.success(t('nodes.restart_success'))
    } else {
      toast.error(t('nodes.restart_error'))
    }
  } catch {
    toast.error(t('nodes.restart_error'))
  } finally {
    restartingService.value = null
  }
}

// ─── Danger field detection ──────────────────────────────────────────────────
function isDangerField(proto: string, fieldName: string): boolean {
  if (proto === 'ssh' && fieldName === 'shell_access') return true
  if (proto === 'openvpn' && fieldName === 'comp_lzo') return true
  if (proto === 'openvpn' && fieldName === 'tls_mode' && configForm.value?.extra_json?.tls_mode === 'none') return true
  if (proto === 'l2tp' && fieldName === 'ipsec_mode' && configForm.value?.extra_json?.ipsec_mode === 'plain') return true
  return false
}

// ─── Clear PSK when mode switches away from PSK-based auth ──────────────────
watch(() => configForm.value?.extra_json?.ipsec_mode, (newMode, oldMode) => {
  if (oldMode === 'ipsec' && newMode === 'plain' && configForm.value?.extra_json) {
    configForm.value.extra_json.psk = ''
  }
})

watch(() => configForm.value?.extra_json?.auth_type, (newType, oldType) => {
  if (oldType === 'psk' && newType === 'certificate' && configForm.value?.extra_json) {
    configForm.value.extra_json.psk = ''
  }
})

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
      <KButton variant="primary" icon="+" @click="showAddForm = true">{{ t('nodes.add_node') }}</KButton>
    </header>

    <!-- New Token Display -->
    <div v-if="newToken" class="token-banner">
      <p><strong>{{ t('nodes.node_token') }}:</strong> <code>{{ newToken }}</code></p>
      <p class="text-muted text-sm">{{ t('nodes.token_save_warning') }}</p>
      <KButton variant="ghost" size="sm" @click="newToken = null">{{ t('nodes.dismiss') }}</KButton>
    </div>

    <!-- Add Node Form -->
    <div v-if="showAddForm" class="panel">
      <h4 class="panel-title">{{ t('nodes.add_node') }}</h4>
      <form class="node-form" @submit.prevent="handleCreateNode">
        <div class="form-grid">
          <KFormField name="node-name" :label="t('nodes.node_name')" required>
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="nodeForm.name" placeholder="node-us-1" />
            </template>
          </KFormField>
          <KFormField name="node-ip" :label="t('nodes.public_ip')" required>
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="nodeForm.public_ip" placeholder="1.2.3.4" />
            </template>
          </KFormField>
          <KFormField name="node-domain" :label="t('nodes.domain')" required>
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="nodeForm.domain" placeholder="us1.example.com" />
            </template>
          </KFormField>
        </div>
        <div class="form-actions">
          <KButton variant="ghost" @click="showAddForm = false">{{ t('btn.cancel') }}</KButton>
          <KButton type="submit" variant="primary" :loading="creating">{{ t('nodes.create_node') }}</KButton>
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
            :title="t('nodes.no_nodes')"
            :description="t('nodes.no_nodes_desc')"
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
                <KButton variant="ghost" size="sm" @click="startEditNode(node)">
                  {{ t('btn.edit') }}
                </KButton>
                <KButton variant="ghost" size="sm" @click="toggleNode(node.id, node.status)">
                  {{ node.status === 'online' ? t('btn.disable') : t('btn.enable') }}
                </KButton>
                <KButton variant="danger" size="sm" @click="handleDeleteNode(node.id, node.name)">
                  {{ t('btn.delete') }}
                </KButton>
              </div>
              <!-- Inline Edit Node Form -->
              <div v-if="editingNodeId === node.id" class="node-edit-form">
                <form @submit.prevent="handleEditNode">
                  <div class="form-grid">
                    <KFormField name="edit-name" :label="t('nodes.node_name')" required>
                      <template #default="{ fieldId }">
                        <KInput :id="fieldId" v-model="editNodeForm.name" placeholder="node-us-1" />
                      </template>
                    </KFormField>
                    <KFormField name="edit-ip" :label="t('nodes.public_ip')" required>
                      <template #default="{ fieldId }">
                        <KInput :id="fieldId" v-model="editNodeForm.public_ip" placeholder="1.2.3.4" />
                      </template>
                    </KFormField>
                    <KFormField name="edit-domain" :label="t('nodes.domain')" required>
                      <template #default="{ fieldId }">
                        <KInput :id="fieldId" v-model="editNodeForm.domain" placeholder="us1.example.com" />
                      </template>
                    </KFormField>
                  </div>
                  <div class="form-actions">
                    <KButton variant="ghost" size="sm" @click="cancelEditNode">{{ t('btn.cancel') }}</KButton>
                    <KButton type="submit" variant="primary" size="sm" :loading="savingNode">{{ t('btn.save') }}</KButton>
                  </div>
                </form>
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
            :title="t('nodes.no_nodes')"
            :description="t('nodes.no_nodes_cores')"
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
                        {{ t('btn.edit') }}
                      </KButton>
                      <KButton
                        variant="ghost"
                        size="sm"
                        :loading="restartingService === `${node.id}-${proto}`"
                        @click="restartService(node.id, proto)"
                      >
                        {{ t('nodes.restart_service') }}
                      </KButton>
                    </div>
                  </div>


                  <!-- Inline Edit Form -->
                  <div
                    v-if="editingConfig && editingConfig.nodeId === node.id && editingConfig.protocol === proto"
                    class="protocol-form"
                  >
                    <!-- Networking Group -->
                    <div class="form-group">
                      <h5 class="form-group__title">{{ t('nodes.group_networking') }}</h5>
                      <div class="protocol-form__grid">
                        <KFormField :name="`${proto}-port`" :label="t('label.port')" :hint="t('nodes.hint_port')" :class="{ 'field--invalid': configForm.port && !isPortValid(configForm.port) }">
                          <template #default="{ fieldId }">
                            <KInput :id="fieldId" v-model="configForm.port" type="number" placeholder="Port" />
                            <span v-if="configForm.port && !isPortValid(configForm.port)" class="validation-msg">{{ t('nodes.validation_port') }}</span>
                          </template>
                        </KFormField>
                        <KFormField :name="`${proto}-network`" :label="t('label.network')" :hint="t('nodes.hint_network')" :class="{ 'field--invalid': configForm.network && !isCidrValid(configForm.network) }">
                          <template #default="{ fieldId }">
                            <KInput :id="fieldId" v-model="configForm.network" placeholder="10.8.0.0/24" />
                            <span v-if="configForm.network && !isCidrValid(configForm.network)" class="validation-msg">{{ t('nodes.validation_cidr') }}</span>
                          </template>
                        </KFormField>
                        <KFormField v-if="proto !== 'ssh'" :name="`${proto}-mtu`" :label="t('nodes.mtu')">
                          <template #default="{ fieldId }">
                            <KInput :id="fieldId" v-model="configForm.mtu" type="number" placeholder="1500" />
                          </template>
                        </KFormField>

                        <!-- OpenVPN networking -->
                        <template v-if="proto === 'openvpn'">
                          <KFormField :name="`${proto}-transport`" :label="t('nodes.transport')" :hint="t('nodes.hint_transport')">
                            <template #default="{ fieldId }">
                              <KSelect :id="fieldId" v-model="configForm.extra_json.transport" :options="[{ label: 'UDP', value: 'udp' }, { label: 'TCP', value: 'tcp' }]" />
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-topology`" :label="t('nodes.topology')">
                            <template #default="{ fieldId }">
                              <KSelect :id="fieldId" v-model="configForm.extra_json.topology" :options="[{ label: 'subnet', value: 'subnet' }, { label: 'net30', value: 'net30' }, { label: 'p2p', value: 'p2p' }]" />
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-dns1`" :label="t('nodes.dns1')" :hint="t('nodes.hint_dns')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.dns1" placeholder="8.8.8.8" />
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-dns2`" :label="t('nodes.dns2')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.dns2" placeholder="8.8.4.4" />
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-push-routes`" :label="t('nodes.push_routes')" class="form-group__full-width">
                            <template #default>
                              <div class="chip-field">
                                <div class="chip-list">
                                  <span v-for="(route, idx) in getPushRoutesArray()" :key="idx" class="chip">
                                    {{ route }}
                                    <button type="button" class="chip__remove" @click="removePushRoute(idx)">&times;</button>
                                  </span>
                                </div>
                                <div class="chip-input-row">
                                  <KInput v-model="newRouteInput" :placeholder="t('nodes.add_route_placeholder')" @keydown.enter.prevent="addPushRoute" />
                                  <KButton variant="ghost" size="sm" type="button" @click="addPushRoute">{{ t('nodes.add') }}</KButton>
                                </div>
                              </div>
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-fragment`" :label="t('nodes.fragment')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.fragment" type="number" placeholder="0" />
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-mssfix`" :label="t('nodes.mssfix')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.mssfix" type="number" placeholder="0" />
                            </template>
                          </KFormField>
                        </template>


                        <!-- L2TP networking -->
                        <template v-if="proto === 'l2tp'">
                          <KFormField :name="`${proto}-dns1`" :label="t('nodes.dns1')" :hint="t('nodes.hint_dns')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.dns1" placeholder="8.8.8.8" />
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-dns2`" :label="t('nodes.dns2')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.dns2" placeholder="8.8.4.4" />
                            </template>
                          </KFormField>
                        </template>

                        <!-- IKEv2 networking -->
                        <template v-if="proto === 'ikev2'">
                          <KFormField :name="`${proto}-dns1`" :label="t('nodes.dns1')" :hint="t('nodes.hint_dns')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.dns1" placeholder="8.8.8.8" />
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-dns2`" :label="t('nodes.dns2')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.dns2" placeholder="8.8.4.4" />
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-fragment-size`" :label="t('nodes.fragment_size')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.fragment_size" type="number" placeholder="0" />
                            </template>
                          </KFormField>
                        </template>

                        <!-- SSH networking -->
                        <template v-if="proto === 'ssh'">
                          <KFormField :name="`${proto}-listen`" :label="t('nodes.listen_address')" :hint="t('nodes.hint_listen_address')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.listen_address" placeholder="0.0.0.0" />
                            </template>
                          </KFormField>
                        </template>
                      </div>
                    </div>

                    <!-- Security Group -->
                    <div class="form-group">
                      <h5 class="form-group__title">{{ t('nodes.group_security') }}</h5>
                      <div class="protocol-form__grid">
                        <template v-if="proto === 'openvpn'">
                          <KFormField :name="`${proto}-cipher`" :label="t('nodes.cipher')" :hint="t('nodes.hint_cipher')">
                            <template #default="{ fieldId }">
                              <KSelect :id="fieldId" v-model="configForm.extra_json.cipher" :options="[{ label: 'AES-256-GCM', value: 'AES-256-GCM' }, { label: 'AES-128-GCM', value: 'AES-128-GCM' }, { label: 'CHACHA20-POLY1305', value: 'CHACHA20-POLY1305' }]" />
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-tls`" :label="t('nodes.tls_mode')" :hint="t('nodes.hint_tls_mode')" :class="{ 'field--danger': isDangerField(proto, 'tls_mode') }">
                            <template #default="{ fieldId }">
                              <KSelect :id="fieldId" v-model="configForm.extra_json.tls_mode" :options="[{ label: 'tls-crypt', value: 'tls-crypt' }, { label: 'tls-auth', value: 'tls-auth' }, { label: 'none', value: 'none' }]" />
                            </template>
                          </KFormField>
                        </template>

                        <template v-if="proto === 'l2tp'">
                          <KFormField :name="`${proto}-ipsec`" :label="t('nodes.mode')" :hint="t('nodes.hint_ipsec_mode')" :class="{ 'field--danger': isDangerField(proto, 'ipsec_mode') }">
                            <template #default="{ fieldId }">
                              <KSelect :id="fieldId" v-model="configForm.extra_json.ipsec_mode" :options="[{ label: 'L2TP/IPSec', value: 'ipsec' }, { label: 'Plain L2TP', value: 'plain' }]" />
                            </template>
                          </KFormField>
                          <KFormField v-if="configForm.extra_json.ipsec_mode === 'ipsec'" :name="`${proto}-psk`" :label="t('nodes.psk')" :hint="t('nodes.hint_psk')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.psk" type="password" placeholder="PSK" />
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-auth`" :label="t('nodes.auth_method')" :hint="t('nodes.hint_auth_method')">
                            <template #default="{ fieldId }">
                              <KSelect :id="fieldId" v-model="configForm.extra_json.auth_method" :options="[{ label: 'CHAP', value: 'CHAP' }, { label: 'PAP', value: 'PAP' }, { label: 'MS-CHAPv2', value: 'MS-CHAPv2' }]" />
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-refuse-chap`" :label="t('nodes.refuse_chap')">
                            <template #default>
                              <label class="toggle-switch">
                                <input type="checkbox" :checked="configForm.extra_json.refuse_chap" @change="configForm.extra_json.refuse_chap = ($event.target as HTMLInputElement).checked" />
                                <span class="toggle-switch__slider" />
                              </label>
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-refuse-pap`" :label="t('nodes.refuse_pap')">
                            <template #default>
                              <label class="toggle-switch">
                                <input type="checkbox" :checked="configForm.extra_json.refuse_pap" @change="configForm.extra_json.refuse_pap = ($event.target as HTMLInputElement).checked" />
                                <span class="toggle-switch__slider" />
                              </label>
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-require-mschap-v2`" :label="t('nodes.require_mschapv2')">
                            <template #default>
                              <label class="toggle-switch">
                                <input type="checkbox" :checked="configForm.extra_json.require_mschap_v2" @change="configForm.extra_json.require_mschap_v2 = ($event.target as HTMLInputElement).checked" />
                                <span class="toggle-switch__slider" />
                              </label>
                            </template>
                          </KFormField>
                        </template>

                        <template v-if="proto === 'ikev2'">
                          <KFormField :name="`${proto}-authtype`" :label="t('nodes.auth_type')" :hint="t('nodes.hint_auth_type')">
                            <template #default="{ fieldId }">
                              <KSelect :id="fieldId" v-model="configForm.extra_json.auth_type" :options="[{ label: 'PSK', value: 'psk' }, { label: 'Certificate', value: 'certificate' }]" />
                            </template>
                          </KFormField>
                          <KFormField v-if="configForm.extra_json.auth_type === 'psk'" :name="`${proto}-psk`" :label="t('nodes.psk')" :hint="t('nodes.hint_psk')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.psk" type="password" placeholder="PSK" />
                            </template>
                          </KFormField>
                          <KFormField v-if="configForm.extra_json.auth_type === 'certificate'" :name="`${proto}-certid`" :label="t('nodes.cert_id')" :hint="t('nodes.hint_cert_id')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.cert_id" placeholder="Certificate identifier" />
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-ike-proposals`" :label="t('nodes.ike_proposals')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.ike_proposals" placeholder="aes256-sha256-modp2048" />
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-esp-proposals`" :label="t('nodes.esp_proposals')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.esp_proposals" placeholder="aes256-sha256" />
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-left-id`" :label="t('nodes.left_id')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.left_id" placeholder="Server identity" />
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-right-id`" :label="t('nodes.right_id')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.right_id" placeholder="%any" />
                            </template>
                          </KFormField>
                        </template>

                        <template v-if="proto === 'ssh'">
                          <KFormField :name="`${proto}-keytype`" :label="t('nodes.key_type')" :hint="t('nodes.hint_key_type')">
                            <template #default="{ fieldId }">
                              <KSelect :id="fieldId" v-model="configForm.extra_json.key_type" :options="[{ label: 'ed25519', value: 'ed25519' }, { label: 'rsa', value: 'rsa' }, { label: 'ecdsa', value: 'ecdsa' }]" />
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-shell-access`" :label="t('nodes.shell_access')" :class="{ 'field--danger': isDangerField(proto, 'shell_access') }">
                            <template #default>
                              <label class="toggle-switch">
                                <input type="checkbox" :checked="configForm.extra_json.shell_access" @change="configForm.extra_json.shell_access = ($event.target as HTMLInputElement).checked" />
                                <span class="toggle-switch__slider" />
                              </label>
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-allowed-keys`" :label="t('nodes.allowed_keys')" class="form-group__full-width">
                            <template #default>
                              <div class="chip-field">
                                <div class="chip-list">
                                  <span v-for="(key, idx) in getAllowedKeysArray()" :key="idx" class="chip chip--key">
                                    {{ key.length > 40 ? key.slice(0, 40) + '...' : key }}
                                    <button type="button" class="chip__remove" @click="removeAllowedKey(idx)">&times;</button>
                                  </span>
                                </div>
                                <div class="chip-input-row">
                                  <KInput v-model="newKeyInput" :placeholder="t('nodes.add_key_placeholder')" @keydown.enter.prevent="addAllowedKey" />
                                  <KButton variant="ghost" size="sm" type="button" @click="addAllowedKey">{{ t('nodes.add') }}</KButton>
                                </div>
                              </div>
                            </template>
                          </KFormField>
                        </template>
                      </div>
                    </div>

                    <!-- Performance Group -->
                    <div class="form-group">
                      <h5 class="form-group__title">{{ t('nodes.group_performance') }}</h5>
                      <div class="protocol-form__grid">
                        <KFormField :name="`${proto}-max-clients`" :label="t('nodes.max_clients')">
                          <template #default="{ fieldId }">
                            <KInput :id="fieldId" v-model="configForm.max_clients" type="number" placeholder="0" />
                          </template>
                        </KFormField>
                        <KFormField :name="`${proto}-conn-limit`" :label="t('nodes.conn_limit')">
                          <template #default="{ fieldId }">
                            <KInput :id="fieldId" v-model="configForm.conn_limit" type="number" placeholder="0" />
                          </template>
                        </KFormField>
                        <template v-if="proto === 'openvpn'">
                          <KFormField :name="`${proto}-keepalive`" :label="t('nodes.keepalive')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.keepalive" placeholder="10 120" />
                            </template>
                          </KFormField>
                        </template>
                        <template v-if="proto === 'l2tp'">
                          <KFormField :name="`${proto}-lcp-echo-interval`" :label="t('nodes.lcp_echo_interval')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.lcp_echo_interval" type="number" placeholder="30" />
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-lcp-echo-failure`" :label="t('nodes.lcp_echo_failure')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.lcp_echo_failure" type="number" placeholder="4" />
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-idle-timeout`" :label="t('nodes.idle_timeout')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.idle_timeout" type="number" placeholder="0" />
                            </template>
                          </KFormField>
                        </template>
                        <template v-if="proto === 'ikev2'">
                          <KFormField :name="`${proto}-dpd-interval`" :label="t('nodes.dpd_interval')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.dpd_interval" type="number" placeholder="30" />
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-dpd-timeout`" :label="t('nodes.dpd_timeout')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.dpd_timeout" type="number" placeholder="150" />
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-rekey-time`" :label="t('nodes.rekey_time')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.rekey_time" placeholder="4h" />
                            </template>
                          </KFormField>
                        </template>
                        <template v-if="proto === 'ssh'">
                          <KFormField :name="`${proto}-max-sessions`" :label="t('nodes.max_sessions')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.max_sessions" type="number" placeholder="10" />
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-idle-timeout`" :label="t('nodes.idle_timeout')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.idle_timeout" type="number" placeholder="0" />
                            </template>
                          </KFormField>
                        </template>
                      </div>
                    </div>

                    <!-- Logging & Advanced Group -->
                    <div class="form-group">
                      <h5 class="form-group__title">{{ t('nodes.group_logging') }}</h5>
                      <div class="protocol-form__grid">
                        <KFormField :name="`${proto}-enable-logs`" :label="t('nodes.enable_logs')">
                          <template #default>
                            <label class="toggle-switch">
                              <input type="checkbox" :checked="configForm.enable_logs" @change="configForm.enable_logs = ($event.target as HTMLInputElement).checked" />
                              <span class="toggle-switch__slider" />
                            </label>
                          </template>
                        </KFormField>
                        <template v-if="proto === 'openvpn'">
                          <KFormField :name="`${proto}-verb`" :label="t('nodes.verbosity')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.verb" type="number" placeholder="3" />
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-comp-lzo`" :label="t('nodes.comp_lzo')" :class="{ 'field--danger': isDangerField(proto, 'comp_lzo') }">
                            <template #default>
                              <label class="toggle-switch">
                                <input type="checkbox" :checked="configForm.extra_json.comp_lzo" @change="configForm.extra_json.comp_lzo = ($event.target as HTMLInputElement).checked" />
                                <span class="toggle-switch__slider" />
                              </label>
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-custom-directives`" :label="t('nodes.custom_directives')" class="form-group__full-width">
                            <template #default="{ fieldId }">
                              <KTextarea :id="fieldId" v-model="configForm.extra_json.custom_directives" :rows="3" placeholder="One directive per line" />
                            </template>
                          </KFormField>
                        </template>
                      </div>
                    </div>

                    <!-- Proxy Group -->
                    <div class="form-group">
                      <h5 class="form-group__title">{{ t('nodes.outbound') }}</h5>
                      <p class="form-group__desc">{{ t('nodes.outbound_desc') }}</p>
                      <div class="protocol-form__grid">
                        <KFormField :name="`${proto}-outbound-enabled`" :label="t('nodes.outbound_enabled')" :hint="t('nodes.hint_outbound')">
                          <template #default>
                            <label class="toggle-switch">
                              <input type="checkbox" :checked="configForm.extra_json.outbound?.enabled ?? false" @change="initOutbound(); configForm.extra_json.outbound.enabled = ($event.target as HTMLInputElement).checked" />
                              <span class="toggle-switch__slider" />
                            </label>
                          </template>
                        </KFormField>
                        <template v-if="configForm.extra_json.outbound?.enabled">
                          <KFormField :name="`${proto}-outbound-type`" :label="t('nodes.outbound_type')">
                            <template #default="{ fieldId }">
                              <KSelect :id="fieldId" v-model="configForm.extra_json.outbound.type" :options="[{ label: 'VLESS', value: 'vless' }, { label: 'VMess', value: 'vmess' }, { label: 'Trojan', value: 'trojan' }, { label: 'Shadowsocks', value: 'shadowsocks' }, { label: 'SOCKS5', value: 'socks5' }]" />
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-outbound-address`" :label="t('nodes.outbound_address')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.outbound.address" placeholder="proxy.example.com:443" />
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-outbound-uuid`" :label="t('nodes.outbound_uuid')">
                            <template #default="{ fieldId }">
                              <KInput :id="fieldId" v-model="configForm.extra_json.outbound.uuid" :placeholder="t('nodes.placeholder_uuid_or_password')" type="password" />
                            </template>
                          </KFormField>
                          <KFormField :name="`${proto}-outbound-tls`" :label="t('nodes.outbound_tls')">
                            <template #default>
                              <label class="toggle-switch">
                                <input type="checkbox" :checked="configForm.extra_json.outbound.tls" @change="configForm.extra_json.outbound.tls = ($event.target as HTMLInputElement).checked" />
                                <span class="toggle-switch__slider" />
                              </label>
                            </template>
                          </KFormField>
                          <template v-if="['vless', 'vmess', 'trojan'].includes(configForm.extra_json.outbound.type)">
                            <KFormField :name="`${proto}-outbound-path`" :label="t('nodes.outbound_path')">
                              <template #default="{ fieldId }">
                                <KInput :id="fieldId" v-model="configForm.extra_json.outbound.path" placeholder="/ws" />
                              </template>
                            </KFormField>
                            <KFormField :name="`${proto}-outbound-sni`" :label="t('nodes.outbound_sni')">
                              <template #default="{ fieldId }">
                                <KInput :id="fieldId" v-model="configForm.extra_json.outbound.sni" placeholder="sni.example.com" />
                              </template>
                            </KFormField>
                          </template>
                        </template>
                      </div>
                    </div>

                    <div class="protocol-form__actions">
                      <KButton variant="ghost" size="sm" @click="showConfigPreview = !showConfigPreview">
                        {{ showConfigPreview ? t('nodes.hide_preview') : t('nodes.show_preview') }}
                      </KButton>
                      <KButton variant="ghost" size="sm" @click="resetToDefaults">{{ t('nodes.reset_defaults') }}</KButton>
                      <KButton variant="ghost" size="sm" @click="cancelEdit">{{ t('btn.cancel') }}</KButton>
                      <KButton variant="primary" size="sm" :loading="savingConfig" @click="saveConfig">{{ t('nodes.save_config') }}</KButton>
                    </div>

                    <!-- Config Preview Panel -->
                    <div v-if="showConfigPreview" class="config-preview">
                      <h5 class="config-preview__title">{{ t('nodes.config_preview') }}</h5>
                      <pre class="config-preview__code"><code>{{ getConfigPreview().join('\n') }}</code></pre>
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

/* Form Groups */
.form-group {
  margin-top: var(--space-4);
  padding-top: var(--space-3);
  border-top: 1px solid var(--color-border);
}
.form-group:first-child {
  margin-top: 0;
  padding-top: 0;
  border-top: none;
}
.form-group__title {
  margin: 0 0 var(--space-3);
  font-size: var(--text-sm);
  font-weight: var(--font-semibold);
  color: var(--color-muted);
  text-transform: uppercase;
  letter-spacing: 0.03em;
}
.form-group__desc {
  margin: 0 0 var(--space-3);
  font-size: var(--text-xs);
  color: var(--color-muted);
  line-height: 1.5;
}
.form-group__full-width {
  grid-column: 1 / -1;
}

/* Danger field indicator */
.field--danger {
  border-left: 3px solid rgba(239, 68, 68, 0.5);
  padding-left: var(--space-2);
  border-radius: var(--radius-sm);
}

.text-muted { color: var(--color-muted); }
.text-sm { font-size: var(--text-sm); }

/* Responsive: single column on mobile */
@media (max-width: 640px) {
  .protocol-form__grid {
    grid-template-columns: 1fr;
  }
  .form-grid {
    grid-template-columns: 1fr;
  }
  .nodes-grid {
    grid-template-columns: 1fr;
  }
  .protocol-card__header {
    flex-direction: column;
    align-items: flex-start;
  }
  .protocol-card__controls {
    margin-left: 0;
    width: 100%;
    justify-content: flex-end;
    flex-wrap: wrap;
  }
}

/* Node Edit Form */
.node-edit-form {
  border-top: 1px solid var(--color-border);
  padding-top: var(--space-3);
  margin-top: var(--space-2);
}

/* Validation indicators */
.field--invalid :deep(input) {
  border-color: var(--color-danger, #ef4444) !important;
  box-shadow: 0 0 0 1px rgba(239, 68, 68, 0.3);
}
.validation-msg {
  font-size: var(--text-xs);
  color: var(--color-danger, #ef4444);
  margin-top: 2px;
  display: block;
}

/* Config Preview */
.config-preview {
  margin-top: var(--space-4);
  padding: var(--space-3);
  background: var(--color-surface-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
}
.config-preview__title {
  margin: 0 0 var(--space-2);
  font-size: var(--text-xs);
  font-weight: var(--font-semibold);
  color: var(--color-muted);
  text-transform: uppercase;
}
.config-preview__code {
  margin: 0;
  font-size: var(--text-xs);
  line-height: 1.6;
  color: var(--color-text);
  white-space: pre-wrap;
  word-break: break-all;
}

/* Chip Field */
.chip-field {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.chip-list {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
  min-height: 24px;
}
.chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  background: rgba(37, 99, 235, 0.1);
  color: var(--color-primary);
  border-radius: 9999px;
  font-size: var(--text-xs);
  font-weight: var(--font-medium);
  max-width: 100%;
  word-break: break-all;
}
.chip--key {
  font-family: monospace;
  background: var(--color-surface-2);
  color: var(--color-text);
}
.chip__remove {
  border: none;
  background: none;
  color: var(--color-danger, #ef4444);
  font-size: 14px;
  line-height: 1;
  cursor: pointer;
  padding: 0;
  opacity: 0.7;
  transition: opacity var(--duration-fast);
}
.chip__remove:hover { opacity: 1; }
.chip-input-row {
  display: flex;
  gap: var(--space-2);
  align-items: center;
}
.chip-input-row :deep(input) {
  flex: 1;
}

/* RTL support */
[data-dir="rtl"] .node-card { text-align: right; }
[data-dir="rtl"] .metric-row__val { text-align: left; }
[data-dir="rtl"] .metric-row__label { text-align: right; }
[data-dir="rtl"] .protocol-card__controls { margin-left: 0; margin-right: auto; }
[data-dir="rtl"] .form-group__title { text-align: right; }
[data-dir="rtl"] .field--danger { border-left: none; padding-left: 0; border-right: 3px solid rgba(239, 68, 68, 0.5); padding-right: var(--space-2); }
[data-dir="rtl"] .config-preview { text-align: right; }
[data-dir="rtl"] .config-preview__title { text-align: right; }
[data-dir="rtl"] .panel-title { text-align: right; }
[data-dir="rtl"] .token-banner { text-align: right; }
</style>
