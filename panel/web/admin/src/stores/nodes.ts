import { ref } from 'vue'
import { defineStore } from 'pinia'
import { useApi } from '@koris/composables/useApi'
import type { NodeItem } from '@koris/types'

/**
 * VPN global settings
 */
export interface VPNSettings {
  id?: number
  openvpn_port: number
  openvpn_protocol: string
  openvpn_network: string
  l2tp_network: string
  ikev2_network: string
  ipsec_psk: string
  dns_1: string
  dns_2: string
  updated_at?: string
  openvpn_service_status: string
  ca_file?: string
  ca_exists: boolean
  tls_crypt_file?: string
  tls_crypt_exists: boolean
  remote_host?: string
  active_node?: string
}

/**
 * A task dispatched to a node (e.g. restart service, apply config)
 */
export interface NodeTask {
  id: number
  node_id: number
  node_name?: string
  action: string
  payload_json?: Record<string, unknown>
  status: string
  error?: string
  created_at: string
  completed_at?: string
}

/**
 * Per-node VPN configuration entry
 */
export interface NodeVPNConfig {
  id?: number
  node_id: number
  protocol: string
  enabled: boolean
  port: number
  network: string
  extra_json?: Record<string, unknown>
  encryption?: string
  mtu?: number
  max_clients?: number
  enable_logs?: boolean
  conn_limit?: number
}

/**
 * Node creation payload matching POST /api/nodes
 */
export interface CreateNodePayload {
  name: string
  public_ip: string
  domain: string
}

/**
 * Node edit payload matching PATCH /api/nodes/:id
 */
export interface EditNodePayload {
  name?: string
  public_ip?: string
  domain?: string
  proxy_enabled?: boolean
  proxy_type?: string
  proxy_address?: string
  proxy_username?: string
  proxy_password?: string
}

/**
 * Node task creation payload matching POST /api/node/tasks
 */
export interface CreateNodeTaskPayload {
  node_id: number
  action: string
  payload_json?: Record<string, unknown>
}

/**
 * VPN settings update payload matching PATCH /api/vpn/settings
 */
export interface UpdateVPNSettingsPayload {
  openvpn_port?: number
  openvpn_protocol?: string
  openvpn_network?: string
  l2tp_network?: string
  ikev2_network?: string
  ipsec_psk?: string
  dns_1?: string
  dns_2?: string
  apply?: boolean
}

/**
 * API response types matching backend endpoints
 */
interface NodesListResponse {
  ok: boolean
  nodes: NodeItem[]
}

interface NodeCreateResponse {
  ok: boolean
  id: number
  token: string
}

interface NodeTasksListResponse {
  ok: boolean
  tasks: NodeTask[]
}

interface NodeTaskCreateResponse {
  ok: boolean
  id: number
}

interface VPNSettingsResponse {
  ok: boolean
  settings: VPNSettings
}

interface VPNSettingsUpdateResponse {
  ok: boolean
  settings: VPNSettings
  applied?: boolean
  apply_error?: string
}

interface NodeVPNConfigsResponse {
  ok: boolean
  configs: NodeVPNConfig[]
}

interface NodeTokenResponse {
  ok: boolean
  token: string
}

interface NodeMutationResponse {
  ok: boolean
}

/**
 * Nodes management store (Pinia Composition API style)
 *
 * Manages node list, node tasks, VPN global settings, and per-node VPN configurations.
 * Provides actions for CRUD operations on nodes, task dispatch, and VPN config management.
 * Uses useApi composable for all API interactions with loading state management.
 *
 * Requirements: 3.1, 3.3, 22.8
 */
