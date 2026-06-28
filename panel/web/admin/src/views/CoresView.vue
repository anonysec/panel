<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useApi } from '@koris/composables/useApi'
import { useToast } from '@koris/composables/useToast'
import { useConfirm } from '@koris/composables/useConfirm'
import { useI18n } from '@koris/composables/useI18n'
import KSkeleton from '@koris/ui/KSkeleton.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KButton from '@koris/ui/KButton.vue'
import KInput from '@koris/ui/KInput.vue'
import KSelect from '@koris/ui/KSelect.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KSlideOver from '@koris/ui/KSlideOver.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'

const { get, post, put, del } = useApi()
const toast = useToast()
const { confirm } = useConfirm()
const { t } = useI18n()

// ═══════════════════════════════════════════════════════════════════════════════
// Types
// ═══════════════════════════════════════════════════════════════════════════════

interface Node {
  id: number
  name: string
  address: string
  status?: string
}

interface ProtocolConfig {
  enabled: boolean
  port: number
  network: string
  extra_json?: Record<string, any>
}

type VpnConfig = Record<string, ProtocolConfig>

interface NodeWithConfig {
  node: Node
  config: VpnConfig
  loading: boolean
  expanded: boolean
}

// ═══════════════════════════════════════════════════════════════════════════════
// Protocol definitions
// ═══════════════════════════════════════════════════════════════════════════════

const protocols = [
  { key: 'openvpn', name: 'OpenVPN', icon: '🔐', defaultPort: 1194, defaultNetwork: '10.8.0.0/20' },
  { key: 'wireguard', name: 'WireGuard', icon: '🛡️', defaultPort: 51820, defaultNetwork: '10.66.0.0/20' },
  { key: 'l2tp', name: 'L2TP/IPsec', icon: '🔗', defaultPort: 1701, defaultNetwork: '10.9.0.0/20' },
  { key: 'ikev2', name: 'IKEv2', icon: '🔑', defaultPort: 500, defaultNetwork: '10.10.0.0/20' },
  { key: 'ssh', name: 'SSH Tunnel', icon: '💻', defaultPort: 2222, defaultNetwork: '' },
] as const

// ═══════════════════════════════════════════════════════════════════════════════
// DNS Presets
// ═══════════════════════════════════════════════════════════════════════════════

const dnsPresets = [
  { label: 'Google', value: '8.8.8.8', secondary: '8.8.4.4' },
  { label: 'Cloudflare', value: '1.1.1.1', secondary: '1.0.0.1' },
  { label: 'Quad9', value: '9.9.9.9', secondary: '149.112.112.112' },
  { label: 'OpenDNS', value: '208.67.222.222', secondary: '208.67.220.220' },
  { label: 'AdGuard', value: '94.140.14.14', secondary: '94.140.15.15' },
  { label: 'Shecan', value: '178.22.122.100', secondary: '185.51.200.2' },
]

// ═══════════════════════════════════════════════════════════════════════════════
// State
// ═══════════════════════════════════════════════════════════════════════════════

const nodes = ref<NodeWithConfig[]>([])
const loading = ref(true)

// Side panel state
const panelOpen = ref(false)
const panelNodeId = ref<number | null>(null)
const panelProtocol = ref<string>('')
const panelSaving = ref(false)

// DNS dropdown visibility
const showDnsDropdown = ref(false)

// Protocol settings form
const panelForm = reactive<Record<string, any>>({})

// Edit node state
const editNodeOpen = ref(false)
const editNodeForm = reactive({ id: 0, name: '', address: '', port: 2083, api_key: '', client_cert: '', client_key: '', ca_cert: '' })
const editNodeSaving = ref(false)

// ═══════════════════════════════════════════════════════════════════════════════
// Protocol settings schemas
// ═══════════════════════════════════════════════════════════════════════════════

interface FieldDef {
  key: string
  label: string
  type: 'number' | 'text' | 'password' | 'select' | 'toggle' | 'dns'
  default?: any
  options?: { label: string; value: string }[]
  showIf?: (form: Record<string, any>) => boolean
  tooltip?: string
}

