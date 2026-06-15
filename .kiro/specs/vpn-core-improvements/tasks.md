# Tasks: VPN Core Improvements

## Task Group 1: SQL Injection Prevention in OpenVPN Scripts

- [x] 1.1 Add strict regex validation for `common_name` (`^[A-Za-z0-9_.-]{1,64}$`) at the top of `koris-client-connect.sh` with `exit 1` on failure
- [x] 1.2 Add IPv4 regex validation for `trusted_ip` (`^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$`); default to `"0.0.0.0"` if invalid
- [x] 1.3 Add numeric regex validation for `trusted_port` (`^[0-9]{1,5}$`); default to `"0"` if invalid
- [x] 1.4 Add numeric regex validation for `time_unix` (`^[0-9]+$`); default to `$(date +%s)` if invalid
- [x] 1.5 Pass `SESSION_ID` and `UNIQUE_ID` through `sql_escape()` before interpolation into MySQL heredoc
- [x] 1.6 In `koris-client-disconnect.sh`, validate `bytes_received`, `bytes_sent`, `time_duration` as `^[0-9]+$` with default "0"
- [x] 1.7 Sanitize `signal` (disconnect cause) to `[A-Za-z0-9 _.:-]` max 64 chars, then `sql_escape()`
- [x] 1.8 Add username validation at the top of `koris-client-disconnect.sh` matching the connect script pattern

## Task Group 2: Remove Hardcoded Secrets and IPs

- [x] 2.1 Replace `RADIUS_SECRET="${KORIS_RADIUS_SECRET:-OvpnRad2026}"` with `RADIUS_SECRET="${KORIS_RADIUS_SECRET:?ERROR: KORIS_RADIUS_SECRET must be set}"` in `koris-radius-auth.sh`
- [x] 2.2 Replace `NAS_IP="${KORIS_NAS_IP:-91.107.168.34}"` with auto-detection logic using `ip -4 route get 1.1.1.1` and fatal exit on failure
- [x] 2.3 Remove all hardcoded `91.107.168.34` references from `koris-client-connect.sh` and `koris-client-disconnect.sh`, replacing with `"${KORIS_NAS_IP:?ERROR: KORIS_NAS_IP must be set}"`
- [x] 2.4 Update `node-install.sh` to generate a random RADIUS secret (32 chars) and write it to `/etc/panel-node/node.env` during installation
- [x] 2.5 Add `KORIS_NAS_IP` auto-detection to `node-install.sh` using the node's public IP (already detected as `$NODE_IP`)

## Task Group 3: Safe Config Application with Validation

- [ ] 3.1 Create `panel/internal/templates/validate.go` with `ValidatePrivateNetwork(cidr string, allowIPv6 bool) error` function covering RFC1918 and ULA ranges
- [ ] 3.2 Add port validation (1-65535), protocol validation ("udp"/"tcp"), and DNS IP validation helpers in `validate.go`
- [ ] 3.3 Refactor `applyOpenVPNServerConfig()` in `api.go` to call `ValidatePrivateNetwork()` before any file operations
- [ ] 3.4 Ensure backup creation (`{path}.bak.{timestamp}`) happens before any config file overwrite
- [ ] 3.5 Return validation error and skip service restart if any validation check fails
- [ ] 3.6 Write unit tests for `ValidatePrivateNetwork` covering: valid RFC1918 ranges, ULA, public IPs, invalid CIDR syntax, prefix length boundaries

## Task Group 4: WireGuard Protocol Support

- [ ] 4.1 Create migration `014_wireguard.sql`: ALTER `node_vpn_configs.protocol` ENUM to add `'wireguard'`, CREATE `wg_peers` table
- [ ] 4.2 Create `panel/internal/wireguard/keygen.go` with `GenerateKeyPair() (priv, pub string, err error)` using `golang.org/x/crypto/curve25519`
- [ ] 4.3 Add WireGuard API endpoints in `api.go`: `GET/POST /api/wireguard/peers`, `DELETE /api/wireguard/peers/{id}`, `GET /api/wireguard/peers/{id}/config`
- [ ] 4.4 Implement `GenerateClientConfig()` that produces a complete `.conf` file for download
- [ ] 4.5 Add task actions `wireguard.add_peer`, `wireguard.remove_peer`, `wireguard.sync_config` to node agent `executeTask()`
- [ ] 4.6 Implement WireGuard peer management in node agent using `wg set` and `wg-quick` commands
- [ ] 4.7 Add WireGuard status reporting (`wg show wg0`) to node agent push cycle alongside existing service status checks
- [ ] 4.8 Update `node-install.sh` to install `wireguard-tools` in the dependency list for both Debian and RHEL families
- [ ] 4.9 Add `wireguard` to `normalizeService()` mapping in node agent (map to "wg-quick@wg0")

## Task Group 5: IPv6 Dual-Stack Support

- [ ] 5.1 Create migration `015_ipv6.sql`: ADD `network_ipv6 VARCHAR(64) NULL` column to `node_vpn_configs`
- [ ] 5.2 Extend `ValidatePrivateNetwork()` to handle IPv6 ULA validation (fc00::/7, prefix /48-/112)
- [ ] 5.3 Update OpenVPN template to include `server-ipv6` directive when `network_ipv6` is configured
- [ ] 5.4 Add IPv6 tc filter rules in `koris-client-connect.sh`: `tc filter add ... protocol ipv6 ...` alongside IPv4 rules
- [ ] 5.5 Update WireGuard peer `allowed_ips` to support dual-stack format (e.g., `10.11.0.2/32,fd00:koris::2/128`)
- [ ] 5.6 Add `DNS1v6` and `DNS2v6` fields to VPN settings and push as `dhcp-option DNS6` in templates
- [ ] 5.7 Update node push struct to include IPv6 address in service status

