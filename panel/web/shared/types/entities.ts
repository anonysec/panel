/**
 * Shared entity type definitions for KorisPanel
 * Consolidated from both Admin Panel and Customer Portal
 */

export interface Customer {
  id: number
  username: string
  display_name: string
  status: 'active' | 'disabled' | 'expired' | 'limited'
  plan_id?: number | null
  plan: string
  credit: number
  avatar: string
  created_by: string
  created_at: string
}

export interface RadiusCheck {
  id: number
  attribute: string
  op: string
  value: string
}

export interface Subscription {
  id: number
  plan_id: number
  plan_name: string
  start_date: string
  end_date: string
  data_limit_gb: number
  data_used_gb: number
}

export interface SubscriptionHistory {
  id: number
  plan_name: string
  started_at: string
  expires_at: string
  status: string
}

export interface WalletTransaction {
  id: number
  amount: number
  type: string
  description: string
  created_at: string
}

/** Connected client session (live) */
export interface ConnectedClient {
  ip: string
  device: string | null
  user_agent: string | null
  connected_at: string
  protocol: string
}

/** Wallet adjustment request */
export interface WalletAdjustRequest {
  amount: number        // positive for top-up, negative for deduct
  description: string
}

export interface CustomerDetail extends Customer {
  notes: string
  sub_token: string
  radius_checks: RadiusCheck[]
  radius_replies: RadiusCheck[]
  subscription?: Subscription
  subscriptions: SubscriptionHistory[]
  wallet_transactions: WalletTransaction[]
  billing_enabled?: boolean
}

export interface Plan {
  id: number
  name: string
  data_gb: number
  speed_mbps: number
  duration_days: number
  price: number
  billing_type: 'quota' | 'payg'
  price_per_gb: number
  price_per_day: number
  disconnect_on_zero: boolean
  is_active: boolean
  sort_order: number
  created_at: string
}

export interface NodeService {
  name: string
  status: string
}

export interface BandwidthSnapshot {
  timestamp: string
  rx_bps: number
  tx_bps: number
}

export interface NodeMetrics {
  cpu_percent: number
  ram_percent: number
  disk_percent: number
  rx_bps: number
  tx_bps: number
  openvpn_status: string
  l2tp_status: string
  ikev2_status: string
  updated_at: string
}

export interface NodeItem {
  id: number
  name: string
  public_ip: string
  domain: string
  status: 'online' | 'offline' | 'disabled'
  last_seen_at: string
  created_at: string
  status_metrics: NodeMetrics
  services: NodeService[]
  history?: BandwidthSnapshot[]
  agent_version?: string
  group_id?: number | null
  max_capacity?: number
  bandwidth_quota_gb?: number | null
  bandwidth_used_bytes?: number
}

export interface Ticket {
  id: number
  customer_id?: number
  username: string
  subject: string
  status: 'open' | 'closed' | 'pending'
  priority: 'low' | 'normal' | 'high' | 'urgent'
  created_at: string
  updated_at: string
  closed_at: string
}

export interface Payment {
  id: number
  username: string
  amount: number
  method: string
  status: 'pending' | 'approved' | 'rejected'
  intent_type: string
  intent_id?: number
  intent_label: string
  created_at: string
  updated_at: string
}
