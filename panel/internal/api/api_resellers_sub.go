package api

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"math"
	"net/http"
	"strings"
)

func (s *Server) subscriptionLink(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		token = strings.TrimPrefix(r.URL.Path, "/portal/sub/")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		http.Error(w, "Unauthorized: missing token", http.StatusUnauthorized)
		return
	}

	var username string
	var status string
	err := s.DB.QueryRow(`SELECT username, status FROM customers WHERE sub_token=$1 LIMIT 1`, token).Scan(&username, &status)
	if err == sql.ErrNoRows {
		http.Error(w, "Subscription not found", http.StatusNotFound)
		return
	}

	ua := strings.ToLower(r.Header.Get("User-Agent"))

	if strings.Contains(ua, "clash") {
		host, port, _, _ := s.openVPNEndpoint(r)

		yaml := fmt.Sprintf(`port: 7890
socks-port: 7891
allow-lan: true
mode: Rule
log-level: info
proxies:
  - name: "Koris-OpenVPN-%s"
    type: socks5
    server: "%s"
    port: %d
    # Subscription URL for direct profile: http://%s/api/portal/profiles/openvpn.ovpn$1token=%s
  - name: "Koris-L2TP-%s"
    type: socks5
    server: "%s"
    port: 1701
    # L2TP/IPSec connection available. Configure in device settings.
proxy-groups:
  - name: PROXY
    type: select
    proxies:
      - "Koris-OpenVPN-%s"
      - "Koris-L2TP-%s"
rules:
  - DOMAIN-SUFFIX,ir,DIRECT
  - DOMAIN-SUFFIX,telewebion.com,DIRECT
  - DOMAIN-SUFFIX,snapp.ir,DIRECT
  - DOMAIN-KEYWORD,adservice,REJECT
  - DOMAIN-KEYWORD,analytics,REJECT
  - MATCH,PROXY
`, username, host, port, r.Host, token, username, host, username, username)

		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(yaml))
		return
	}

	isClientApp := strings.Contains(ua, "shadowrocket") || strings.Contains(ua, "sing-box") || strings.Contains(ua, "v2ray") || strings.Contains(ua, "trojan")

	if isClientApp {
		host, port, proto, _ := s.openVPNEndpoint(r)
		var psk string
		_ = s.DB.QueryRow(`SELECT COALESCE(ipsec_psk,'') FROM vpn_core_settings WHERE id=1`).Scan(&psk)

		var builder strings.Builder
		builder.WriteString("# Koris Unified Subscription\n")
		builder.WriteString(fmt.Sprintf("# User: %s (Status: %s)\n\n", username, status))

		ovpnURL := fmt.Sprintf("http://%s/api/portal/profiles/openvpn.ovpn?token=%s", r.Host, token)
		builder.WriteString(fmt.Sprintf("REMARKS=OpenVPN Node, URL=%s, PORT=%d, PROTOCOL=%s\n", ovpnURL, port, proto))
		builder.WriteString(fmt.Sprintf("REMARKS=L2TP Node, HOST=%s, PSK=%s, USERNAME=%s\n", host, psk, username))
		builder.WriteString(fmt.Sprintf("REMARKS=IKEv2 Node, HOST=%s, USERNAME=%s\n", host, username))

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Subscription-Userinfo", fmt.Sprintf("upload=0; download=0; total=100000000000; expire=0"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(base64.StdEncoding.EncodeToString([]byte(builder.String()))))
		return
	}

	var maxData int64
	_ = s.DB.QueryRow(`SELECT COALESCE(value,0) FROM radcheck WHERE username=$1 AND attribute='Max-Data'`, username).Scan(&maxData)

	var used int64
	_ = s.DB.QueryRow(`SELECT COALESCE(SUM(acctinputoctets+acctoutputoctets),0) FROM radacct WHERE username=$1`, username).Scan(&used)

	var online int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM radacct WHERE username=$1 AND acctstoptime IS NULL`, username).Scan(&online)

	isOnline := online > 0
	pct := 0.0
	if maxData > 0 {
		pct = math.Min(100.0, float64(used)/float64(maxData)*100.0)
	}

	lang := strings.ToLower(r.URL.Query().Get("lang"))
	if lang == "" {
		lang = "en"
	}

	translations := map[string]map[string]string{
		"en": {
			"title":       "Unified Secure Access Portal",
			"status":      "Status",
			"usage":       "Usage Summary",
			"download":    "Download OpenVPN Profile",
			"server":      "Server",
			"username":    "Username",
			"l2tp_psk":    "L2TP PSK",
			"unlimited":   "Unlimited",
			"online":      "Online",
			"offline":     "Offline",
			"langs":       `LANGS: <a href="?token=%s&lang=en">EN</a> · <a href="?token=%s&lang=fa">FA</a> · <a href="?token=%s&lang=ru">RU</a> · <a href="?token=%s&lang=zh">ZH</a>`,
			"guide_title": "Manual Setup Connection Guides",
			"guide_desc":  "For Windows, iOS & macOS native setups: Add an L2TP/IPSec VPN connection. Use the Server Address above, select Username/Password authentication, and enter the pre-shared secret (PSK) provided by your administrator.",
		},
		"fa": {
			"title":       "پورتال دسترسی امن یکپارچه",
			"status":      "وضعیت",
			"usage":       "خلاصه مصرف",
			"download":    "دانلود فایل تنظیمات OpenVPN",
			"server":      "سرور",
			"username":    "نام کاربری",
			"l2tp_psk":    "کلید L2TP PSK",
			"unlimited":   "نامحدود",
			"online":      "متصل",
			"offline":     "قطع",
			"langs":       `زبان‌ها: <a href="?token=%s&lang=en">EN</a> · <a href="?token=%s&lang=fa">FA</a> · <a href="?token=%s&lang=ru">RU</a> · <a href="?token=%s&lang=zh">ZH</a>`,
			"guide_title": "راهنمای اتصال دستی",
			"guide_desc":  "برای تنظیمات بومی ویندوز، iOS و macOS: یک اتصال VPN از نوع L2TP/IPSec اضافه کنید. از آدرس سرور بالا استفاده کنید، نوع تایید هویت را روی Username/Password بگذارید، و کلید مشترک (PSK) را وارد کنید.",
		},
		"ru": {
			"title":       "Единый портал безопасного доступа",
			"status":      "Статус",
			"usage":       "Сводка использования",
			"download":    "Скачать профиль OpenVPN",
			"server":      "Сервер",
			"username":    "Имя пользователя",
			"l2tp_psk":    "L2TP PSK",
			"unlimited":   "Безлимитный",
			"online":      "Онлайн",
			"offline":     "Оффлайн",
			"langs":       `Языки: <a href="?token=%s&lang=en">EN</a> · <a href="?token=%s&lang=fa">FA</a> · <a href="?token=%s&lang=ru">RU</a> · <a href="?token=%s&lang=zh">ZH</a>`,
			"guide_title": "Инструкции по ручной настройке",
			"guide_desc":  "Для стандартных подключений Windows, iOS и macOS: Добавьте VPN-подключение L2TP/IPSec. Используйте адрес сервера выше, выберите аутентификацию по имени пользователя/паролю и введите общий ключ (PSK).",
		},
		"zh": {
			"title":       "统一安全访问门户",
			"status":      "状态",
			"usage":       "用量摘要",
			"download":    "下载 OpenVPN 配置文件",
			"server":      "服务器",
			"username":    "用户名",
			"l2tp_psk":    "L2TP 预共享密钥",
			"unlimited":   "无限制",
			"online":      "在线",
			"offline":     "离线",
			"langs":       `语言: <a href="?token=%s&lang=en">EN</a> · <a href="?token=%s&lang=fa">FA</a> · <a href="?token=%s&lang=ru">RU</a> · <a href="?token=%s&lang=zh">ZH</a>`,
			"guide_title": "手动连接配置指南",
			"guide_desc":  "对于 Windows、iOS 和 macOS 原生设置：添加 L2TP/IPSec VPN 连接。使用上方的服务器地址，选择用户名/密码身份验证，然后输入预共享密钥 (PSK)。",
		},
	}

	t := translations[lang]
	if t == nil {
		t = translations["en"]
	}

	dir := "ltr"
	if lang == "fa" {
		dir = "rtl"
	}

	usedGB := float64(used) / (1024 * 1024 * 1024)
	totalGBStr := t["unlimited"]
	if maxData > 0 {
		totalGBStr = fmt.Sprintf("%.2f GB", float64(maxData)/(1024*1024*1024))
	}

	langsBar := fmt.Sprintf(t["langs"], token, token, token, token)

	html := fmt.Sprintf(`<!doctype html>