export const useNodesStore = defineStore('nodes', () => {
  // ─── State ────────────────────────────────────────────────────────────────
  const list = ref<NodeItem[]>([])
  const tasks = ref<NodeTask[]>([])
  const vpnSettings = ref<VPNSettings | null>(null)
  const vpnConfigs = ref<Record<number, NodeVPNConfig[]>>({})
  const loading = ref(false)

  // ─── API composable ───────────────────────────────────────────────────────
  // No onUnauthorized handler — the router guard handles auth redirects.
  // This prevents race conditions where a 401 during initial data load
  // would clear auth state and cause a redirect loop after login.
  const { get, post, patch, del, error } = useApi()

  // ─── Actions ──────────────────────────────────────────────────────────────

  /**
   * Load all nodes from the backend.
   * GET /api/nodes → { ok, nodes: NodeItem[] }
   *
   * Sets loading = true before request, false after (Requirement 3.3).
   * On error, preserves existing data (Requirement 3.4).
   */
  async function loadNodes(): Promise<void> {
    loading.value = true
    try {
      const res = await get<NodesListResponse>('/api/nodes')
      list.value = res.nodes || []
    } catch {
      // Preserve existing data on error (Requirement 3.4)
    } finally {
      loading.value = false
    }
  }

  /**
   * Load all node tasks from the backend.
   * GET /api/node/tasks → { ok, tasks: NodeTask[] }
   *
   * Sets loading = true before request, false after (Requirement 3.3).
   * On error, preserves existing data (Requirement 3.4).
   */
  async function loadNodeTasks(): Promise<void> {
    loading.value = true
    try {
      const res = await get<NodeTasksListResponse>('/api/node/tasks')
      tasks.value = res.tasks || []
    } catch {
      // Preserve existing data on error (Requirement 3.4)
    } finally {
      loading.value = false
    }
  }

  /**
   * Load VPN global settings.
   * GET /api/vpn/settings → { ok, settings: VPNSettings }
   *
   * Sets loading = true before request, false after (Requirement 3.3).
   * On error, preserves existing data (Requirement 3.4).
   */
  async function loadVpnSettings(): Promise<void> {
    loading.value = true
    try {
      const res = await get<VPNSettingsResponse>('/api/vpn/settings')
      vpnSettings.value = res.settings
    } catch {
      // Preserve existing data on error (Requirement 3.4)
    } finally {
      loading.value = false
    }
  }

  /**
   * Load per-node VPN configurations.
   * GET /api/nodes/vpn-config/:nodeId → { ok, configs: NodeVPNConfig[] }
   *
   * On error, preserves existing data (Requirement 3.4).
   */
  async function loadNodeVpnConfigs(nodeId: number): Promise<void> {
    loading.value = true
    try {
      const res = await get<NodeVPNConfigsResponse>(`/api/nodes/vpn-config/${nodeId}`)
      vpnConfigs.value[nodeId] = res.configs || []
    } catch {
      // Preserve existing data on error (Requirement 3.4)
    } finally {
      loading.value = false
    }
  }

  /**
   * Create a new node.
   * POST /api/nodes → { ok, id, token }
   *
   * Returns the auth token on success for display to the user.
   * On success, reloads the nodes list.
   * On error, preserves existing data.
   */
  async function createNode(payload: CreateNodePayload): Promise<string | null> {
    loading.value = true
    try {
      const res = await post<NodeCreateResponse>('/api/nodes', payload)
      await loadNodes()
      return res.token
    } catch {
      // Preserve existing data on error (Requirement 3.4)
      return null
    } finally {
      loading.value = false
    }
  }

  /**
   * Rotate a node's authentication token.
   * POST /api/nodes/:id/rotate-token → { ok, token }
   *
   * Returns the new token on success.
   * On error, preserves existing data.
   */
  async function rotateNodeToken(nodeId: number): Promise<string | null> {
    loading.value = true
    try {
      const res = await post<NodeTokenResponse>(`/api/nodes/${nodeId}/rotate-token`)
      return res.token
    } catch {
      // Preserve existing data on error (Requirement 3.4)
      return null
    } finally {
      loading.value = false
    }
  }

  /**
   * Enable or disable a node.
   * POST /api/nodes/:id/enable or /api/nodes/:id/disable → { ok }
   *
   * On success, reloads the nodes list.
   * On error, preserves existing data.
   */
  async function updateNode(nodeId: number, enabled: boolean): Promise<boolean> {
    loading.value = true
    try {
      const endpoint = enabled ? 'enable' : 'disable'
      await post<NodeMutationResponse>(`/api/nodes/${nodeId}/${endpoint}`)
      await loadNodes()
      return true
    } catch {
      // Preserve existing data on error (Requirement 3.4)
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * Create a node task (dispatch an action to a node).
   * POST /api/node/tasks → { ok, id }
   *
   * On success, reloads the node tasks list.
   * On error, preserves existing data.
   */
  async function createNodeTask(payload: CreateNodeTaskPayload): Promise<boolean> {
    loading.value = true
    try {
      await post<NodeTaskCreateResponse>('/api/node/tasks', payload)
      await loadNodeTasks()
      return true
    } catch {
      // Preserve existing data on error (Requirement 3.4)
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * Update VPN global settings.
   * PATCH /api/vpn/settings → { ok, settings, applied?, apply_error? }
   *
   * On success, updates the vpnSettings state.
   * On error, preserves existing data.
   * Returns the apply_error string if settings were saved but apply failed.
   */
  async function updateVpnSettings(payload: UpdateVPNSettingsPayload): Promise<{ success: boolean; applyError?: string }> {
    loading.value = true
    try {
      const res = await patch<VPNSettingsUpdateResponse>('/api/vpn/settings', payload)
      vpnSettings.value = res.settings
      return { success: true, applyError: res.apply_error }
    } catch {
      // Preserve existing data on error (Requirement 3.4)
      return { success: false }
    } finally {
      loading.value = false
    }
  }

  /**
   * Save per-node VPN configuration.
   * POST /api/nodes/vpn-config/:nodeId → { ok }
   *
   * On success, reloads the node's VPN configs.
   * On error, preserves existing data.
   */
  async function saveNodeVpnConfig(nodeId: number, config: Omit<NodeVPNConfig, 'id' | 'node_id'>): Promise<boolean> {
    loading.value = true
    try {
      await post<NodeMutationResponse>(`/api/nodes/vpn-config/${nodeId}`, config)
      await loadNodeVpnConfigs(nodeId)
      return true
    } catch {
      // Preserve existing data on error (Requirement 3.4)
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * Delete a node.
   * DELETE /api/nodes/:id → { ok }
   *
   * On success, reloads the nodes list.
   * On error, preserves existing data.
   */
  async function deleteNode(nodeId: number): Promise<boolean> {
    loading.value = true
    try {
      await del<NodeMutationResponse>(`/api/nodes/${nodeId}`)
      await loadNodes()
      return true
    } catch {
      // Preserve existing data on error (Requirement 3.4)
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * Edit a node's name, public_ip, or domain.
   * PATCH /api/nodes/:id → { ok }
   *
   * On success, reloads the nodes list.
   * On error, preserves existing data.
   */
  async function editNode(nodeId: number, payload: EditNodePayload): Promise<boolean> {
    loading.value = true
    try {
      await patch<NodeMutationResponse>(`/api/nodes/${nodeId}`, payload)
      await loadNodes()
      return true
    } catch {
      // Preserve existing data on error (Requirement 3.4)
      return false
    } finally {
      loading.value = false
    }
  }

  // ─── Expose ───────────────────────────────────────────────────────────────
  return {
    // State
    list,
    tasks,
    vpnSettings,
    vpnConfigs,
    loading,

    // API state (from useApi)
    error,

    // Actions
    loadNodes,
    loadNodeTasks,
    loadVpnSettings,
    loadNodeVpnConfigs,
    createNode,
    rotateNodeToken,
    updateNode,
    deleteNode,
    editNode,
    createNodeTask,
    updateVpnSettings,
    saveNodeVpnConfig,
  }
})
