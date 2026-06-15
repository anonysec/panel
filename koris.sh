#!/usr/bin/env bash
#
# KorisPanel Management CLI
# Usage: koris [command]
#

red='\033[0;31m'; green='\033[0;32m'; yellow='\033[0;33m'; blue='\033[0;34m'; cyan='\033[0;36m'; bold='\033[1m'; dim='\033[2m'; plain='\033[0m'
info()  { echo -e "${green}[+]${plain} $*"; }
warn()  { echo -e "${yellow}[!]${plain} $*"; }
error() { echo -e "${red}[-]${plain} $*"; }

INSTALL_DIR="/opt/koris-next"
PANEL_ENV="/etc/panel/panel.env"
NODE_ENV="/etc/panel-node/node.env"

is_panel() { [[ -f /usr/local/bin/panel && -f "$PANEL_ENV" ]]; }
is_node()  { [[ -f /usr/local/bin/panel-node && -f "$NODE_ENV" ]]; }
get_version() { cat "$INSTALL_DIR/VERSION" 2>/dev/null || echo "?"; }

panel_status() { systemctl is-active panel 2>/dev/null || echo "not installed"; }
node_status()  { systemctl is-active node-agent 2>/dev/null || echo "not installed"; }

cmd_start()   { [[ $EUID -ne 0 ]] && { error "Need root"; exit 1; }; is_panel && systemctl start panel && info "Panel started"; is_node && systemctl start node-agent && info "Node started"; }
cmd_stop()    { [[ $EUID -ne 0 ]] && { error "Need root"; exit 1; }; is_panel && systemctl stop panel && info "Panel stopped"; is_node && systemctl stop node-agent && info "Node stopped"; }
cmd_restart() { [[ $EUID -ne 0 ]] && { error "Need root"; exit 1; }; is_panel && systemctl restart panel && info "Panel restarted"; is_node && systemctl restart node-agent && info "Node restarted"; }

cmd_status() {
    echo -e "${bold}${blue}KorisPanel${plain} v$(get_version)"
    echo "───────────────────────────────────"
    printf "  %-14s %s\n" "Panel:" "$(panel_status)"
    printf "  %-14s %s\n" "Node Agent:" "$(node_status)"
    if is_panel; then
        local addr=$(grep 'PANEL_ADDR' "$PANEL_ENV" 2>/dev/null | cut -d= -f2 | tr -d "'\"")
        printf "  %-14s %s\n" "Listen:" "${addr:-?}"
        curl -fsS "http://${addr}/api/health" >/dev/null 2>&1 && printf "  %-14s ${green}%s${plain}\n" "Health:" "OK" || printf "  %-14s ${red}%s${plain}\n" "Health:" "FAIL"
    fi
    echo "───────────────────────────────────"
    printf "  %-14s %s\n" "CPU:" "$(nproc) cores"
    printf "  %-14s %s\n" "RAM:" "$(free -h | awk '/^Mem:/{print $3"/"$2}')"
    printf "  %-14s %s\n" "Disk:" "$(df -h / | awk 'NR==2{print $3"/"$2" ("$5")"}')"
}

cmd_logs() {
    is_panel && { echo -e "${cyan}=== Panel ===${plain}"; journalctl -u panel -n 50 --no-pager; }
    is_node  && { echo -e "${cyan}=== Node ===${plain}"; journalctl -u node-agent -n 50 --no-pager; }
}

cmd_follow() {
    is_panel && exec journalctl -u panel -f
    is_node  && exec journalctl -u node-agent -f
}

cmd_update() {
    [[ $EUID -ne 0 ]] && { error "Need root"; exit 1; }
    [[ ! -d "$INSTALL_DIR/.git" ]] && { error "Not a git install"; exit 1; }
    cd "$INSTALL_DIR"
    local old=$(get_version)
    git fetch origin main --depth=1 >/dev/null 2>&1
    git reset --hard origin/main >/dev/null 2>&1
    local new=$(get_version)
    [[ "$old" == "$new" ]] && { info "Already up to date (v${new})."; return; }
    info "Updating v${old} -> v${new}..."
    bash "$INSTALL_DIR/deploy.sh"
    # Update self
    cp "$INSTALL_DIR/koris.sh" /usr/local/bin/koris 2>/dev/null; chmod +x /usr/local/bin/koris 2>/dev/null
    info "Done: v${new}"
}