<html lang="%s" dir="%s">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<meta name="referrer" content="no-referrer">
	<title>%s</title>
	<style>
		:root {
			--bg: #030712;
			--panel: rgba(17, 24, 39, 0.7);
			--line: rgba(75, 85, 99, 0.25);
			--cyan: #22d3ee;
			--blue: #3b82f6;
			--green: #10b981;
			--red: #ef4444;
			--text: #f3f4f6;
		}
		* { box-sizing: border-box; }
		body {
			background: radial-gradient(1000px 600px at 70%% -10%%, rgba(59, 130, 246, 0.2), transparent 60%%), linear-gradient(135deg, #030712 0%%, #0f172a 100%%);
			color: var(--text);
			font-family: 'Inter', system-ui, -apple-system, sans-serif;
			margin: 0;
			min-height: 100vh;
			display: grid;
			place-items: center;
			padding: 24px;
		}
		.card {
			background: var(--panel);
			border: 1px solid var(--line);
			border-radius: 24px;
			width: 100%%;
			max-width: 580px;
			padding: 28px;
			box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5);
			backdrop-filter: blur(16px);
		}
		.brand {
			display: flex;
			align-items: center;
			gap: 12px;
			margin-bottom: 24px;
		}
		.logo {
			background: linear-gradient(135deg, var(--blue), var(--cyan));
			border-radius: 12px;
			width: 38px;
			height: 38px;
			display: grid;
			place-items: center;
			font-weight: 900;
			color: #fff;
		}
		h2 { margin: 0; font-size: 22px; font-weight: 800; letter-spacing: -0.03em; }
		.status-row {
			display: flex;
			justify-content: space-between;
			align-items: center;
			margin-bottom: 18px;
		}
		.pill {
			border-radius: 99px;
			padding: 6px 12px;
			font-size: 11px;
			font-weight: 900;
			text-transform: uppercase;
		}
		.pill.online { background: rgba(16, 185, 129, 0.15); color: var(--green); }
		.pill.offline { background: rgba(75, 85, 99, 0.15); color: #9ca3af; }
		.bar-wrap { margin-bottom: 24px; }
		.bar {
			background: rgba(0, 0, 0, 0.4);
			border-radius: 99px;
			height: 8px;
			overflow: hidden;
			margin-top: 6px;
		}
		.bar i { display: block; height: 100%%; background: linear-gradient(90deg, var(--blue), var(--cyan)); width: %.1f%%; }
		.row {
			display: flex;
			justify-content: space-between;
			align-items: center;
			border-bottom: 1px solid var(--line);
			padding: 12px 0;
		}
		.row b { color: #9ca3af; font-size: 13px; }
		.row span { font-size: 14px; font-weight: 700; word-break: break-all; text-align: right; }
		.btn {
			background: linear-gradient(135deg, var(--blue), #1d4ed8);
			border: 0;
			border-radius: 12px;
			color: #fff;
			display: block;
			width: 100%%;
			padding: 14px;
			font-weight: 900;
			text-align: center;
			text-decoration: none;
			margin-top: 18px;
			cursor: pointer;
		}
		.langs-bar {
			text-align: center;
			margin-bottom: 15px;
			font-size: 12px;
			color: var(--cyan);
		}
		.langs-bar a {
			color: #9ca3af;
			text-decoration: none;
			margin: 0 4px;
			font-weight: bold;
		}
		.langs-bar a:hover {
			color: var(--cyan);
		}
	</style>
</head>
<body>
	<div class="card">
		<div class="langs-bar">
			%s
		</div>
		<div class="brand">
			<div class="logo">K</div>
			<div>
				<h2>%s</h2>
			</div>
		</div>
		<div class="status-row">
			<div>
				<b>%s:</b> <span>%s</span>
			</div>
			<span class="pill %s">%s</span>
		</div>
		<div class="bar-wrap">
			<div class="status-row" style="margin-bottom: 6px;">
				<b>%s</b>
				<span>%.2f GB / %s</span>
			</div>
			<div class="bar"><i></i></div>
		</div>
		<div class="row">
			<b>%s</b>
			<span>%s</span>
		</div>
		<div class="row">
			<b>%s</b>
			<span>Default</span>
		</div>
		<p><a class="btn" href="/api/portal/profiles/openvpn.ovpn?token=%s">%s</a></p>
		<div class="row" style="margin-top: 18px; border-top: 1px solid var(--line); padding-top: 15px;">
			<b style="color: var(--cyan); font-size: 14px; font-weight: 800;">%s</b>
		</div>
		<p class="mu" style="font-size: 13px; line-height: 1.5; margin: 5px 0 0 0; color: var(--text); opacity: 0.85;">%s</p>
	</div>
</body>
</html>`, lang, dir, t["title"], pct, langsBar, t["title"], t["username"], username, map[bool]string{true: "online", false: "offline"}[isOnline], map[bool]string{true: t["online"], false: t["offline"]}[isOnline], t["usage"], usedGB, totalGBStr, t["status"], status, t["l2tp_psk"], token, t["download"], t["guide_title"], t["guide_desc"])

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
}
