# Implementation Plan: WireGuard Protocol Support

## Overview

This plan implements the remaining WireGuard VPN protocol features for KorisPanel. The database migration (014_wireguard.sql) and basic peer CRUD API handlers already exist. This plan covers: IP allocation module, validation module, gaming optimize mode, wireguard.setup and wireguard.update_config task handlers, portal endpoints, plan integration (auto-provisioning), status reporting with peer stats, frontend admin views, frontend portal views, config generation improvements (dual-stack, gaming), and property-based tests.

## Tasks

- [x] 1. Backend core modules (validation, IP allocation)
  - [x] 1.1 Create IP allocation module `panel/internal/wireguard/ipalloc.go`
    - Implement `AllocateNextIP(networkCIDR string, usedIPs []string) (string, error)` that finds the next available IP in the subnet
    - Implement `ParseSubnetRange(networkCIDR string) (first, last net.IP, bits int, err error)` for subnet boundary calculation
    - Skip network address, broadcast address (IPv4), and gateway (.1 / ::1) from allocation
    - Support both IPv4 and IPv6 CIDRs
    - _Requirements: 3.2, 3.5, 11.1, 11.4_

  - [x] 1.2 Create validation module `panel/internal/wireguard/validation.go`
    - Implement `ValidatePort(port int) error` — accepts 1024–65535 inclusive
    - Implement `ValidateNetworkCIDR(cidr string) error` — validates via `net.ParseCIDR` for IPv4 and IPv6
    - Implement `ValidateWireGuardKey(key string) error` — 44-char base64 decoding to exactly 32 bytes
    - _Requirements: 1.6, 1.7, 11.1, 11.4_

  - [ ]* 1.3 Write property tests for port validation (Property 1)
    - **Property 1: Port validation accepts only valid ranges**
    - Use `pgregory.net/rapid` to generate random integers; verify acceptance iff 1024 ≤ port ≤ 65535
    - **Validates: Requirements 1.6**

  - [ ]* 1.4 Write property tests for CIDR validation (Property 2)
    - **Property 2: CIDR validation accepts valid IPv4 and IPv6 subnets**
    - Generate random valid/invalid CIDR strings; verify acceptance iff `net.ParseCIDR` succeeds
    - **Validates: Requirements 1.7, 11.1, 11.4**

  - [ ]* 1.5 Write property tests for IP allocation (Property 4)
    - **Property 4: IP allocation returns addresses within subnet that don't conflict**
    - Generate random /24 and /16 subnets with random used-IP sets; verify returned IP is within subnet, not network/broadcast/gateway, and not already used
    - **Validates: Requirements 3.2, 3.5**

  - [ ]* 1.6 Write property tests for key generation (Property 3)
    - **Property 3: Generated keys are valid WireGuard keys**
    - Generate multiple key pairs; verify each is 44-char base64 decoding to exactly 32 bytes
    - **Validates: Requirements 3.1**

- [ ] 2. Backend API enhancements (IP allocation integration, gaming optimize, config improvements)
  - [x] 2.1 Integrate IP allocation into peer creation handler in `panel/internal/api/wireguard.go`
    - Query active peer IPs for the node: `SELECT allowed_ips FROM wg_peers WHERE node_id=? AND status='active'`
    - Fetch node's WireGuard network CIDR from `node_vpn_configs.extra_json`
    - Call `wireguard.AllocateNextIP(networkCIDR, usedIPs)` to auto-assign IP when `allowed_ips` not provided
    - Return `ip_pool_exhausted` error when no IPs available
    - _Requirements: 3.2, 3.5, 3.6_

  - [x] 2.2 Add port and CIDR validation to WireGuard config endpoints
    - Validate port (1024–65535) and network CIDR before inserting/updating `node_vpn_configs`
    - Validate on the existing `nodeVPNConfig` handler when `protocol=wireguard`
    - Return structured 400 errors (`invalid_port`, `invalid_network_cidr`)
    - _Requirements: 1.6, 1.7, 2.4_

  - [x] 2.3 Add gaming optimize support to config generation in `panel/internal/wireguard/clientconfig.go`
    - Extend `ClientConfig` struct with `GamingOptimize bool` and `MTU int` fields
    - When `GamingOptimize=true`: add `MTU = 1280` to [Interface], set `PersistentKeepalive = 15`
    - When `GamingOptimize=false`: keep default behavior (no MTU line, `PersistentKeepalive = 25`)
    - Update `wireguardPeerConfig` handler to read `gaming_optimize` from `node_vpn_configs.extra_json`
    - _Requirements: 7.2, 7.3, 7.6_

  - [ ]* 2.4 Write property test for config round-trip (Property 5)
    - **Property 5: Config file generation round-trip**
    - Generate random valid ClientConfig structs; verify generating then parsing produces matching key-value pairs
    - **Validates: Requirements 6.1, 6.3, 6.5**

  - [ ]* 2.5 Write property test for gaming optimize config (Property 9)
    - **Property 9: Gaming optimize config transformation**
    - Generate configs with gaming_optimize=true/false; verify MTU and PersistentKeepalive values match expected
    - **Validates: Requirements 7.2, 7.3, 7.6**

  - [ ] 2.6 Add dual-stack support to config generation
    - Support comma-separated IPv4+IPv6 addresses in the `Address` field
    - When node has both `network` (IPv4) and `network_ipv6` (IPv6) in extra_json, allocate from both and combine
    - _Requirements: 11.1, 11.2, 11.3_