cmd_uninstall() {
    [[ $EUID -ne 0 ]] && { error "Need root"; exit 1; }
    echo -e "${red}This will remove KorisPanel completely.${plain}"
    read -rp "Type 'yes' to confirm: " c; [[ "$c" != "yes" ]] && exit 0
    systemctl stop panel node-agent 2>/dev/null; systemctl disable panel node-agent 2>/dev/null
    rm -f /etc/systemd/system/panel.service /etc/systemd/system/node-agent.service
    systemctl daemon-reload
    rm -f /usr/local/bin/panel /usr/local/bin/panel-node /usr/local/bin/koris
    rm -rf /etc/panel /etc/panel-node "$INSTALL_DIR"
    rm -f /etc/nginx/sites-enabled/koris-panel.conf /etc/nginx/sites-available/koris-panel.conf
    systemctl reload nginx 2>/dev/null || true
    info "Uninstalled. Database not removed (manual cleanup needed)."
}

cmd_config() {
    is_panel && { echo -e "${cyan}Panel Config:${plain}"; grep -v 'SECRET\|PASSWORD\|TOKEN' "$PANEL_ENV" 2>/dev/null | sed 's/^/  /'; echo "  (secrets hidden)"; }
    is_node  && { echo -e "${cyan}Node Config:${plain}"; grep -v 'TOKEN' "$NODE_ENV" 2>/dev/null | sed 's/^/  /'; echo "  (token hidden)"; }
}

show_menu() {
    clear
    echo -e "${bold}${blue}KorisPanel${plain} v$(get_version)    Panel: $(panel_status)  Node: $(node_status)"
    echo ""
    echo -e "  ${green}1.${plain} Start       ${green}5.${plain} Logs          ${green}9.${plain}  Enable autostart"
    echo -e "  ${green}2.${plain} Stop        ${green}6.${plain} Live logs     ${green}10.${plain} Disable autostart"
    echo -e "  ${green}3.${plain} Restart     ${green}7.${plain} Update        ${green}11.${plain} Uninstall"
    echo -e "  ${green}4.${plain} Status      ${green}8.${plain} Config        ${green}0.${plain}  Exit"
    echo ""
    read -rp "$(echo -e "${cyan}Choose: ${plain}")" ch
    case "$ch" in
        1) cmd_start;; 2) cmd_stop;; 3) cmd_restart;; 4) cmd_status;;
        5) cmd_logs;; 6) cmd_follow;; 7) cmd_update;; 8) cmd_config;;
        9) systemctl enable panel node-agent 2>/dev/null; info "Enabled.";;
        10) systemctl disable panel node-agent 2>/dev/null; info "Disabled.";;
        11) cmd_uninstall;; 0) exit 0;; *) warn "Invalid.";;
    esac
    echo ""; read -rp "Enter to continue..." _; show_menu
}

case "${1:-}" in
    start)     cmd_start;; stop) cmd_stop;; restart) cmd_restart;;
    status)    cmd_status;; logs) cmd_logs;; follow|logs-live) cmd_follow;;
    update)    cmd_update;; config) cmd_config;; uninstall) cmd_uninstall;;
    enable)    systemctl enable panel node-agent 2>/dev/null; info "Enabled.";;
    disable)   systemctl disable panel node-agent 2>/dev/null; info "Disabled.";;
    node-status)  echo "Node Agent: $(node_status)";;
    node-restart) systemctl restart node-agent 2>/dev/null; info "Node restarted.";;
    node-logs)    journalctl -u node-agent -n 50 --no-pager;;
    help|-h|--help) echo "Usage: koris [start|stop|restart|status|logs|follow|update|config|uninstall|enable|disable|node-status|node-restart|node-logs]"; echo "Run without args for interactive menu.";;
    "") show_menu;;
    *) error "Unknown: $1. Run 'koris help'."; exit 1;;
esac