const protocolFields: Record<string, FieldDef[]> = {
  openvpn: [
    { key: 'port', label: 'services.port', type: 'number', default: 1194 },
    { key: 'transport', label: 'services.transport', type: 'select', default: 'udp', options: [
      { label: 'UDP', value: 'udp' }, { label: 'TCP', value: 'tcp' },
    ]},
    { key: 'cipher', label: 'services.cipher', type: 'select', default: 'AES-256-GCM', options: [
      { label: 'AES-256-GCM', value: 'AES-256-GCM' },
      { label: 'AES-128-GCM', value: 'AES-128-GCM' },
      { label: 'CHACHA20-POLY1305', value: 'CHACHA20-POLY1305' },
    ]},
    { key: 'tls_mode', label: 'services.tls_mode', type: 'select', default: 'tls-crypt', options: [
      { label: 'tls-crypt (most secure)', value: 'tls-crypt' },
      { label: 'tls-auth (compatible)', value: 'tls-auth' },
      { label: 'None (no protection)', value: 'none' },
    ]},
    { key: 'dns', label: 'services.dns_label', type: 'dns', default: '8.8.8.8' },
    { key: 'mtu', label: 'services.mtu', type: 'number', default: 1500 },
  ],
  wireguard: [
    { key: 'port', label: 'services.port', type: 'number', default: 51820 },
    { key: 'dns', label: 'services.dns_label', type: 'dns', default: '1.1.1.1' },
    { key: 'gaming_optimize', label: 'services.gaming_optimize', type: 'toggle', default: false,
      tooltip: 'services.gaming_desc' },
  ],
  l2tp: [
    { key: 'port', label: 'services.port', type: 'number', default: 1701 },
    { key: 'psk', label: 'services.psk', type: 'text', default: '' },
    { key: 'dns', label: 'services.dns_label', type: 'dns', default: '8.8.8.8' },
    { key: 'simple_mode', label: 'services.simple_mode', type: 'toggle', default: true },
    { key: 'auth_method', label: 'nodes.auth_method', type: 'select', default: 'MS-CHAPv2',
      showIf: (f) => !f.simple_mode, options: [
        { label: 'CHAP', value: 'CHAP' },
        { label: 'PAP', value: 'PAP' },
        { label: 'MS-CHAPv2', value: 'MS-CHAPv2' },
      ]},
    { key: 'dpd_interval', label: 'nodes.dpd_interval', type: 'number', default: 30,
      showIf: (f) => !f.simple_mode },
    { key: 'dpd_timeout', label: 'nodes.dpd_timeout', type: 'number', default: 120,
      showIf: (f) => !f.simple_mode },
  ],
  ikev2: [
    { key: 'port', label: 'services.port', type: 'number', default: 500 },
    { key: 'psk', label: 'services.psk', type: 'text', default: '' },
    { key: 'dns', label: 'services.dns_label', type: 'dns', default: '8.8.8.8' },
    { key: 'domain', label: 'services.domain', type: 'text', default: '' },
    { key: 'cert_source', label: 'services.tls_mode', type: 'select', default: 'letsencrypt', options: [
      { label: "Let's Encrypt (auto)", value: 'letsencrypt' },
      { label: 'Custom Certificate', value: 'custom' },
    ]},
    { key: 'domain', label: 'services.domain', type: 'text', default: '' },
  ],
  ssh: [
    { key: 'port', label: 'services.port', type: 'number', default: 2222 },
    { key: 'max_sessions', label: 'services.max_connections', type: 'number', default: 10 },
    { key: 'key_type', label: 'services.key_type', type: 'select', default: 'ed25519', options: [
      { label: 'ed25519 (recommended)', value: 'ed25519' }, { label: 'RSA', value: 'rsa' },
    ]},
  ],
}

// ═══════════════════════════════════════════════════════════════════════════════
// Computed
// ═══════════════════════════════════════════════════════════════════════════════

const panelTitle = computed(() => {
  const proto = protocols.find(p => p.key === panelProtocol.value)
  const node = nodes.value.find(n => n.node.id === panelNodeId.value)
  return proto && node ? `${proto.name} — ${node.node.name}` : ''
})

const currentFields = computed<FieldDef[]>(() => {
  return protocolFields[panelProtocol.value] || []
})

function enabledCount(config: VpnConfig): number {
  return protocols.filter(p => config[p.key]?.enabled).length
}

function isNodeOnline(node: Node): boolean {
  return node.status !== 'offline'
}

