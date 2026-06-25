import { ref } from 'vue'
import { defineStore } from 'pinia'
import { useApi } from '@koris/composables/useApi'

/**
 * gRPC-based node model
 */
export interface KnodeNode {
  id: number
  name: string
  address: string
  port: number
  enabled: boolean
  status: 'online' | 'offline' | 'stale'
  lastSeenAt: string
  createdAt: string
  updatedAt: string
}

/**
 * Core (VPN protocol) status on a node
 */
export interface CoreStatus {
  coreType: string
  status: 'running' | 'stopped' | 'error'
  port?: number
  sessions?: number
  pid?: number
}

/**
 * Active VPN session on a node
 */
export interface VPNSession {
  username: string
  coreType: string
  clientIp: string
  assignedIp: string
  duration: number
  rxBytes: number
  txBytes: number
}

/**
 * Firewall rule on a node
 */
export interface FirewallRule {
  port: number
  protocol: 'tcp' | 'udp'
  direction: 'in' | 'out'
  action: 'allow' | 'deny'
  sourceCidr?: string
  comment?: string
}

/**
 * Outbound tunnel on a node
 */
export interface Tunnel {
  id: string
  protocol: string
  exitAddress: string
  exitPort: number
  state: 'active' | 'inactive' | 'error'
  createdAt: string
}

/**
 * Tunnel setup configuration
 */
export interface TunnelConfig {
  protocol: string
  exitAddress: string
  exitPort: number
  extra?: Record<string, unknown>
}

/**
 * Per-core certificate information
 */
export interface CertInfo {
  coreType: string
  subject: string
  issuer: string
  notBefore: string
  notAfter: string
  daysUntilExpiry: number
}

/**
 * Node creation payload
 */
export interface NodeFormData {
  name: string
  address: string
  port: number
  api_key: string
  client_cert_pem: string
  client_key_pem: string
  ca_cert_pem: string
}

/**
 * API response types
 */
interface NodesListResponse {
  ok: boolean
  nodes: KnodeNode[]
}

interface NodeCreateResponse {
  ok: boolean
  id: number
}

interface NodeMutationResponse {
  ok: boolean
}

interface TestConnectionResponse {
  ok: boolean
  version?: string
  health?: string
}

interface CoresListResponse {
  ok: boolean
  cores: CoreStatus[]
}

interface SessionsListResponse {
  ok: boolean
  sessions: VPNSession[]
}

interface FirewallListResponse {
  ok: boolean
  rules: FirewallRule[]
}

interface TunnelsListResponse {
  ok: boolean
  tunnels: Tunnel[]
}

interface CertsInfoResponse {
  ok: boolean
  certs: CertInfo[]
}

/**
 * Nodes management store — refactored for gRPC model.
 *
 * Provides CRUD for nodes, core management, session management,
 * firewall rules, tunnel management, and certificate operations.
 * All actions use the useApi() composable with proper endpoints.
 *
 * Requirements: 1.1–1.7, 2.1–2.5, 3.1–3.4, 5.1–5.6, 6.1–6.4, 7.1–7.5, 8.1–8.6, 9.1–9.4
 */
