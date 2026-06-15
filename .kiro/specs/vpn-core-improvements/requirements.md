# Requirements Document: VPN Core Improvements

## Requirement 1: SQL Injection Prevention in OpenVPN Scripts

### Acceptance Criteria

1.1. All OpenVPN environment variables (`common_name`, `trusted_ip`, `trusted_port`, `time_unix`, `ifconfig_pool_remote_ip`) MUST be validated against strict regex patterns before being used in any string that is later interpolated into SQL.

1.2. `trusted_ip` MUST match `^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$` or be replaced with "0.0.0.0".

1.3. `trusted_port` MUST match `^[0-9]{1,5}$` or be replaced with "0".

1.4. `time_unix` MUST match `^[0-9]+$` or be replaced with current epoch from `date +%s`.

1.5. The `SESSION_ID` and `UNIQUE_ID` values MUST be passed through `sql_escape()` before interpolation into the MySQL heredoc in `koris-client-connect.sh`.

1.6. The `common_name` (used as `$U`) MUST be validated with `^[A-Za-z0-9_.-]{1,64}$` at the top of both connect and disconnect scripts; the script MUST exit 1 on invalid.

1.7. In `koris-client-disconnect.sh`, `bytes_received`, `bytes_sent`, and `time_duration` MUST be validated as `^[0-9]+$` and defaulted to "0" if invalid.

1.8. The `signal` (disconnect cause) MUST be sanitized to allow only `[A-Za-z0-9 _.:-]` characters with a max length of 64 and passed through `sql_escape()`.

---

## Requirement 2: Remove Hardcoded Secrets and IPs

### Acceptance Criteria

2.1. `KORIS_RADIUS_SECRET` MUST NOT have a hardcoded default. If the environment variable is unset or empty, the script MUST exit with a fatal error message instructing the admin to set it in `/etc/panel-node/node.env`.

2.2. `KORIS_NAS_IP` MUST auto-detect from the system's primary IPv4 address (via `ip route get` or equivalent) if not explicitly set. If auto-detection fails and the variable is unset, the script MUST exit with a fatal error.

2.3. The hardcoded values `OvpnRad2026` and `91.107.168.34` MUST be completely removed from all script files.

2.4. The `node-install.sh` script MUST prompt for or generate a unique RADIUS secret during installation and write it to `/etc/panel-node/node.env`.

---

## Requirement 3: Safe Config Application with Validation

### Acceptance Criteria

3.1. The `applyOpenVPNServerConfig()` function MUST validate the `OpenVPNNetwork` CIDR is a valid RFC1918 private network before writing any config file.

3.2. The validation MUST reject any public IP range, invalid CIDR syntax, or prefix lengths outside [16, 28] for IPv4.

3.3. Before overwriting the config file, the function MUST create a backup at `{path}.bak.{unix_timestamp}`.

3.4. If validation fails, the original config file MUST remain unchanged and the function MUST return an error without restarting the service.

3.5. The `Port` field MUST be validated as an integer between 1 and 65535.

3.6. The `Protocol` field MUST be validated as either "udp" or "tcp".

3.7. DNS fields MUST be validated as valid IPv4 or IPv6 addresses.

---

## Requirement 4: WireGuard Protocol Support

### Acceptance Criteria

4.1. The system MUST support WireGuard as a fourth VPN protocol alongside OpenVPN, L2TP, and IKEv2.

4.2. The panel MUST generate Curve25519 key pairs (32-byte private key, 32-byte public key) for both server and per-client keys using `crypto/rand` and `golang.org/x/crypto/curve25519`.

4.3. A `wg_peers` table MUST store peer public keys, preshared keys, allowed IPs, status, and traffic counters per node per customer.

4.4. The `node_vpn_configs.protocol` ENUM MUST be extended to include `'wireguard'`.

4.5. The node agent MUST support task actions `wireguard.add_peer`, `wireguard.remove_peer`, and `wireguard.sync_config` to manage peers via `wg` CLI.

4.6. The panel MUST generate downloadable WireGuard client `.conf` files containing the client private key, server public key, endpoint, DNS, and allowed IPs.

4.7. The node agent MUST report WireGuard interface status (`wg show wg0`) in its periodic push alongside OpenVPN/L2TP/IKEv2 status.

4.8. The `node-install.sh` MUST install `wireguard-tools` as part of the dependency set.

---

## Requirement 5: IPv6 Dual-Stack Support

### Acceptance Criteria

5.1. The `node_vpn_configs` table MUST have a `network_ipv6` column for configuring IPv6 tunnel addresses (ULA range fc00::/7).