## Task Group 6: Config Templating System

- [ ] 6.1 Create `panel/internal/templates/engine.go` with `TemplateEngine` struct, `NewEngine(basePath)`, and `Render(protocol, vars)` method
- [ ] 6.2 Define `TemplateVars` struct with all config fields (Port, Protocol, Network, NetworkIPv6, DNS1, DNS2, DNS1v6, DNS2v6, IPSecPSK, ServerIP, ExtraJSON)
- [ ] 6.3 Create template files: `openvpn.conf.tmpl`, `strongswan.conf.tmpl`, `xl2tpd.conf.tmpl`, `wireguard.conf.tmpl`
- [ ] 6.4 Implement `Validate(protocol, rendered)` that performs protocol-specific syntax checking
- [ ] 6.5 Implement `Diff(current, proposed)` that returns a unified diff string for admin preview
- [ ] 6.6 Implement `Apply(protocol, confPath, vars)` that validates → renders → backs up → writes
- [ ] 6.7 Refactor `applyOpenVPNServerConfig()` to use `TemplateEngine.Apply()` instead of line-by-line manipulation
- [ ] 6.8 Add `PANEL_TEMPLATE_DIR` environment variable to config loader (default: `/etc/koris/templates/`)
- [ ] 6.9 Write unit tests for template rendering with valid and invalid vars

## Task Group 7: Real-Time Bandwidth Display

- [ ] 7.1 Create `node/bandwidth/collector.go` with `Collector` struct and `Collect()` method that parses `tc -s class show dev` output
- [ ] 7.2 Implement delta rate calculation in `DeltaRates()` with counter-wrap clamping (negative → 0)
- [ ] 7.3 Add `PerUserBandwidth []UserBandwidth` field to node Push struct
- [ ] 7.4 Integrate bandwidth collection into node agent main loop (collect on each push cycle)
- [ ] 7.5 Create migration `016_bandwidth_snapshots.sql`: CREATE `user_bandwidth_snapshots` table
- [ ] 7.6 Add panel-side handler to store bandwidth snapshots from node push and map class IPs to usernames
- [ ] 7.7 Extend WebSocket `/api/realtime` broadcast to include per-user bandwidth data
- [ ] 7.8 Write unit tests for tc output parsing and rate calculation

## Task Group 8: Automatic Certificate Rotation

- [ ] 8.1 Create migration `017_cert_expiry.sql`: ADD `expires_at DATETIME NULL` and `fingerprint VARCHAR(128) NULL` to `vpn_certificates`, add index on `expires_at`
- [ ] 8.2 Create `panel/internal/certrotation/worker.go` with `Worker` struct and `Start()` method (launches goroutine with 1-hour ticker)
- [ ] 8.3 Implement `CheckExpiring()` that queries certs with `expires_at < NOW() + INTERVAL 30 DAY`
- [ ] 8.4 Implement `Regenerate(cert)` that calls `openssl` or `easy-rsa` to generate new CA/TLS certs
- [ ] 8.5 Implement `DistributeToNodes()` that creates `cert.distribute` tasks for affected nodes
- [ ] 8.6 Add `cert.distribute` task action to node agent: write cert file to disk, validate chain with `openssl verify`
- [ ] 8.7 Generate warning events for certs expiring within 30 days, critical events within 7 days
- [ ] 8.8 Initialize cert rotation worker in panel `main.go` startup

## Task Group 9: Connection Limit Double-Enforcement

- [ ] 9.1 In `koris-radius-auth.sh`, after customer status checks and before RADIUS auth, query `radcheck` for `Simultaneous-Use` value
- [ ] 9.2 Query `radacct` for `COUNT(*) WHERE username=X AND acctstoptime IS NULL`
- [ ] 9.3 If `active_sessions >= max_sessions` and `max_sessions > 0`, call `reject "$USER_NAME" "connection limit reached: active=$active max=$max"`
- [ ] 9.4 If DB query fails, log the error and continue (fail-open to let RADIUS handle it)
- [ ] 9.5 Add logging: `echo "$(date -Is) CONN_LIMIT user=$USER_NAME active=$active max=$max" >> "$LOG"`

## Task Group 10: Log Rotation Configuration

- [ ] 10.1 Create `scripts/logrotate/koris-openvpn` logrotate config file with daily rotation, 14-day retention, compress, delaycompress, missingok, notifempty
- [ ] 10.2 Add `postrotate` script that signals OpenVPN to reopen logs (`systemctl reload openvpn@server || killall -USR1 openvpn || true`)
- [ ] 10.3 Update `node-install.sh` to copy logrotate config to `/etc/logrotate.d/koris-openvpn` during installation
- [ ] 10.4 Ensure `node-install.sh` creates `/var/log/openvpn/` directory with permissions 0750 if it doesn't exist
- [ ] 10.5 Add logrotate config for node agent logs if applicable (`/var/log/panel-node/` or journald confirmation)