// ═══════════════════════════════════════════════════════════════════════════════
// Data fetching
// ═══════════════════════════════════════════════════════════════════════════════

onMounted(async () => {
  await fetchNodes()
})

async function fetchNodes() {
  loading.value = true
  try {
    const res = await get<{ nodes: Node[] }>('/api/admin/knode/nodes')
    const nodeList: Node[] = res.nodes || []
    nodes.value = nodeList.map(node => ({
      node,
      config: {},
      loading: true,
      expanded: false,
    }))
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
    const res = await get<{ configs: any[] }>(`/api/nodes/vpn-config/${nodeId}`)
    const configMap: VpnConfig = {}
    const configs = res.configs || []
    for (const c of configs) {
      configMap[c.protocol] = {
        enabled: c.enabled,
        port: c.port,
        network: c.network || '',
        extra_json: c.extra_json || {},
      }
    }
    nodes.value[idx].config = configMap
  } catch {
    // Node might be offline
  } finally {
    nodes.value[idx].loading = false
  }
}

// ═══════════════════════════════════════════════════════════════════════════════
// Toggle protocol (FIX: includes port in payload)
// ═══════════════════════════════════════════════════════════════════════════════

async function toggleProtocol(nodeId: number, idx: number, protocolKey: string, event: Event) {
  event.stopPropagation()
  const entry = nodes.value[idx]
  const current = entry.config[protocolKey]
  const proto = protocols.find(p => p.key === protocolKey)!
  const newEnabled = !(current?.enabled ?? false)

  // Optimistic update
  if (!entry.config[protocolKey]) {
    entry.config[protocolKey] = {
      enabled: newEnabled,
      port: proto.defaultPort,
      network: proto.defaultNetwork,
    }
  } else {
    entry.config[protocolKey].enabled = newEnabled
  }

  try {
    await post(`/api/nodes/vpn-config/${nodeId}`, {
      protocol: protocolKey,
      enabled: newEnabled,
      port: current?.port || proto.defaultPort,
      network: current?.network || proto.defaultNetwork,
    })
    toast.success(
      t(newEnabled ? 'services.enabled' : 'services.disabled')
        .replace('{proto}', proto.name)
    )
  } catch {
    // Rollback
    entry.config[protocolKey].enabled = !newEnabled
  }
}

// ═══════════════════════════════════════════════════════════════════════════════
// Node actions: Edit & Delete
// ═══════════════════════════════════════════════════════════════════════════════

function openEditNode(node: Node) {
  editNodeForm.id = node.id
  editNodeForm.name = node.name
  editNodeForm.address = node.address
  editNodeForm.port = 2083
  editNodeForm.api_key = ''
  editNodeForm.client_cert = ''
  editNodeForm.client_key = ''
  editNodeForm.ca_cert = ''
  editNodeOpen.value = true
}

async function saveEditNode() {
  editNodeSaving.value = true
  try {
    await put(`/api/admin/knode/nodes/${editNodeForm.id}`, {
      name: editNodeForm.name,
      address: editNodeForm.address,
      port: editNodeForm.port,
      ...(editNodeForm.api_key && { api_key: editNodeForm.api_key }),
      ...(editNodeForm.client_cert && { client_cert_pem: editNodeForm.client_cert }),
      ...(editNodeForm.client_key && { client_key_pem: editNodeForm.client_key }),
      ...(editNodeForm.ca_cert && { ca_cert_pem: editNodeForm.ca_cert }),
    })
    toast.success(t('nodes.edit_success'))
    const idx = nodes.value.findIndex(n => n.node.id === editNodeForm.id)
    if (idx >= 0) {
      nodes.value[idx].node.name = editNodeForm.name
      nodes.value[idx].node.address = editNodeForm.address
    }
    editNodeOpen.value = false
  } catch {
    // handled by useApi
  } finally {
    editNodeSaving.value = false
  }
}

async function deleteNode(node: Node) {
  const confirmed = await confirm({
    title: t('nodes.confirm_delete_title'),
    message: t('services.confirm_delete').replace('{name}', node.name),
    variant: 'danger',
    confirmText: t('services.delete_node'),
  })
  if (!confirmed) return

  try {
    await del(`/api/admin/knode/nodes/${node.id}`)
    toast.success(t('services.deleted').replace('{name}', node.name))
    nodes.value = nodes.value.filter(n => n.node.id !== node.id)
  } catch {
    // handled by useApi
  }
}

