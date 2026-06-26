package api

import (
	"net/http"
	"os/exec"
	"strings"
)

func (s *Server) diagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	isActive := func(service string) bool {
		cmd := exec.Command("systemctl", "is-active", service)
		out, err := cmd.Output()
		if err != nil {
			return false
		}
		return strings.TrimSpace(string(out)) == "active"
	}

	runCmd := func(name string, args ...string) string {
		cmd := exec.Command(name, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}

	var checks []map[string]any

	checks = append(checks, map[string]any{
		"name":   "Nginx service",
		"ok":     isActive("nginx"),
		"detail": "systemctl is-active nginx",
	})
	checks = append(checks, map[string]any{
		"name":   "MariaDB service",
		"ok":     isActive("mariadb"),
		"detail": "systemctl is-active mariadb",
	})
	checks = append(checks, map[string]any{
		"name":   "Auth service",
		"ok":     isActive("freeradius"),
		"detail": "systemctl is-active freeradius",
	})
	checks = append(checks, map[string]any{
		"name":   "Panel service",
		"ok":     isActive("panel"),
		"detail": "systemctl is-active koris",
	})
	checks = append(checks, map[string]any{
		"name":   "OpenVPN service",
		"ok":     isActive("openvpn-server@server") || isActive("openvpn"),
		"detail": "systemctl is-active openvpn-server@server",
	})
	checks = append(checks, map[string]any{
		"name":   "Node agent",
		"ok":     isActive("knode"),
		"detail": "systemctl is-active knode",
	})
	checks = append(checks, map[string]any{
		"name":   "L2TP service",
		"ok":     isActive("xl2tpd"),
		"detail": "systemctl is-active xl2tpd",
	})
	checks = append(checks, map[string]any{
		"name":   "IKEv2 service",
		"ok":     isActive("strongswan") || isActive("strongswan-starter") || isActive("swanctl"),
		"detail": "strongswan service check",
	})

	disk := runCmd("sh", "-c", "df -h / | tail -1 | awk '{print $3 \" / \" $2 \" (\" $5 \")\"}'")
	if disk == "" {
		disk = "N/A"
	}

	mem := runCmd("sh", "-c", "free -h | awk '/Mem:/ {print $3 \" / \" $2}'")
	if mem == "" {
		mem = "N/A"
	}

	ports := runCmd("sh", "-c", "ss -ltnp | grep -E ':(80|443|8088|1194|1812|1813)'")

	writeJSON(w, map[string]any{
		"ok":     true,
		"disk":   disk,
		"mem":    mem,
		"checks": checks,
		"ports":  ports,
	})
}
