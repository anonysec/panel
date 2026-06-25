import { ref, shallowRef, triggerRef } from 'vue'
import { defineStore } from 'pinia'
import { useWebSocket } from '@koris/composables/useWebSocket'
import { useApi } from '@koris/composables/useApi'

/**
 * Per-node real-time metrics state
 */
export interface NodeMetricsState {
  nodeId: number
  name: string
  status: 'online' | 'stale' | 'offline'
  cpu: number
  ram: number
  disk: number
  rxBps: number
  txBps: number
  sessions: number
  uptime: number
  lastUpdated: string
}

/**
 * Active alert triggered when a metric exceeds its threshold
 */
export interface Alert {
  nodeId: number
  nodeName: string
  type: 'cpu' | 'ram' | 'disk'
  value: number
  threshold: number
  since: string
}

/**
 * Historical metrics data point (from /api/admin/nodes/{id}/metrics/history)
 */
export interface MetricsHistoryPoint {
  ts: string
  cpu: number
  ram: number
  disk: number
  rx_bps: number
  tx_bps: number
}

/**
 * API response for historical metrics
 */
interface MetricsHistoryResponse {
  ok: boolean
  data: MetricsHistoryPoint[]
}

/**
 * Real-time metrics store — receives node metrics via WebSocket and manages alerts.
 *
 * Connects to the existing /api/ws/realtime endpoint which now emits
 * `node_metrics` and `node_status_change` message types.
 *
 * Requirements: 11.1, 11.2, 11.3, 11.4, 11.5, 12.1, 12.2, 12.3
 */
export const useMetricsStore = defineStore('metrics', () => {
  // ─── State ────────────────────────────────────────────────────────────────
  const nodes = shallowRef<Map<number, NodeMetricsState>>(new Map())
  const alerts = ref<Alert[]>([])
  const thresholds = ref({ cpu: 90, ram: 90, disk: 85 })

  // ─── WebSocket ────────────────────────────────────────────────────────────
  const wsUrl = typeof window !== 'undefined'
    ? `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/api/ws/realtime`
    : 'ws://localhost/api/ws/realtime'

  const { connected, connect, disconnect } = useWebSocket({
    url: wsUrl,
    autoConnect: false,
    reconnect: true,
    maxReconnectAttempts: 10,
    onMessage: handleMessage,
  })

  // ─── Message Handling ─────────────────────────────────────────────────────

  /**
   * Processes incoming WebSocket messages.
   * Handles `node_metrics` and `node_status_change` message types.
   */
  function handleMessage(data: any): void {
    if (!data || typeof data !== 'object') return

    if (data.type === 'node_metrics' && data.data) {
      handleMetricsMessage(data.data)
    }

    if (data.type === 'node_status_change' && data.data) {
      handleStatusChange(data.data)
    }
  }

  /**
   * Processes a node_metrics payload.
   * Updates the nodes map and re-evaluates alert state.
   */
  function handleMetricsMessage(m: any): void {
    const updated = new Map(nodes.value)
    updated.set(m.node_id, {
      nodeId: m.node_id,
      name: m.name ?? '',
      status: m.status ?? 'online',
      cpu: m.cpu_percent ?? 0,
      ram: m.ram_percent ?? 0,
      disk: m.disk_percent ?? 0,
      rxBps: m.rx_bps ?? 0,
      txBps: m.tx_bps ?? 0,
      sessions: m.sessions ?? 0,
      uptime: m.uptime ?? 0,
      lastUpdated: new Date().toISOString(),
    })
    nodes.value = updated
    triggerRef(nodes)
    updateAlerts(m)
  }

  /**
   * Processes a node_status_change payload.
   * Updates the node's status in the map.
   */
  function handleStatusChange(data: any): void {
    const existing = nodes.value.get(data.node_id)
    if (existing) {
      const updated = new Map(nodes.value)
      updated.set(data.node_id, {
        ...existing,
        status: data.new_status,
        lastUpdated: data.timestamp || new Date().toISOString(),
      })
      nodes.value = updated
      triggerRef(nodes)
    }
  }

  /**
   * Compares metric values against thresholds.
   * Adds alerts when values exceed thresholds, removes them when they drop.
   *
   * Requirement 12.3: alert removed when value <= threshold
   */
  function updateAlerts(m: any): void {
    const nodeId = m.node_id as number
    const nodeName = (m.name ?? '') as string
    const now = new Date().toISOString()

    const metricChecks: Array<{ type: 'cpu' | 'ram' | 'disk'; value: number; threshold: number }> = [
      { type: 'cpu', value: m.cpu_percent ?? 0, threshold: thresholds.value.cpu },
      { type: 'ram', value: m.ram_percent ?? 0, threshold: thresholds.value.ram },
      { type: 'disk', value: m.disk_percent ?? 0, threshold: thresholds.value.disk },
    ]

    const updated = alerts.value.filter(a => a.nodeId !== nodeId)

    for (const check of metricChecks) {
      if (check.value > check.threshold) {
        // Check if alert already existed (preserve the "since" timestamp)
        const existing = alerts.value.find(
          a => a.nodeId === nodeId && a.type === check.type
        )
        updated.push({
          nodeId,
          nodeName,
          type: check.type,
          value: check.value,
          threshold: check.threshold,
          since: existing?.since ?? now,
        })
      }
    }

    alerts.value = updated
  }

  // ─── Historical Data ──────────────────────────────────────────────────────

  /**
   * Loads historical metrics for a specific node and time range.
   * GET /api/admin/nodes/{nodeId}/metrics/history?range=1h|6h|24h
   */
  async function loadHistorical(
    nodeId: number,
    range: '1h' | '6h' | '24h'
  ): Promise<MetricsHistoryPoint[]> {
    const { get } = useApi()
    try {
      const res = await get<MetricsHistoryResponse>(
        `/api/admin/nodes/${nodeId}/metrics/history?range=${range}`
      )
      return res.data || []
    } catch {
      return []
    }
  }

  // ─── Expose ───────────────────────────────────────────────────────────────
  return {
    nodes,
    alerts,
    connected,
    thresholds,
    handleMetricsMessage,
    updateAlerts,
    loadHistorical,
    connect,
    disconnect,
  }
})