function toggleExpand(idx: number) {
  nodes.value[idx].expanded = !nodes.value[idx].expanded
}

// ═══════════════════════════════════════════════════════════════════════════════
// Side panel: Protocol settings
// ═══════════════════════════════════════════════════════════════════════════════

function openProtocolPanel(nodeId: number, protocolKey: string) {
  panelNodeId.value = nodeId
  panelProtocol.value = protocolKey
  panelOpen.value = true
  showDnsDropdown.value = false

  // Populate form from existing config
  const entry = nodes.value.find(n => n.node.id === nodeId)
  const config = entry?.config[protocolKey]
  const fields = protocolFields[protocolKey] || []

  // Reset form
  Object.keys(panelForm).forEach(k => delete panelForm[k])

  for (const field of fields) {
    if (field.key === 'port') {
      panelForm[field.key] = config?.port ?? field.default
    } else if (field.key === 'network') {
      panelForm[field.key] = config?.network ?? field.default
    } else {
      panelForm[field.key] = config?.extra_json?.[field.key] ?? field.default
    }
  }
}

function closePanel() {
  panelOpen.value = false
  showDnsDropdown.value = false
}

function selectDns(dns: string) {
  panelForm.dns = dns
  showDnsDropdown.value = false
}

function generatePsk() {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789'
  let result = ''
  const arr = new Uint8Array(32)
  crypto.getRandomValues(arr)
  for (let i = 0; i < 32; i++) {
    result += chars[arr[i] % chars.length]
  }
  panelForm.psk = result
}

async function saveProtocolSettings() {
  if (!panelNodeId.value || !panelProtocol.value) return
  panelSaving.value = true

  const fields = protocolFields[panelProtocol.value] || []
  const extra_json: Record<string, any> = {}
  let port = 0
  let network = ''

  for (const field of fields) {
    const val = panelForm[field.key]
    if (field.key === 'port') {
      port = Number(val) || 0
    } else if (field.key === 'network') {
      network = String(val || '')
    } else {
      extra_json[field.key] = val
    }
  }

  // Apply gaming optimize settings for wireguard
  if (panelProtocol.value === 'wireguard' && panelForm.gaming_optimize) {
    extra_json.persistent_keepalive = 15
    extra_json.mtu = 1280
  }

  try {
    await post(`/api/nodes/vpn-config/${panelNodeId.value}`, {
      protocol: panelProtocol.value,
      port,
      enabled: true,
      network,
      extra_json,
    })
    toast.success(t('services.settings_saved'))

    // Update local state
    const idx = nodes.value.findIndex(n => n.node.id === panelNodeId.value)
    if (idx >= 0) {
      nodes.value[idx].config[panelProtocol.value] = {
        enabled: true, port, network, extra_json,
      }
    }
    closePanel()
  } catch {
    // error handled by useApi
  } finally {
    panelSaving.value = false
  }
}
</script>

