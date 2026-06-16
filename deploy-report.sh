#!/bin/bash
# deploy-report.sh — Posts deploy diagnostics to a GitHub Issue for remote debugging.

# Source environment from common locations where the token might be stored
[ -f /opt/koris-next/panel.env ] && source /opt/koris-next/panel.env
[ -f /etc/panel-env ] && source /etc/panel-env
[ -f /root/.panel-token ] && source /root/.panel-token
[ -f ~/.bashrc ] && source ~/.bashrc 2>/dev/null

# Requires: GITHUB_TOKEN env var with repo scope
# Optional: GITHUB_REPO (defaults to anonysec/panel)

GITHUB_REPO="${GITHUB_REPO:-anonysec/panel}"
GITHUB_TOKEN="${GITHUB_TOKEN:-}"

if [ -z "$GITHUB_TOKEN" ]; then
    echo "[deploy-report] GITHUB_TOKEN not set, skipping log upload"
    exit 0
fi

# Collect diagnostics
PANEL_LOGS=$(journalctl -u panel -n 30 --no-pager -o short-iso 2>&1 || echo "not available")
NGINX_LOGS=$(journalctl -u nginx -n 15 --no-pager -o short-iso 2>&1 || echo "not available")
OPENVPN_LOGS=$(journalctl -u openvpn@server -n 15 --no-pager -o short-iso 2>&1 || echo "not available")
MYSQL_LOGS=$(journalctl -u mariadb -n 10 --no-pager -o short-iso 2>&1 || echo "not available")
NODE_AGENT_LOGS=$(journalctl -u node-agent -n 15 --no-pager -o short-iso 2>&1 || echo "not available")
SERVICE_STATUS=$(systemctl is-active panel 2>/dev/null || echo "unknown")
HEALTH_CHECK=$(curl -s --max-time 5 http://127.0.0.1:${PANEL_PORT:-8088}/api/health 2>/dev/null || echo "health check failed")
PANEL_VERSION=$(cat /opt/koris-next/VERSION 2>/dev/null || echo "unknown")
HOSTNAME=$(hostname 2>/dev/null || echo "unknown")
DATE=$(date -u '+%Y-%m-%d %H:%M:%S UTC')

# Build issue title
TITLE="[Deploy] ${DATE} - ${SERVICE_STATUS}"

# Build issue body - use heredoc and jq if available, otherwise use simple sed escaping
BODY="## Deploy Report — ${DATE}

**Host:** \`${HOSTNAME}\`
**Service Status:** \`${SERVICE_STATUS}\`
**Version:** \`${PANEL_VERSION}\`

### Health Check
\`\`\`
${HEALTH_CHECK}
\`\`\`

### Panel Logs (last 30)
\`\`\`
${PANEL_LOGS}
\`\`\`

### Nginx Logs (last 15)
\`\`\`
${NGINX_LOGS}
\`\`\`

### OpenVPN Logs (last 15)
\`\`\`
${OPENVPN_LOGS}
\`\`\`

### MariaDB Logs (last 10)
\`\`\`
${MYSQL_LOGS}
\`\`\`

### Node Agent Logs (last 15)
\`\`\`
${NODE_AGENT_LOGS}
\`\`\`"

# Try jq first for proper JSON escaping, fallback to simple approach
if command -v jq &>/dev/null; then
    JSON_PAYLOAD=$(jq -n --arg title "$TITLE" --arg body "$BODY" '{title: $title, body: $body}')
else
    # Simple escape: replace backslashes, quotes, newlines
    ESCAPED_BODY=$(echo "$BODY" | sed 's/\\/\\\\/g; s/"/\\"/g' | awk '{printf "%s\\n", $0}')
    ESCAPED_TITLE=$(echo "$TITLE" | sed 's/"/\\"/g')
    JSON_PAYLOAD="{\"title\":\"${ESCAPED_TITLE}\",\"body\":\"${ESCAPED_BODY}\"}"
fi

# Create GitHub issue (no label required)
RESPONSE=$(curl -s -w "\n%{http_code}" -X POST \
    -H "Authorization: token $GITHUB_TOKEN" \
    -H "Accept: application/vnd.github.v3+json" \
    -H "Content-Type: application/json" \
    "https://api.github.com/repos/$GITHUB_REPO/issues" \
    -d "$JSON_PAYLOAD" 2>&1)

HTTP_CODE=$(echo "$RESPONSE" | tail -1)
RESPONSE_BODY=$(echo "$RESPONSE" | sed '$d')

if [ "$HTTP_CODE" = "201" ]; then
    ISSUE_URL=$(echo "$RESPONSE_BODY" | grep -o '"html_url":"[^"]*"' | head -1 | cut -d'"' -f4)
    echo "[deploy-report] ✓ Diagnostics posted: $ISSUE_URL"
else
    echo "[deploy-report] ✗ Failed to post (HTTP $HTTP_CODE)"
    echo "[deploy-report] Response: $RESPONSE_BODY" | head -5
fi
