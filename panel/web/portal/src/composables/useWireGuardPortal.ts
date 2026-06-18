import { ref } from 'vue'
import { useApi } from '@koris/composables/useApi'

export interface WireGuardPeer {
  id: number
  node_name: string
  allowed_ips: string
  status: string
  created_at: string
}

interface PeersResponse {
  ok: boolean
  peers: WireGuardPeer[]
}

/**
 * Composable for WireGuard portal operations.
 * Provides peer listing, config download, and QR code URL generation.
 *
 * Validates: Requirements 8.1, 8.2, 8.4
 */
export function useWireGuardPortal() {
  const { get, loading } = useApi()
  const peers = ref<WireGuardPeer[]>([])

  async function fetchMyPeers(): Promise<void> {
    try {
      const res = await get<PeersResponse>('/api/portal/wireguard/peers')
      peers.value = res.peers || []
    } catch {
      peers.value = []
    }
  }

  function downloadConfig(peerId: number): void {
    window.location.href = `/api/portal/wireguard/peers/${peerId}/config`
  }

  function getQRCodeUrl(peerId: number): string {
    return `/api/portal/wireguard/peers/${peerId}/qr`
  }

  return {
    peers,
    loading,
    fetchMyPeers,
    downloadConfig,
    getQRCodeUrl,
  }
}