- [ ] 3. Checkpoint - Ensure all backend core tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 4. Node agent task handlers (setup, update_config, gaming optimize)
  - [x] 4.1 Implement `wireguard.setup` task handler in `node/cmd/node/main.go`
    - Accept payload: `{port, network, dns_1, dns_2, mtu}`
    - Generate server key pair using `wg genkey | wg pubkey`
    - Write `/etc/wireguard/wg0.conf` with [Interface] section (PrivateKey, Address=network.1/cidr, ListenPort, DNS)
    - Run `systemctl enable wg-quick@wg0` and `wg-quick up wg0`
    - Return `server_public_key` in task completion response
    - On failure, return descriptive error message
    - _Requirements: 1.1, 1.2, 1.3, 1.5_

  - [ ] 4.2 Implement `wireguard.update_config` task handler in `node/cmd/node/main.go`
    - Accept payload: `{port, network, dns_1, dns_2, mtu, gaming_optimize}`
    - Rewrite [Interface] section of `/etc/wireguard/wg0.conf` preserving [Peer] blocks
    - Run `wg syncconf wg0 <stripped_conf>` to apply changes without dropping peers
    - If `gaming_optimize=true`: apply fwmark 51820, ip rule priority 100, MTU 1280
    - If `gaming_optimize=false`: remove fwmark rules, revert MTU to specified/1420
    - _Requirements: 2.2, 2.3, 7.4, 7.5, 12.4_

  - [ ] 4.3 Extend node status push with WireGuard peer statistics
    - Parse output of `wg show wg0 dump` to extract per-peer latest-handshake and transfer bytes
    - Add `wireguard_peers` array and `wireguard_active_peers` count to push payload
    - Active peer = last handshake within 3 minutes of current time
    - _Requirements: 5.3, 10.1, 10.2_

  - [ ]* 4.4 Write property test for removePeerFromConfig (Property 7)
    - **Property 7: removePeerFromConfig preserves other peers**
    - Generate random multi-peer config files; verify removing one peer preserves all others exactly
    - **Validates: Requirements 4.3**

  - [ ]* 4.5 Write property test for active peer count (Property 10)
    - **Property 10: WireGuard status active peer count**
    - Generate random peer lists with various handshake timestamps; verify count equals peers with handshake within 3 minutes
    - **Validates: Requirements 5.3, 10.2**

- [ ] 5. Backend API — Portal endpoints and status ingestion
  - [ ] 5.1 Implement portal WireGuard peer list endpoint `GET /api/portal/wireguard/peers`
    - Return only peers where `customer_id` matches authenticated customer session
    - Include peer ID, node info, status, allowed_ips, created_at
    - _Requirements: 8.1_

  - [ ] 5.2 Implement portal config download `GET /api/portal/wireguard/peers/{id}/config`
    - Verify peer belongs to authenticated customer (return 403 if not)
    - Generate and serve .conf file with correct Content-Type and Content-Disposition headers
    - _Requirements: 8.2, 8.3, 6.2_

  - [ ] 5.3 Implement portal QR code endpoint `GET /api/portal/wireguard/peers/{id}/qr`
    - Generate QR code PNG from the config file content
    - Use a lightweight Go QR library (e.g., `github.com/skip2/go-qrcode`)
    - Verify peer belongs to authenticated customer (403 if not)
    - _Requirements: 8.4_

  - [ ] 5.4 Implement peer status ingestion from node push
    - In the existing `/api/node/push` handler, parse `wireguard_peers` array from payload
    - Update `wg_peers` table: `last_handshake_at`, `rx_bytes`, `tx_bytes` for each reported peer (matched by public_key + node_id)
    - Update node's WireGuard service status display data
    - _Requirements: 5.3, 5.4, 10.1, 10.3_

  - [ ]* 5.5 Write property test for portal peer isolation (Property 8)
    - **Property 8: Customer portal peer isolation**
    - Generate random multi-customer peer sets; verify query returns only peers for the given customer_id
    - File: `panel/web/shared/src/__tests__/wireguard.test.ts` using fast-check
    - **Validates: Requirements 8.1, 8.3**

