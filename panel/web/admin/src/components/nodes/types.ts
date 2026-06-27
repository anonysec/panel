/**
 * TypeScript interfaces for VPN Core Management UI components.
 */

/** Represents the runtime state of a VPN core on a node. */
export interface CoreInfo {
  type: string // "openvpn" | "wireguard" | "l2tp" | "ikev2" | "ssh"
  state: 'running' | 'stopped' | 'crashed' | 'unknown'
  port: number
  activeSessions: number
  lastError?: string
}

/** Configuration payload for enabling/updating a VPN core. */
export interface CoreConfig {
  type: string
  listenPort: number
  extra: Record<string, any>
}