<template>
  <div class="page services-view">
    <header class="page-header">
      <div>
        <h1>{{ t('services.title') }}</h1>
        <p class="subtitle">{{ t('services.subtitle') }}</p>
      </div>
    </header>

    <!-- Loading state -->
    <div v-if="loading" class="loading-grid">
      <KSkeleton v-for="i in 3" :key="i" height="64px" />
    </div>

    <!-- Empty state -->
    <KEmptyState
      v-else-if="nodes.length === 0"
      icon="🖥️"
      :title="t('services.no_nodes')"
      :description="t('services.no_nodes_desc')"
      :action-text="t('services.add_node')"
      @action="$router.push('/dashboard/services')"
    />

    <!-- Node table -->
    <div v-else class="nodes-table">
      <!-- Table header -->
      <div class="table-header">
        <span class="col-name">{{ t('nodes.node_name') }}</span>
        <span class="col-address">{{ t('nodes.address') }}</span>
        <span class="col-status">Status</span>
        <span class="col-protocols">Protocols</span>
        <span class="col-actions">Actions</span>
      </div>

      <!-- Table rows -->
      <div
        v-for="(entry, idx) in nodes"
        :key="entry.node.id"
        class="node-row-wrap"
        :class="{ expanded: entry.expanded }"
      >
        <div class="node-row" @click="toggleExpand(idx)">
          <span class="col-name">
            <span class="node-name">{{ entry.node.name }}</span>
          </span>
          <span class="col-address">
            <code class="node-ip">{{ entry.node.address }}</code>
          </span>
          <span class="col-status">
            <KStatusPill
              :status="isNodeOnline(entry.node) ? 'active' : 'inactive'"
              :label="isNodeOnline(entry.node) ? 'Online' : 'Offline'"
            />
          </span>
          <span class="col-protocols">
            <span class="proto-count">
              {{ t('services.protocols_enabled').replace('{count}', String(enabledCount(entry.config))) }}
            </span>
          </span>
          <span class="col-actions" @click.stop>
            <button class="action-btn" :title="t('services.edit_node')" @click="openEditNode(entry.node)">
              <svg width="16" height="16" viewBox="0 0 16 16" fill="none"><path d="M11.13 1.87a1.25 1.25 0 0 1 1.77 0l1.23 1.23a1.25 1.25 0 0 1 0 1.77l-8.5 8.5-3.25.75.75-3.25 8.5-8.5z" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/></svg>
            </button>
            <button class="action-btn action-btn--danger" :title="t('services.delete_node')" @click="deleteNode(entry.node)">
              <svg width="16" height="16" viewBox="0 0 16 16" fill="none"><path d="M2 4h12M5.33 4V2.67a1.33 1.33 0 0 1 1.34-1.34h2.66a1.33 1.33 0 0 1 1.34 1.34V4m2 0v9.33a1.33 1.33 0 0 1-1.34 1.34H4.67a1.33 1.33 0 0 1-1.34-1.34V4h9.34z" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/></svg>
            </button>
          </span>
        </div>

        <!-- Expanded protocol cards -->
        <Transition name="expand">
          <div v-if="entry.expanded" class="protocols-area">
            <div v-if="entry.loading" class="protocol-cards">
              <KSkeleton v-for="i in 5" :key="i" height="52px" />
            </div>
            <div v-else class="protocol-cards">
              <div
                v-for="proto in protocols"
                :key="proto.key"
                class="protocol-card"
                :class="{ enabled: entry.config[proto.key]?.enabled }"
                @click="openProtocolPanel(entry.node.id, proto.key)"
              >
                <div class="card-left">
                  <span class="proto-icon">{{ proto.icon }}</span>
                  <span class="proto-name">{{ proto.name }}</span>
                </div>
                <button
                  class="toggle-btn"
                  :class="{ active: entry.config[proto.key]?.enabled }"
                  :title="entry.config[proto.key]?.enabled ? 'Disable' : 'Enable'"
                  @click="toggleProtocol(entry.node.id, idx, proto.key, $event)"
                >
                  <span class="toggle-track">
                    <span class="toggle-thumb" />
                  </span>
                </button>
              </div>
            </div>
          </div>
        </Transition>
      </div>
    </div>

    <!-- Protocol settings side panel -->
    <KSlideOver
      :open="panelOpen"
      :title="panelTitle"
      width="420px"
      @close="closePanel"
    >
      <div class="panel-body">
        <template v-for="field in currentFields" :key="field.key">
          <div
            v-if="!field.showIf || field.showIf(panelForm)"
            class="panel-field"
          >
            <KFormField :label="t(field.label)" :name="field.key">
              <!-- Select -->
              <KSelect
                v-if="field.type === 'select'"
                v-model="panelForm[field.key]"
                :options="field.options || []"
              />

              <!-- Toggle -->
              <div v-else-if="field.type === 'toggle'" class="toggle-field">
                <button
                  class="toggle-btn"
                  :class="{ active: panelForm[field.key] }"
                  @click="panelForm[field.key] = !panelForm[field.key]"
                >
                  <span class="toggle-track">
                    <span class="toggle-thumb" />
                  </span>
                </button>
                <span v-if="field.tooltip" class="field-hint">{{ t(field.tooltip) }}</span>
              </div>

              <!-- DNS field with dropdown -->
              <div v-else-if="field.type === 'dns'" class="dns-field">
                <div class="dns-input-wrap">
                  <KInput
                    v-model="panelForm[field.key]"
                    type="text"
                    placeholder="8.8.8.8"
                  />
                  <button
                    class="dns-dropdown-btn"
                    type="button"
                    @click.stop="showDnsDropdown = !showDnsDropdown"
                  >
                    ▾
                  </button>
                </div>
                <Transition name="fade">
                  <div v-if="showDnsDropdown" class="dns-dropdown">
                    <button
                      v-for="dns in dnsPresets"
                      :key="dns.value"
                      class="dns-option"
                      @click="selectDns(dns.value)"
                    >
                      <span class="dns-label">{{ dns.label }}</span>
                      <span class="dns-value">{{ dns.value }}</span>
                    </button>
                  </div>
                </Transition>
              </div>

              <!-- Password with generate button (PSK) -->
              <div v-else-if="field.type === 'password' || (field.key === 'psk')" class="password-field">
                <KInput
                  v-model="panelForm[field.key]"
                  type="text"
                  autocomplete="off"
                  :placeholder="t(field.label)"
                />
                <KButton size="sm" variant="ghost" @click="generatePsk">
                  {{ t('services.auto_generate') }}
                </KButton>
              </div>

              <!-- Default: number/text input -->
              <KInput
                v-else
                v-model="panelForm[field.key]"
                :type="field.type"
                :placeholder="String(field.default || '')"
              />
            </KFormField>
          </div>
        </template>
      </div>

      <template #footer>
        <div class="panel-footer">
          <KButton variant="ghost" @click="closePanel">Cancel</KButton>
          <KButton :loading="panelSaving" @click="saveProtocolSettings">
            {{ t('nodes.save_config') }}
          </KButton>
        </div>
      </template>
    </KSlideOver>

    <!-- Edit node slide-over -->
    <KSlideOver
      :open="editNodeOpen"
      :title="t('nodes.edit_node')"
      width="420px"
      @close="editNodeOpen = false"
    >
      <div class="panel-body">
        <form autocomplete="off" @submit.prevent>
          <KFormField :label="t('nodes.node_name')" name="edit-name">
            <KInput v-model="editNodeForm.name" type="text" autocomplete="off" />
          </KFormField>
          <KFormField :label="t('nodes.address')" name="edit-address">
            <KInput v-model="editNodeForm.address" type="text" autocomplete="off" />
          </KFormField>
          <KFormField label="Port" name="edit-port">
            <KInput v-model="editNodeForm.port" type="number" autocomplete="off" />
          </KFormField>
          <KFormField label="API Key" name="edit-apikey">
            <KInput v-model="editNodeForm.api_key" type="text" placeholder="Leave empty to keep current" autocomplete="off" />
          </KFormField>
          <KFormField label="Client Certificate (PEM)" name="edit-cert">
            <textarea v-model="editNodeForm.client_cert" class="pem-textarea" placeholder="Leave empty to keep current" autocomplete="off" spellcheck="false" />
          </KFormField>
          <KFormField label="Client Key (PEM)" name="edit-key">
            <textarea v-model="editNodeForm.client_key" class="pem-textarea" placeholder="Leave empty to keep current" autocomplete="off" spellcheck="false" />
          </KFormField>
          <KFormField label="CA Certificate (PEM)" name="edit-ca">
            <textarea v-model="editNodeForm.ca_cert" class="pem-textarea" placeholder="Leave empty to keep current" autocomplete="off" spellcheck="false" />
          </KFormField>
        </form>
      </div>
      <template #footer>
        <div class="panel-footer">
          <KButton variant="ghost" @click="editNodeOpen = false">Cancel</KButton>
          <KButton :loading="editNodeSaving" @click="saveEditNode">
            {{ t('nodes.save_config') }}
          </KButton>
        </div>
      </template>
    </KSlideOver>
  </div>
