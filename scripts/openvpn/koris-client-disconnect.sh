#!/usr/bin/env bash
set -euo pipefail
DB="${KORIS_RADIUS_DB:-radius_next}"
LOG="${KORIS_ACCT_LOG:-/var/log/openvpn/koris-acct.log}"
TC_LOG="${KORIS_TC_LOG:-/var/log/openvpn/koris-tc.log}"
U="${username:-${common_name:-}}"

# Validate username
[[ "$U" =~ ^[A-Za-z0-9_.-]{1,64}$ ]] || { echo "$(date -Is) REJECT invalid username: $U" >> "$LOG"; exit 1; }

IP="${ifconfig_pool_remote_ip:-}"
TUN="${dev:-tun0}"
IN="${bytes_received:-0}"
OUT="${bytes_sent:-0}"
DUR="${time_duration:-0}"
CAUSE="${signal:-User-Request}"
[ -z "$U" ] && exit 0

# --- Input validation to prevent SQL injection ---
# Validate numeric fields: must be non-negative integers
[[ "$IN" =~ ^[0-9]+$ ]] || IN="0"
[[ "$OUT" =~ ^[0-9]+$ ]] || OUT="0"
[[ "$DUR" =~ ^[0-9]+$ ]] || DUR="0"

# Sanitize CAUSE: allow only safe characters (alphanum, dash, underscore, space, dot)
CAUSE="$(printf '%s' "$CAUSE" | tr -cd 'A-Za-z0-9 _.:-' | head -c 64)"
[ -z "$CAUSE" ] && CAUSE="User-Request"

sql_escape() { printf "%s" "$1" | sed "s/'/''/g"; }
SQL_USER="$(sql_escape "$U")"
SQL_CAUSE="$(sql_escape "$CAUSE")"

cleanup_tc_limit() {
  [ -n "$IP" ] || return 0
  command -v tc >/dev/null 2>&1 || return 0
  local oct cid prio
  oct="${IP##*.}"
  [[ "$oct" =~ ^[0-9]+$ ]] || oct="$(printf '%s' "$IP" | cksum | awk '{print ($1 % 60000) + 1000}')"
  cid="1:$oct"
  prio="$oct"
  tc filter del dev "$TUN" protocol ip parent 1: prio "$prio" 2>/dev/null || true
  tc qdisc del dev "$TUN" parent "$cid" 2>/dev/null || true
  tc class del dev "$TUN" classid "$cid" 2>/dev/null || true
  tc filter del dev "$TUN" parent ffff: protocol ip prio "$prio" 2>/dev/null || true
  echo "$(date -Is) UNLIMIT user=$U ip=$IP dev=$TUN class=$cid prio=$prio" >> "$TC_LOG"
}

cleanup_tc_limit || true

mysql -u root "$DB" <<SQL
UPDATE radacct
SET acctstoptime=NOW(), acctupdatetime=NOW(), acctsessiontime=${DUR}, acctinputoctets=${IN}, acctoutputoctets=${OUT}, acctterminatecause='${SQL_CAUSE}', connectinfo_stop='OpenVPN disconnect'
WHERE username='${SQL_USER}' AND acctstoptime IS NULL
ORDER BY radacctid DESC LIMIT 1;
SQL
echo "$(date -Is) STOP user=$U ip=$IP in=$IN out=$OUT duration=$DUR cause=$CAUSE" >> "$LOG"
exit 0
