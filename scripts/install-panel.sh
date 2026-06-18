#!/usr/bin/env bash
set -euo pipefail
[ "$(id -u)" = 0 ] || { echo "run as root"; exit 1; }
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PANEL_ADDR="${PANEL_ADDR:-127.0.0.1:8080}"
DB_NAME="${DB_NAME:-radius}"
DB_USER="${DB_USER:-radius}"
DB_PASS="${DB_PASS:-RadiusDb2026}"
SETUP_KEY="${SETUP_KEY:-$(openssl rand -hex 16 2>/dev/null || head -c 16 /dev/urandom | od -An -tx1 | tr -d ' \n')}"
SESSION_SECRET="${SESSION_SECRET:-$(openssl rand -hex 32 2>/dev/null || head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')}"
DOMAIN="${DOMAIN:-_}"

echo "[info] Installing Koris Next panel..."
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq >/dev/null
apt-get install -y -qq mariadb-server freeradius freeradius-mysql freeradius-utils nginx golang-go curl openssl ca-certificates >/dev/null

echo "[info] Preparing database..."
systemctl enable --now mariadb >/dev/null 2>&1
mysql -u root <<SQL
CREATE DATABASE IF NOT EXISTS ${DB_NAME} CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS '${DB_USER}'@'localhost' IDENTIFIED BY '${DB_PASS}';
ALTER USER '${DB_USER}'@'localhost' IDENTIFIED BY '${DB_PASS}';
GRANT ALL PRIVILEGES ON ${DB_NAME}.* TO '${DB_USER}'@'localhost';
FLUSH PRIVILEGES;
SQL
SCHEMA="/etc/freeradius/3.0/mods-config/sql/main/mysql/schema.sql"
if ! mysql -u root "$DB_NAME" -N -B -e "SHOW TABLES LIKE 'radcheck';" | grep -q '^radcheck$'; then
  mysql -u root "$DB_NAME" < "$SCHEMA"
fi

echo "[info] Configuring FreeRADIUS SQL for ${DB_NAME}..."
SQL_MOD="/etc/freeradius/3.0/mods-available/sql"
if [ -f "$SQL_MOD" ]; then
  cp -a "$SQL_MOD" "${SQL_MOD}.bak.$(date +%s)"
  sed -i \
    -e 's/^\s*dialect = .*/\tdialect = "mysql"/' \
    -e 's/^\s*driver = .*/\tdriver = "rlm_sql_${dialect}"/' \
    -e "s/^\s*server = .*/\tserver = \"localhost\"/" \
    -e "s/^\s*port = .*/\tport = 3306/" \
    -e "s/^\s*login = .*/\tlogin = \"${DB_USER}\"/" \
    -e "s/^\s*password = .*/\tpassword = \"${DB_PASS}\"/" \
    -e "s/^\s*radius_db = .*/\tradius_db = \"${DB_NAME}\"/g" \
    "$SQL_MOD"
  ln -sf ../mods-available/sql /etc/freeradius/3.0/mods-enabled/sql
  rm -f /etc/freeradius/3.0/mods-enabled/sql.bak.*
  systemctl restart freeradius >/dev/null 2>&1 || true
fi

build_frontend_if_needed() {
  local app_dir="$1"
  if [ -f "$app_dir/www/index.html" ]; then
    echo "[info] Found prebuilt frontend: $app_dir/www"
    return 0
  fi
  if command -v npm >/dev/null 2>&1; then
    echo "[info] Building frontend in $app_dir"
    (cd "$app_dir" && npm install --no-audit --no-fund && npm run build)
  else
    echo "[warn] No prebuilt www/ and npm is not installed for $app_dir; UI route will show a build warning."
  fi
}

copy_dir() {
  local src="$1"
  local dst="$2"
  mkdir -p "$(dirname "$dst")"
  rm -rf "$dst"
  if [ -d "$src" ]; then
    cp -a "$src" "$dst"
  else
    mkdir -p "$dst"
  fi
}

VERSION="${PANEL_VERSION:-$(cat "$ROOT/VERSION" 2>/dev/null || echo next-dev)}"

echo "[info] Preparing Vue frontends..."
build_frontend_if_needed "$ROOT/panel/web/admin"
build_frontend_if_needed "$ROOT/panel/web/portal"

echo "[info] Building panel binary..."
cd "$ROOT"
go mod tidy
go build -o /usr/local/bin/panel ./panel/cmd/panel
chmod +x /usr/local/bin/panel

mkdir -p /opt/KorisPanel/panel/web
copy_dir "$ROOT/panel/migrations" /opt/KorisPanel/panel/migrations
copy_dir "$ROOT/panel/web/admin/www" /opt/KorisPanel/panel/web/admin/www
copy_dir "$ROOT/panel/web/portal/www" /opt/KorisPanel/panel/web/portal/www

mkdir -p /etc/panel
cat > /etc/panel/panel.env <<ENV
PANEL_ADDR='${PANEL_ADDR}'
PANEL_DB_DSN='${DB_USER}:${DB_PASS}@tcp(127.0.0.1:3306)/${DB_NAME}?parseTime=true&multiStatements=true&charset=utf8mb4,utf8'
PANEL_MIGRATIONS='/opt/KorisPanel/panel/migrations'
PANEL_SETUP_KEY='${SETUP_KEY}'
PANEL_SESSION_SECRET='${SESSION_SECRET}'
PANEL_PUBLIC_BASE='/dashboard'
PANEL_ADMIN_WEB_DIR='/opt/KorisPanel/panel/web/admin/www'
PANEL_PORTAL_WEB_DIR='/opt/KorisPanel/panel/web/portal/www'
PANEL_VERSION='${VERSION}'
ENV
chmod 600 /etc/panel/panel.env

cp "$ROOT/panel/systemd/panel.service" /etc/systemd/system/panel.service
systemctl daemon-reload
systemctl enable --now panel.service
systemctl restart panel.service
sleep 2
curl -fsS "http://${PANEL_ADDR}/api/health" >/dev/null || { journalctl -u panel -n 100 --no-pager; exit 1; }

echo "[info] Configuring nginx..."
cat > /etc/nginx/sites-available/panel-next.conf <<NGINX
server {
    listen 80 default_server;
    server_name ${DOMAIN};
    client_max_body_size 20m;

    location = / { return 404; }

    location = /dashboard { return 302 /dashboard/; }
    location /dashboard/ {
        proxy_pass http://${PANEL_ADDR};
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }

    location /api/ {
        proxy_pass http://${PANEL_ADDR};
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection \"upgrade\";
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }

    location = /portal { return 302 /portal/; }
    location /portal/ {
        proxy_pass http://${PANEL_ADDR};
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
}
NGINX
rm -f /etc/nginx/sites-enabled/default /etc/nginx/sites-enabled/koris.conf
ln -sf /etc/nginx/sites-available/panel-next.conf /etc/nginx/sites-enabled/panel-next.conf
nginx -t
systemctl enable --now nginx >/dev/null 2>&1
systemctl reload nginx

echo ""
echo "========== Koris Next Panel =========="
echo "Dashboard : http://$(curl -fsS4 --max-time 3 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')/dashboard/"
echo "Portal    : http://$(curl -fsS4 --max-time 3 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')/portal/"
echo "Setup key : ${SETUP_KEY}"
echo "Health    : http://${PANEL_ADDR}/api/health"
echo "======================================"