5.2. OpenVPN config templates MUST support `server-ipv6` directive when an IPv6 network is configured.

5.3. WireGuard peers MUST support dual-stack `allowed_ips` (e.g., `10.11.0.2/32,fd00:koris::2/128`).

5.4. The tc bandwidth limiting in `koris-client-connect.sh` MUST add IPv6 filter rules (`protocol ipv6`) alongside IPv4 rules when an IPv6 tunnel address is assigned.

5.5. IPv6 network validation MUST only allow ULA addresses (fc00::/7) with prefix lengths between /48 and /112.

5.6. DNS push options MUST support `dhcp-option DNS6` for IPv6 DNS servers when configured.

---

## Requirement 6: Config Templating System

### Acceptance Criteria

6.1. A `TemplateEngine` component MUST load Go `text/template` files for each supported protocol (OpenVPN, StrongSwan, xl2tpd, WireGuard).

6.2. Templates MUST receive a `TemplateVars` struct containing port, protocol, network, IPv6 network, DNS servers, PSK, server IP, and extra JSON settings.

6.3. The engine MUST provide a `Validate()` function that performs protocol-specific syntax checking on rendered output before applying.

6.4. The engine MUST provide a `Diff()` function that returns a textual diff between current config and proposed config for admin preview.

6.5. The `applyOpenVPNServerConfig()` function MUST be refactored to use the template engine instead of line-by-line string manipulation.

6.6. Template files MUST be stored in a configurable directory (default: `/etc/koris/templates/`).

---

## Requirement 7: Real-Time Bandwidth Display

### Acceptance Criteria

7.1. The node agent MUST read per-class tc statistics from `tc -s class show dev {tun_interface}` on each push cycle.

7.2. The node push payload MUST include a `per_user_bandwidth` array with entries containing class ID, IP, download rate (bps), and upload rate (bps).

7.3. The panel MUST store bandwidth snapshots in `user_bandwidth_snapshots` table with node_id, username, IP, rx_bps, tx_bps, and timestamp.

7.4. The existing WebSocket endpoint (`/api/realtime`) MUST broadcast per-user bandwidth updates to connected admin clients.

7.5. Rate calculations MUST clamp negative values to zero to handle tc counter wraps gracefully.

7.6. The bandwidth data MUST be aggregated per-username (not per-session) when a user has multiple active sessions.

---

## Requirement 8: Automatic Certificate Rotation

### Acceptance Criteria

8.1. The `vpn_certificates` table MUST be extended with `expires_at DATETIME` and `fingerprint VARCHAR(128)` columns.

8.2. A background worker MUST check certificate expiry every hour.

8.3. Certificates expiring within 30 days MUST generate a warning event in the `events` table.

8.4. Certificates expiring within 7 days MUST trigger automatic regeneration using `easy-rsa` or `openssl`.

8.5. Regenerated certificates MUST be distributed to affected nodes via the existing task system (`cert.distribute` action).

8.6. The node agent MUST support a `cert.distribute` task action that writes the certificate content to the appropriate file path and validates the cert chain.

---

## Requirement 9: Connection Limit Double-Enforcement

### Acceptance Criteria

9.1. The `koris-radius-auth.sh` script MUST query `radcheck` for the user's `Simultaneous-Use` value before performing RADIUS authentication.

9.2. The script MUST query `radacct` for active sessions (`acctstoptime IS NULL`) for the authenticating user.

9.3. If `active_sessions >= Simultaneous-Use`, the script MUST reject the connection with cause "connection limit reached" BEFORE sending the RADIUS request.

9.4. If the `Simultaneous-Use` attribute is not found or is 0, the check MUST be skipped (no limit enforced at script level).

9.5. If the DB query fails, the check MUST fail-open (allow the connection) and log the error, deferring enforcement to FreeRADIUS.

---

## Requirement 10: Log Rotation Configuration

### Acceptance Criteria

10.1. A logrotate config file MUST be installed at `/etc/logrotate.d/koris-openvpn` covering all `/var/log/openvpn/koris-*.log` files.

10.2. Rotation MUST be daily with 14 days retention.

10.3. Rotated logs MUST be compressed with `delaycompress` (compress previous rotation, not current).

10.4. The config MUST include `missingok` and `notifempty` directives.

10.5. A `postrotate` script MUST signal OpenVPN to reopen log files (via `systemctl reload` or `killall -USR1`).

10.6. The `node-install.sh` script MUST deploy this logrotate config during installation.

10.7. The log directory `/var/log/openvpn/` MUST be created with appropriate permissions (0750, root:root) during installation if it doesn't exist.
