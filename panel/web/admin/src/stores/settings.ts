import { ref } from 'vue'
import { defineStore } from 'pinia'
import { useApi } from '@koris/composables/useApi'

/**
 * Panel settings aggregate interface — matches GET /api/admin/settings response
 */
export interface PanelSettings {
  database: {
    backend: 'timescaledb' | 'postgresql' | 'mariadb' | 'sqlite'
    connected: boolean
    version: string
    timescaleVersion?: string
    hypertableEnabled?: boolean
  }
  tls: {
    mode: 'acme' | 'self-signed' | 'manual'
    domain: string
    expiresAt: string
    issuer: string
    acmeAccount?: string
    lastRenewal?: string
  }
  workers: {
    configured: number
    active: number
    leaderId: string
    currentWorkerId: string
    healthStatus: string
  }
  alerts: {
    cpuThreshold: number
    ramThreshold: number
    diskThreshold: number
  }
  grpc: {
    connectTimeout: number
    keepaliveInterval: number
    metricsInterval: number
  }
  panelInfo: {
    version: string
    edition: 'full' | 'lite'
    uptime: number
    goVersion: string
    migrationVersion: number
  }
}

/**
 * API response types
 */
interface SettingsResponse {
  ok: boolean
  settings: PanelSettings
}

interface MutationResponse {
  ok: boolean
  restart_required?: boolean
}

/**
 * Settings store — aggregated panel settings for Database, TLS, Workers, Alerts, gRPC, and Panel Info.
 *
 * Requirements: 13.1, 14.1, 15.1, 16.1, 17.1, 18.1
 */
export const useSettingsStore = defineStore('settings', () => {
  // ─── State ────────────────────────────────────────────────────────────────
  const settings = ref<PanelSettings | null>(null)
  const loading = ref(false)

  // ─── API composable ───────────────────────────────────────────────────────
  const { get, post, error } = useApi()

  // ─── Actions ──────────────────────────────────────────────────────────────

  /**
   * Load aggregated settings from the backend.
   * GET /api/admin/settings → { ok, settings: PanelSettings }
   */
  async function loadSettings(): Promise<void> {
    loading.value = true
    try {
      const res = await get<SettingsResponse>('/api/admin/settings')
      if (res?.ok) {
        settings.value = res.settings
      }
    } catch {
      // Preserve existing data on error
    } finally {
      loading.value = false
    }
  }

  /**
   * Update alert thresholds.
   * POST /api/admin/settings/alerts → { ok }
   *
   * @param thresholds - { cpu: 1-100, ram: 1-100, disk: 1-100 }
   * @returns true on success
   */
  async function updateAlerts(thresholds: {
    cpu: number
    ram: number
    disk: number
  }): Promise<boolean> {
    loading.value = true
    try {
      await post<MutationResponse>('/api/admin/settings/alerts', thresholds)
      // Update local state
      if (settings.value) {
        settings.value.alerts = {
          cpuThreshold: thresholds.cpu,
          ramThreshold: thresholds.ram,
          diskThreshold: thresholds.disk,
        }
      }
      return true
    } catch {
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * Update gRPC parameters.
   * POST /api/admin/settings/grpc → { ok, restart_required }
   *
   * @param params - { connectTimeout, keepaliveInterval, metricsInterval } in seconds
   * @returns Object with success flag and whether restart is required
   */
  async function updateGrpc(params: {
    connectTimeout: number
    keepaliveInterval: number
    metricsInterval: number
  }): Promise<{ success: boolean; restartRequired?: boolean }> {
    loading.value = true
    try {
      const res = await post<MutationResponse>('/api/admin/settings/grpc', {
        connect_timeout: params.connectTimeout,
        keepalive_interval: params.keepaliveInterval,
        metrics_interval: params.metricsInterval,
      })
      // Update local state
      if (settings.value) {
        settings.value.grpc = {
          connectTimeout: params.connectTimeout,
          keepaliveInterval: params.keepaliveInterval,
          metricsInterval: params.metricsInterval,
        }
      }
      return { success: true, restartRequired: res.restart_required }
    } catch {
      return { success: false }
    } finally {
      loading.value = false
    }
  }

  /**
   * Upload a manual TLS certificate and key.
   * POST /api/admin/settings/tls/upload → { ok }
   *
   * @param cert - PEM-encoded certificate
   * @param key - PEM-encoded private key
   * @returns true on success
   */
  async function uploadTlsCert(cert: string, key: string): Promise<boolean> {
    loading.value = true
    try {
      await post<MutationResponse>('/api/admin/settings/tls/upload', {
        cert_pem: cert,
        key_pem: key,
      })
      return true
    } catch {
      return false
    } finally {
      loading.value = false
    }
  }

  // ─── Expose ───────────────────────────────────────────────────────────────
  return {
    settings,
    loading,
    error,
    loadSettings,
    updateAlerts,
    updateGrpc,
    uploadTlsCert,
  }
})