- [ ] 6. Plan integration (auto-provision and revoke)
  - [ ] 6.1 Implement auto-provisioning on subscription activation
    - When a subscription is created/activated on a WireGuard-enabled node, auto-create a WireGuard peer for the customer
    - Hook into existing subscription creation flow in `panel/internal/api/api.go`
    - Call peer creation logic (generate keys, allocate IP, dispatch wireguard.add_peer task)
    - _Requirements: 9.3_

  - [ ] 6.2 Implement auto-revocation on subscription termination
    - When a subscription is terminated/expired, revoke associated WireGuard peers
    - Set peer status to 'revoked', dispatch `wireguard.remove_peer` task
    - Release IP back to pool (revoked peers excluded from active query)
    - _Requirements: 9.4, 4.1, 4.4_

  - [ ]* 6.3 Write property test for revocation frees IP (Property 6)
    - **Property 6: Peer revocation makes IP available for reallocation**
    - Generate random active peer sets, revoke one, verify its IP is no longer in the "used" set
    - **Validates: Requirements 4.1, 4.4**

- [ ] 7. Checkpoint - Ensure all backend tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 8. Frontend admin — WireGuard management views
  - [ ] 8.1 Create WireGuard config panel component `panel/web/admin/src/components/WireGuardConfig.vue`
    - Form fields: listen port, network CIDR, primary DNS, secondary DNS, MTU
    - Gaming Optimize toggle
    - Enable/Disable WireGuard toggle (without removing peers)
    - Save triggers POST to `/api/nodes/vpn-config/{nodeId}`
    - Integrate into existing node detail view (NodesView.vue)
    - _Requirements: 2.1, 2.5, 7.1, 9.2_

  - [ ] 8.2 Create WireGuard peer list view `panel/web/admin/src/views/WireGuardPeersView.vue`
    - Table columns: ID, customer username, node, public key (truncated), allowed IPs, status, last handshake, RX/TX bytes
    - Filters: by node, by status (active/revoked), by customer
    - Download config button per peer
    - Delete (revoke) button per peer
    - Register route in admin router
    - _Requirements: 5.1, 5.2_

  - [ ] 8.3 Create peer creation dialog `panel/web/admin/src/components/WireGuardPeerCreate.vue`
    - Fields: select node (WireGuard-enabled only), optionally select customer
    - IP auto-assigned (show assigned IP after creation)
    - Triggers POST to `/api/wireguard/peers`
    - _Requirements: 3.1, 3.2, 3.6_

  - [ ] 8.4 Create `useWireGuard` composable `panel/web/admin/src/composables/useWireGuard.ts`
    - Functions: `fetchPeers(filters)`, `createPeer(data)`, `deletePeer(id)`, `downloadConfig(id)`, `saveNodeWireGuardConfig(nodeId, config)`
    - _Requirements: 5.1, 3.1, 4.1, 6.1, 2.1_

  - [ ] 8.5 Display WireGuard service status on node detail/list
    - Show WireGuard service status badge (running/stopped/error) on the node list and detail views
    - Show warning indicator when WireGuard configured but service not running
    - _Requirements: 10.3, 10.4_

- [ ] 9. Frontend portal — WireGuard download and QR
  - [ ] 9.1 Create WireGuard peers view `panel/web/portal/src/views/WireGuardPeersView.vue`
    - List customer's WireGuard peers with status and node info
    - Download .conf button per peer
    - Show QR code button per peer (opens modal with QR image)
    - Register route in portal router
    - _Requirements: 8.1, 8.2, 8.4_

  - [ ] 9.2 Create `useWireGuardPortal` composable `panel/web/portal/src/composables/useWireGuardPortal.ts`
    - Functions: `fetchMyPeers()`, `downloadConfig(peerId)`, `getQRCodeUrl(peerId)`
    - _Requirements: 8.1, 8.2, 8.4_

- [ ] 10. Final checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- The database migration (014_wireguard.sql) already exists with the ENUM update and wg_peers table, so no migration task is needed
- Existing code already handles: peer CRUD API (list/create/delete), config download, wireguard.add_peer and wireguard.remove_peer task handlers, key generation, basic client config generation
- Property tests use `pgregory.net/rapid` for Go and `fast-check` for TypeScript as specified in the tech stack
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- The QR code feature (task 5.3) requires adding `github.com/skip2/go-qrcode` dependency

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1", "1.2"] },
    { "id": 1, "tasks": ["1.3", "1.4", "1.5", "1.6", "2.2", "4.1"] },
    { "id": 2, "tasks": ["2.1", "2.3", "4.2", "4.3"] },
    { "id": 3, "tasks": ["2.4", "2.5", "2.6", "4.4", "4.5"] },
    { "id": 4, "tasks": ["5.1", "5.2", "5.3", "5.4"] },
    { "id": 5, "tasks": ["5.5", "6.1", "6.2"] },
    { "id": 6, "tasks": ["6.3", "8.4", "9.2"] },
    { "id": 7, "tasks": ["8.1", "8.2", "8.3", "8.5", "9.1"] }
  ]
}
```