export const useNodesStore = defineStore('nodes', () => {
  // ─── State ────────────────────────────────────────────────────────────────
  const list = ref<KnodeNode[]>([])
  const loading = ref(false)

  // ─── API composable ───────────────────────────────────────────────────────
  const { get, post, put, del, error } = useApi()

  // ─── Node CRUD ────────────────────────────────────────────────────────────

  /**
   * Load all knode nodes.
   * GET /api/admin/knode/nodes → { ok, nodes }
   */
  async function loadNodes(): Promise<void> {
    loading.value = true
    try {
      const res = await get<NodesListResponse>('/api/admin/knode/nodes')
      list.value = res.nodes || []
    } catch {
      // Preserve existing data on error
    } finally {
      loading.value = false
    }
  }

  /**
   * Create a new knode node.
   * POST /api/admin/knode/nodes → { ok, id }
   * Backend tests connection before saving.
   */
  async function createNode(data: NodeFormData): Promise<number | null> {
    loading.value = true
    try {
      const res = await post<NodeCreateResponse>('/api/admin/knode/nodes', data)
      await loadNodes()
      return res.id
    } catch {
      return null
    } finally {
      loading.value = false
    }
  }

  /**
   * Update an existing node's credentials/config.
   * PUT /api/admin/knode/nodes/{id} → { ok }
   */
  async function updateNode(id: number, data: Partial<NodeFormData>): Promise<boolean> {
    loading.value = true
    try {
      await put<NodeMutationResponse>(`/api/admin/knode/nodes/${id}`, data)
      await loadNodes()
      return true
    } catch {
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * Delete a knode node.
   * DELETE /api/admin/knode/nodes/{id} → { ok }
   */
  async function deleteNode(id: number): Promise<boolean> {
    loading.value = true
    try {
      await del<NodeMutationResponse>(`/api/admin/knode/nodes/${id}`)
      await loadNodes()
      return true
    } catch {
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * Test connection to an existing node.
   * POST /api/admin/knode/nodes/{id}/test → { ok, version, health }
   */
  async function testConnection(id: number): Promise<TestConnectionResponse | null> {
    try {
      const res = await post<TestConnectionResponse>(`/api/admin/knode/nodes/${id}/test`)
      return res
    } catch {
      return null
    }
  }

  // ─── Core Management ──────────────────────────────────────────────────────

  /**
   * List cores (VPN protocols) on a node.
   * Derived from node status via gRPC.
   */
  async function listCores(nodeId: number): Promise<CoreStatus[]> {
    try {
      const res = await get<CoresListResponse>(`/api/admin/knode/nodes/${nodeId}`)
      return res.cores || []
    } catch {
      return []
    }
  }

  /**
   * Enable a core (VPN protocol) on a node.
   * POST /api/nodes/{id}/cores/install → { ok }
   */
  async function enableCore(nodeId: number, coreName: string, port: number): Promise<boolean> {
    try {
      await post<NodeMutationResponse>(`/api/nodes/${nodeId}/cores/install`, {
        name: coreName,
        port,
      })
      return true
    } catch {
      return false
    }
  }

  /**
   * Disable/remove a core from a node.
   * DELETE /api/nodes/{id}/cores/{name} → { ok }
   */
  async function disableCore(nodeId: number, coreName: string): Promise<boolean> {
    try {
      await del<NodeMutationResponse>(`/api/nodes/${nodeId}/cores/${coreName}`)
      return true
    } catch {
      return false
    }
  }

  // ─── Sessions ─────────────────────────────────────────────────────────────

  /**
   * List active VPN sessions on a node.
   * GET /api/admin/nodes/sessions?node_id=X → { ok, sessions }
   */
  async function listSessions(nodeId: number): Promise<VPNSession[]> {
    try {
      const res = await get<SessionsListResponse>(`/api/admin/nodes/sessions?node_id=${nodeId}`)
      return res.sessions || []
    } catch {
      return []
    }
  }

  /**
   * Disconnect a user session.
   * DELETE /api/admin/nodes/sessions → { ok }
   */
  async function disconnectUser(
    nodeId: number,
    username: string,
    coreFilter?: string
  ): Promise<boolean> {
    try {
      await del<NodeMutationResponse>('/api/admin/nodes/sessions', {
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ node_id: nodeId, username, core: coreFilter }),
      })
      return true
    } catch {
      return false
    }
  }

  // ─── Firewall ─────────────────────────────────────────────────────────────

  /**
   * List firewall rules on a node.
   * GET /api/admin/nodes/firewall?node_id=X → { ok, rules }
   */
  async function listFirewallRules(nodeId: number): Promise<FirewallRule[]> {
    try {
      const res = await get<FirewallListResponse>(`/api/admin/nodes/firewall?node_id=${nodeId}`)
      return res.rules || []
    } catch {
      return []
    }
  }

  /**
   * Open a port on a node's firewall.
   * POST /api/admin/nodes/firewall → { ok }
   */
  async function openPort(
    nodeId: number,
    port: number,
    protocol: string,
    comment: string
  ): Promise<boolean> {
    try {
      await post<NodeMutationResponse>('/api/admin/nodes/firewall', {
        node_id: nodeId,
        port,
        protocol,
        comment,
      })
      return true
    } catch {
      return false
    }
  }

  /**
   * Close a port on a node's firewall.
   * DELETE /api/admin/nodes/firewall → { ok }
   */
  async function closePort(nodeId: number, port: number, protocol: string): Promise<boolean> {
    try {
      await del<NodeMutationResponse>('/api/admin/nodes/firewall', {
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ node_id: nodeId, port, protocol }),
      })
      return true
    } catch {
      return false
    }
  }

  // ─── Tunnels ──────────────────────────────────────────────────────────────

  /**
   * List outbound tunnels on a node.
   * GET /api/admin/nodes/tunnels?node_id=X → { ok, tunnels }
   */
  async function listTunnels(nodeId: number): Promise<Tunnel[]> {
    try {
      const res = await get<TunnelsListResponse>(`/api/admin/nodes/tunnels?node_id=${nodeId}`)
      return res.tunnels || []
    } catch {
      return []
    }
  }

  /**
   * Setup an outbound tunnel on a node.
   * POST /api/admin/nodes/tunnels → { ok }
   */
  async function setupTunnel(nodeId: number, config: TunnelConfig): Promise<boolean> {
    try {
      await post<NodeMutationResponse>('/api/admin/nodes/tunnels', {
        node_id: nodeId,
        ...config,
      })
      return true
    } catch {
      return false
    }
  }

  /**
   * Teardown an outbound tunnel.
   * DELETE /api/admin/nodes/tunnels → { ok }
   */
  async function teardownTunnel(nodeId: number, tunnelId: string): Promise<boolean> {
    try {
      await del<NodeMutationResponse>('/api/admin/nodes/tunnels', {
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ node_id: nodeId, tunnel_id: tunnelId }),
      })
      return true
    } catch {
      return false
    }
  }

  // ─── Certificates ─────────────────────────────────────────────────────────

  /**
   * Get certificate info for all cores on a node.
   * GET /api/admin/nodes/certs?node_id=X → { ok, certs }
   */
  async function getCertInfo(nodeId: number): Promise<CertInfo[]> {
    try {
      const res = await get<CertsInfoResponse>(`/api/admin/nodes/certs?node_id=${nodeId}`)
      return res.certs || []
    } catch {
      return []
    }
  }

  /**
   * Push new certificates for a specific core on a node.
   * POST /api/admin/nodes/certs → { ok }
   */
  async function pushCerts(
    nodeId: number,
    coreType: string,
    ca: string,
    cert: string,
    key: string
  ): Promise<boolean> {
    try {
      await post<NodeMutationResponse>('/api/admin/nodes/certs', {
        node_id: nodeId,
        core_type: coreType,
        ca_pem: ca,
        cert_pem: cert,
        key_pem: key,
      })
      return true
    } catch {
      return false
    }
  }

  // ─── Expose ───────────────────────────────────────────────────────────────
  return {
    // State
    list,
    loading,
    error,

    // Node CRUD
    loadNodes,
    createNode,
    updateNode,
    deleteNode,
    testConnection,

    // Core management
    listCores,
    enableCore,
    disableCore,

    // Sessions
    listSessions,
    disconnectUser,

    // Firewall
    listFirewallRules,
    openPort,
    closePort,

    // Tunnels
    listTunnels,
    setupTunnel,
    teardownTunnel,

    // Certificates
    getCertInfo,
    pushCerts,
  }
})
