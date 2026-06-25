import { ref } from 'vue'
import { defineStore } from 'pinia'
import { useApi } from '@koris/composables/useApi'

/**
 * Active VPN connection for a portal customer
 */
export interface VPNConnection {
  protocol: string
  nodeName: string
  assignedIp: string
  duration: number
  rxBytes: number
  txBytes: number
}

/**
 * Data usage and quota information
 */
export interface UsageInfo {
  usedBytes: number
  limitBytes: number
  periodStart: string
  periodEnd: string
}

/**
 * API response for GET /api/portal/connections
 */
interface ConnectionsResponse {
  ok: boolean
  connections: Array<{
    protocol: string
    node_name: string
    assigned_ip: string
    duration: number
    rx_bytes: number
    tx_bytes: number
  }>
  usage: {
    used_bytes: number
    limit_bytes: number
    period_start: string
    period_end: string
  }
}

/**
 * Portal connections store — customer's active VPN connections and config downloads.
 *
 * Requirements: 20.1, 20.2, 21.1, 21.2, 21.3
 */
export const useConnectionsStore = defineStore('connections', () => {
  // ─── State ────────────────────────────────────────────────────────────────
  const connections = ref<VPNConnection[]>([])
  const usage = ref<UsageInfo | null>(null)
  const loading = ref(false)

  // ─── API composable ───────────────────────────────────────────────────────
  const { get, error } = useApi()

  // ─── Actions ──────────────────────────────────────────────────────────────

  /**
   * Load the customer's active VPN connections and usage data.
   * GET /api/portal/connections → { ok, connections, usage }
   */
  async function loadConnections(): Promise<void> {
    loading.value = true
    try {
      const res = await get<ConnectionsResponse>('/api/portal/connections')
      if (res?.ok) {
        connections.value = (res.connections || []).map(c => ({
          protocol: c.protocol,
          nodeName: c.node_name,
          assignedIp: c.assigned_ip,
          duration: c.duration,
          rxBytes: c.rx_bytes,
          txBytes: c.tx_bytes,
        }))
        if (res.usage) {
          usage.value = {
            usedBytes: res.usage.used_bytes,
            limitBytes: res.usage.limit_bytes,
            periodStart: res.usage.period_start,
            periodEnd: res.usage.period_end,
          }
        }
      }
    } catch {
      // Preserve existing data on error
    } finally {
      loading.value = false
    }
  }

  /**
   * Download a VPN configuration file for the specified protocol.
   * GET /api/portal/configs/{protocol} → file download
   *
   * Triggers a browser file download via Blob URL.
   */
  async function downloadConfig(protocol: string): Promise<boolean> {
    try {
      const response = await fetch(`/api/portal/configs/${protocol}`, {
        method: 'GET',
        credentials: 'same-origin',
      })

      if (!response.ok) {
        return false
      }

      // Get filename from Content-Disposition header or use default
      const disposition = response.headers.get('Content-Disposition')
      let filename = `${protocol}.conf`
      if (disposition) {
        const match = disposition.match(/filename[^;=\n]*=((['"]).*?\2|[^;\n]*)/)
        if (match?.[1]) {
          filename = match[1].replace(/['"]/g, '')
        }
      }

      // Create a download via Blob URL
      const blob = await response.blob()
      const url = window.URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = filename
      document.body.appendChild(link)
      link.click()
      document.body.removeChild(link)
      window.URL.revokeObjectURL(url)

      return true
    } catch {
      return false
    }
  }

  // ─── Expose ───────────────────────────────────────────────────────────────
  return {
    connections,
    usage,
    loading,
    error,
    loadConnections,
    downloadConfig,
  }
})
