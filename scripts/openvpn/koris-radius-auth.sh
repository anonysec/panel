#!/usr/bin/env bash
set -euo pipefail
AUTH_FILE="${1:-}"
LOG="${KORIS_AUTH_LOG:-/var/log/openvpn/koris-auth.log}"
DB="${KORIS_RADIUS_DB:-radius_next}"
RADIUS_HOST="${KORIS_RADIUS_HOST:-127.0.0.1}"
RADIUS_SECRET="${KORIS_RADIUS_SECRET:?FATAL: KORIS_RADIUS_SECRET must be set in /etc/panel-node/node.env}"
NAS_IP="${KORIS_NAS_IP:-$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="src") print $(i+1); exit}')}"
[ -z "$NAS_IP" ] && { echo "$(date -Is) FATAL: Cannot determine NAS_IP. Set KORIS_NAS_IP." >> "$LOG"; exit 1; }

reject() {
  local user="${1:-unknown}"
  local reason="${2:-reject}"
  echo "$(date -Is) REJECT ${user}: ${reason}" >> "$LOG"
  exit 1
}

sql_escape() {
  printf "%s" "$1" | sed "s/'/''/g"
}

if [ -z "$AUTH_FILE" ] || [ ! -f "$AUTH_FILE" ]; then
  reject "unknown" "missing auth file"
fi

USER_NAME="$(sed -n '1p' "$AUTH_FILE" | tr -d '\r')"
USER_PASS="$(sed -n '2p' "$AUTH_FILE" | tr -d '\r')"

[[ "$USER_NAME" =~ ^[A-Za-z0-9_.-]{1,64}$ ]] || reject "$USER_NAME" "invalid username format"
SQL_USER="$(sql_escape "$USER_NAME")"

# Panel-side policy enforcement before Radius auth:
# - disabled/deleted/expired/limited users cannot log in
# - expired latest subscription marks customer expired and rejects
# - Max-Data from radcheck is enforced against accumulated radacct usage
CUSTOMER_ROW="$(mysql -N -B -u root "$DB" -e "SELECT status,IF(deleted_at IS NULL,0,1) FROM customers WHERE username='${SQL_USER}' LIMIT 1" 2>>"$LOG" || true)"
[ -n "$CUSTOMER_ROW" ] || reject "$USER_NAME" "customer not found"
STATUS="$(printf '%s' "$CUSTOMER_ROW" | awk '{print $1}')"
DELETED="$(printf '%s' "$CUSTOMER_ROW" | awk '{print $2}')"
case "$STATUS" in
  active) ;;
  disabled|deleted|expired|limited) reject "$USER_NAME" "customer status ${STATUS}" ;;
  *) reject "$USER_NAME" "invalid customer status ${STATUS}" ;;
esac
[ "$DELETED" = "0" ] || reject "$USER_NAME" "customer deleted"

SUB_EXPIRED="$(mysql -N -B -u root "$DB" -e "SELECT CASE WHEN expires_at IS NOT NULL AND expires_at <= NOW() THEN 1 ELSE 0 END FROM subscriptions WHERE username='${SQL_USER}' ORDER BY id DESC LIMIT 1" 2>>"$LOG" || true)"
if [ "${SUB_EXPIRED:-0}" = "1" ]; then
  mysql -u root "$DB" -e "UPDATE customers SET status='expired' WHERE username='${SQL_USER}'" 2>>"$LOG" || true
  reject "$USER_NAME" "subscription expired"
fi

MAX_DATA="$(mysql -N -B -u root "$DB" -e "SELECT value FROM radcheck WHERE username='${SQL_USER}' AND attribute='Max-Data' ORDER BY id DESC LIMIT 1" 2>>"$LOG" || true)"
MAX_DATA="${MAX_DATA:-0}"
USED_BYTES="$(mysql -N -B -u root "$DB" -e "SELECT COALESCE(SUM(COALESCE(acctinputoctets,0)+COALESCE(acctoutputoctets,0)),0) FROM radacct WHERE username='${SQL_USER}'" 2>>"$LOG" || echo 0)"
USED_BYTES="${USED_BYTES:-0}"
if [[ "$MAX_DATA" =~ ^[0-9]+$ ]] && [ "$MAX_DATA" -gt 0 ] && [[ "$USED_BYTES" =~ ^[0-9]+$ ]] && [ "$USED_BYTES" -ge "$MAX_DATA" ]; then
  mysql -u root "$DB" -e "UPDATE customers SET status='limited' WHERE username='${SQL_USER}'" 2>>"$LOG" || true
  reject "$USER_NAME" "data limit reached used=${USED_BYTES} max=${MAX_DATA}"
fi

# First-connection activation: if there's a pending subscription with activate_on_connect=1,
# set first_connect_at and calculate expires_at based on plan duration_days.
PENDING_SUB="$(mysql -N -B -u root "$DB" -e "SELECT s.id, COALESCE(p.duration_days,0) FROM subscriptions s LEFT JOIN plans p ON p.id=s.plan_id WHERE s.username='${SQL_USER}' AND s.activate_on_connect=1 AND s.first_connect_at IS NULL ORDER BY s.id DESC LIMIT 1" 2>>"$LOG" || true)"
if [ -n "$PENDING_SUB" ]; then
  SUB_ID="$(printf '%s' "$PENDING_SUB" | awk '{print $1}')"
  PLAN_DAYS="$(printf '%s' "$PENDING_SUB" | awk '{print $2}')"
  if [[ "$SUB_ID" =~ ^[0-9]+$ ]] && [[ "$PLAN_DAYS" =~ ^[0-9]+$ ]]; then
    if [ "$PLAN_DAYS" -gt 0 ]; then
      mysql -u root "$DB" -e "UPDATE subscriptions SET first_connect_at=NOW(), started_at=NOW(), expires_at=DATE_ADD(NOW(), INTERVAL ${PLAN_DAYS} DAY), activate_on_connect=0 WHERE id=${SUB_ID}" 2>>"$LOG" || true
    else
      mysql -u root "$DB" -e "UPDATE subscriptions SET first_connect_at=NOW(), started_at=NOW(), activate_on_connect=0 WHERE id=${SUB_ID}" 2>>"$LOG" || true
    fi
    echo "$(date -Is) ACTIVATE user=$USER_NAME sub_id=$SUB_ID days=$PLAN_DAYS" >> "$LOG"
  fi
fi

REQ="$(mktemp)"
OUT="$(mktemp)"
cleanup(){ rm -f "$REQ" "$OUT"; }
trap cleanup EXIT
python3 - "$USER_NAME" "$USER_PASS" "$NAS_IP" > "$REQ" <<'PYREQ'
import sys
u,p,nas=sys.argv[1:4]
def esc(s): return s.replace('\\','\\\\').replace('"','\\"')
print(f'User-Name = "{esc(u)}"')
print(f'User-Password = "{esc(p)}"')
print(f'NAS-IP-Address = {nas}')
print('NAS-Port-Type = Virtual')
print('Service-Type = Login-User')
print('Framed-Protocol = PPP')
PYREQ
if radclient -r 1 -t 3 "$RADIUS_HOST" auth "$RADIUS_SECRET" < "$REQ" > "$OUT" 2>&1 && grep -q 'Access-Accept' "$OUT"; then
  echo "$(date -Is) ACCEPT ${USER_NAME} used=${USED_BYTES} max=${MAX_DATA}" >> "$LOG"
  exit 0
fi
reject "$USER_NAME" "radius reject: $(tr '\n' ' ' < "$OUT" | tail -c 500)"
