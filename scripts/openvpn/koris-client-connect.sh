#!/usr/bin/env bash
set -euo pipefail
DB="${KORIS_RADIUS_DB:-radius_next}"
LOG="${KORIS_ACCT_LOG:-/var/log/openvpn/koris-acct.log}"
TC_LOG="${KORIS_TC_LOG:-/var/log/openvpn/koris-tc.log}"
U="${username:-${common_name:-}}"
IP="${ifconfig_pool_remote_ip:-}"
TRUSTED_IP="${trusted_ip:-}"
TRUSTED_PORT="${trusted_port:-}"
TUN="${dev:-tun0}"
SESSION_ID="${U}-${IP}-${trusted_ip:-local}-${trusted_port:-0}-${time_unix:-$(date +%s)}"
UNIQUE_ID="$(printf '%s' "$SESSION_ID" | sha1sum | awk '{print $1}' | cut -c1-32)"
[ -z "$U" ] && exit 0

sql_escape() { printf "%s" "$1" | sed "s/'/''/g"; }
SQL_USER="$(sql_escape "$U")"

# --- Input validation to prevent SQL injection ---
# Validate IP addresses (allow only digits and dots for IPv4)
[[ "$IP" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || IP=""
[[ "$TRUSTED_IP" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || TRUSTED_IP="0.0.0.0"
[[ "$TRUSTED_PORT" =~ ^[0-9]+$ ]] || TRUSTED_PORT="0"

normalize_rate() {
  local raw="$(printf '%s' "${1:-}" | tr -d ' ' | tr '[:upper:]' '[:lower:]')"
  [ -z "$raw" ] && return 1
  case "$raw" in
    *mbit|*kbit|*bit) printf '%s' "$raw"; return 0 ;;
    *m) printf '%sbit' "$raw"; return 0 ;;
    *k) printf '%sbit' "$raw"; return 0 ;;
    *[!0-9.]* ) return 1 ;;
    *) printf '%smbit' "$raw"; return 0 ;;
  esac
}

rate_from_policy() {
  local raw policy speed
  raw="$(mysql -N -B -u root "$DB" -e "SELECT value FROM radreply WHERE username='${SQL_USER}' AND attribute='Mikrotik-Rate-Limit' ORDER BY id DESC LIMIT 1" 2>>"$LOG" || true)"
  raw="$(printf '%s' "$raw" | head -n1 | tr -d '\r')"
  if [ -n "$raw" ]; then
    # Mikrotik-Rate-Limit commonly starts as upload/download. For symmetric plans both are equal.
    policy="${raw%% *}"
    local up="${policy%%/*}"
    local down="${policy#*/}"
    [ "$down" = "$policy" ] && down="$up"
    UP_RATE="$(normalize_rate "$up" || true)"
    DOWN_RATE="$(normalize_rate "$down" || true)"
    [ -n "${UP_RATE:-}" ] && [ -n "${DOWN_RATE:-}" ] && return 0
  fi
  speed="$(mysql -N -B -u root "$DB" -e "SELECT COALESCE(p.speed_mbps,0) FROM customers c LEFT JOIN plans p ON p.id=c.plan_id WHERE c.username='${SQL_USER}' LIMIT 1" 2>>"$LOG" || true)"
  speed="$(printf '%s' "$speed" | head -n1 | tr -d '\r')"
  if [[ "$speed" =~ ^[0-9]+([.][0-9]+)?$ ]] && awk "BEGIN{exit !($speed>0)}"; then
    UP_RATE="$(normalize_rate "$speed")"
    DOWN_RATE="$UP_RATE"
    return 0
  fi
  return 1
}

apply_tc_limit() {
  [ -n "$IP" ] || return 0
  command -v tc >/dev/null 2>&1 || { echo "$(date -Is) tc not found user=$U" >> "$TC_LOG"; return 0; }
  rate_from_policy || { echo "$(date -Is) no speed limit user=$U ip=$IP" >> "$TC_LOG"; return 0; }
  [ -n "${UP_RATE:-}" ] && [ -n "${DOWN_RATE:-}" ] || return 0
  local oct cid prio
  oct="${IP##*.}"
  [[ "$oct" =~ ^[0-9]+$ ]] || oct="$(printf '%s' "$IP" | cksum | awk '{print ($1 % 60000) + 1000}')"
  cid="1:$oct"
  prio="$oct"

  tc qdisc add dev "$TUN" root handle 1: htb default 9999 2>/dev/null || true
  tc class replace dev "$TUN" parent 1: classid 1:9999 htb rate 10000mbit ceil 10000mbit 2>/dev/null || true
  tc filter del dev "$TUN" protocol ip parent 1: prio "$prio" 2>/dev/null || true
  tc class replace dev "$TUN" parent 1: classid "$cid" htb rate "$DOWN_RATE" ceil "$DOWN_RATE" 2>/dev/null || true
  tc qdisc replace dev "$TUN" parent "$cid" sfq perturb 10 2>/dev/null || true
  tc filter add dev "$TUN" protocol ip parent 1: prio "$prio" u32 match ip dst "$IP/32" flowid "$cid" 2>/dev/null || true

  tc qdisc add dev "$TUN" handle ffff: ingress 2>/dev/null || true
  tc filter del dev "$TUN" parent ffff: protocol ip prio "$prio" 2>/dev/null || true
  tc filter add dev "$TUN" parent ffff: protocol ip prio "$prio" u32 match ip src "$IP/32" police rate "$UP_RATE" burst 200k drop flowid :1 2>/dev/null || true

  echo "$(date -Is) LIMIT user=$U ip=$IP dev=$TUN down=$DOWN_RATE up=$UP_RATE class=$cid prio=$prio" >> "$TC_LOG"
}

apply_tc_limit || true

mysql -u root "$DB" <<SQL
INSERT INTO radacct(acctsessionid,acctuniqueid,username,nasipaddress,nasporttype,acctstarttime,acctupdatetime,acctauthentic,connectinfo_start,calledstationid,callingstationid,servicetype,framedprotocol,framedipaddress)
VALUES('${SESSION_ID}','${UNIQUE_ID}','${SQL_USER}','${KORIS_NAS_IP:-91.107.168.34}','Virtual',NOW(),NOW(),'RADIUS','OpenVPN','${KORIS_NAS_IP:-91.107.168.34}','${TRUSTED_IP}:${TRUSTED_PORT}','Login-User','PPP','${IP}')
ON DUPLICATE KEY UPDATE acctupdatetime=NOW(), acctstoptime=NULL, framedipaddress=VALUES(framedipaddress), callingstationid=VALUES(callingstationid);
SQL
echo "$(date -Is) START user=$U ip=$IP session=$SESSION_ID" >> "$LOG"
exit 0