</template>

<style scoped>
.services-view {
  padding: var(--space-6, 24px);
  max-width: 1200px;
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
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
  gap: var(--space-3, 12px);
}

/* ═══════════════════════════════════════════════════════════════════
   Node table
   ═══════════════════════════════════════════════════════════════════ */

.nodes-table {
  border: 1px solid var(--color-border, #28333f);
  border-radius: var(--radius-lg, 12px);
  overflow: hidden;
  width: 100%;
}

.table-header {
  display: grid;
  grid-template-columns: 1.5fr 1.5fr 0.8fr 1.2fr 0.8fr;
  gap: var(--space-3, 12px);
  padding: var(--space-3, 12px) var(--space-5, 20px);
  background: var(--color-surface-2, #1e2630);
  border-bottom: 1px solid var(--color-border, #28333f);
  font-size: var(--text-xs, 11px);
  font-weight: var(--font-semibold, 600);
  color: var(--color-muted, #8b98a5);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.node-row-wrap {
  border-bottom: 1px solid var(--color-border, #28333f);
}

.node-row-wrap:last-child {
  border-bottom: none;
}

.node-row {
  display: grid;
  grid-template-columns: 1.5fr 1.5fr 0.8fr 1.2fr 0.8fr;
  gap: var(--space-3, 12px);
  padding: var(--space-4, 16px) var(--space-5, 20px);
  align-items: center;
  cursor: pointer;
  transition: background 0.15s;
}

.node-row:hover {
  background: var(--color-surface-2, #1e2630);
}

.node-name {
  font-size: var(--text-base, 14px);
  font-weight: var(--font-semibold, 600);
  color: var(--color-text, #e6edf3);
}

.node-ip {
  font-size: var(--text-sm, 13px);
  color: var(--color-muted, #8b98a5);
  font-family: var(--font-mono, monospace);
  background: none;
  padding: 0;
}

.proto-count {
  font-size: var(--text-sm, 13px);
  color: var(--color-muted, #8b98a5);
}

/* Action buttons */
.col-actions {
  display: flex;
  gap: var(--space-2, 8px);
  align-items: center;
}

.action-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border: 1px solid var(--color-border, #28333f);
  border-radius: var(--radius-md, 8px);
  background: transparent;
  color: var(--color-muted, #8b98a5);
  cursor: pointer;
  transition: all 0.15s;
}

.action-btn:hover {
  border-color: var(--color-primary, #2563eb);
  color: var(--color-primary, #2563eb);
  background: rgba(37, 99, 235, 0.08);
}

.action-btn--danger:hover {
  border-color: var(--color-danger, #ef4444);
  color: var(--color-danger, #ef4444);
  background: rgba(239, 68, 68, 0.08);
}

/* ═══════════════════════════════════════════════════════════════════
   Protocol cards (expanded area)
   ═══════════════════════════════════════════════════════════════════ */

.protocols-area {
  padding: var(--space-4, 16px) var(--space-5, 20px);
  background: var(--color-surface-2, #1e2630);
  border-top: 1px solid var(--color-border, #28333f);
}

.protocol-cards {
  display: flex;
  gap: var(--space-3, 12px);
  flex-wrap: wrap;
}

.protocol-card {
  display: flex;
  align-items: center;
  gap: var(--space-3, 12px);
  padding: var(--space-3, 12px) var(--space-4, 16px);
  background: var(--color-surface, #161b22);
  border: 1px solid var(--color-border, #28333f);
  border-left: 3px solid var(--color-border, #28333f);
  border-radius: var(--radius-md, 8px);
  cursor: pointer;
  transition: border-color 0.15s, background 0.15s, box-shadow 0.15s;
  min-width: 180px;
  flex: 1;
}

.protocol-card:hover {
  border-color: var(--color-primary, #2563eb);
  background: rgba(37, 99, 235, 0.04);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.15);
}

.protocol-card.enabled {
  border-left-color: var(--color-success, #22c55e);
}

.card-left {
  display: flex;
  align-items: center;
  gap: var(--space-2, 8px);
}

.proto-icon {
  font-size: 18px;
  flex-shrink: 0;
}

.proto-name {
  font-size: var(--text-sm, 13px);
  font-weight: var(--font-semibold, 600);
  color: var(--color-text, #e6edf3);
  white-space: nowrap;
}

.proto-port {
  font-size: var(--text-xs, 11px);
  color: var(--color-muted, #8b98a5);
  font-family: var(--font-mono, monospace);
  margin-left: auto;
  margin-right: var(--space-2, 8px);
}

/* ═══════════════════════════════════════════════════════════════════
   Toggle button
   ═══════════════════════════════════════════════════════════════════ */

.toggle-btn {
  flex-shrink: 0;
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
  transition: background 0.15s;
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
  transition: transform 0.15s;
}

.toggle-btn.active .toggle-thumb {
  transform: translateX(16px);
}

.toggle-field {
  display: flex;
  align-items: center;
  gap: var(--space-3, 12px);
}

.field-hint {
  font-size: var(--text-xs, 11px);
  color: var(--color-muted, #8b98a5);
  line-height: 1.4;
}

/* ═══════════════════════════════════════════════════════════════════
   DNS field
   ═══════════════════════════════════════════════════════════════════ */

.dns-field {
  position: relative;
}

.dns-input-wrap {
  display: flex;
  align-items: stretch;
}

.dns-input-wrap :deep(input) {
  border-top-right-radius: 0;
  border-bottom-right-radius: 0;
}

.dns-dropdown-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  border: 1px solid var(--color-border, #28333f);
  border-left: none;
  border-radius: 0 var(--radius-md, 8px) var(--radius-md, 8px) 0;
  background: var(--color-surface-2, #1e2630);
  color: var(--color-muted, #8b98a5);
  cursor: pointer;
  font-size: 14px;
  transition: background 0.15s;
}

.dns-dropdown-btn:hover {
  background: var(--color-surface, #161b22);
  color: var(--color-text, #e6edf3);
}

.dns-dropdown {
  position: absolute;
  top: calc(100% + 4px);
  left: 0;
  right: 0;
  background: var(--color-surface, #161b22);
  border: 1px solid var(--color-border, #28333f);
  border-radius: var(--radius-md, 8px);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.3);
  z-index: 50;
  overflow: hidden;
}

.dns-option {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding: var(--space-2, 8px) var(--space-3, 12px);
  border: none;
  background: transparent;
  color: var(--color-text, #e6edf3);
  cursor: pointer;
  font-size: var(--text-sm, 13px);
  transition: background 0.1s;
}

.dns-option:hover {
  background: var(--color-surface-2, #1e2630);
}

.dns-label {
  font-weight: var(--font-medium, 500);
}

.dns-value {
  font-family: var(--font-mono, monospace);
  font-size: var(--text-xs, 11px);
  color: var(--color-muted, #8b98a5);
}

/* ═══════════════════════════════════════════════════════════════════
   Password field
   ═══════════════════════════════════════════════════════════════════ */

.password-field {
  display: flex;
  gap: var(--space-2, 8px);
  align-items: flex-start;
}

.password-field :deep(.k-input) {
  flex: 1;
}

/* ═══════════════════════════════════════════════════════════════════
   Side panel body
   ═══════════════════════════════════════════════════════════════════ */

.panel-body {
  display: flex;
  flex-direction: column;
  gap: var(--space-4, 16px);
  padding: var(--space-4, 16px);
}

.panel-field {
  /* wrapper for conditional fields */
}

.panel-footer {
  display: flex;
  justify-content: flex-end;
  gap: var(--space-3, 12px);
}

/* PEM textarea for certs */
.pem-textarea {
  width: 100%;
  min-height: 80px;
  padding: var(--space-2, 8px) var(--space-3, 12px);
  background: var(--color-surface-2, #1e2630);
  border: 1px solid var(--color-border, #28333f);
  border-radius: var(--radius-md, 8px);
  color: var(--color-text, #e6edf3);
  font-family: var(--font-mono, monospace);
  font-size: var(--text-xs, 11px);
  resize: vertical;
}

.pem-textarea:focus {
  outline: none;
  border-color: var(--color-primary, #2563eb);
}

/* ═══════════════════════════════════════════════════════════════════
   Transitions
   ═══════════════════════════════════════════════════════════════════ */

.expand-enter-active,
.expand-leave-active {
  transition: all 250ms ease;
  overflow: hidden;
  max-height: 300px;
  opacity: 1;
}

.expand-enter-from,
.expand-leave-to {
  max-height: 0;
  opacity: 0;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 150ms;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

/* ═══════════════════════════════════════════════════════════════════
   Mobile responsive
   ═══════════════════════════════════════════════════════════════════ */

@media (max-width: 768px) {
  .services-view {
    padding: var(--space-4, 16px);
  }

  .table-header {
    display: none;
  }

  .node-row {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--space-2, 8px);
    padding: var(--space-4, 16px);
  }

  .node-row .col-actions {
    align-self: flex-end;
    margin-top: var(--space-1, 4px);
  }

  .protocol-cards {
    flex-direction: column;
  }

  .protocol-card {
    min-width: unset;
  }
}
</style>
