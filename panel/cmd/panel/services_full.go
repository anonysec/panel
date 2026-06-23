//go:build !lite

package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"KorisPanel/panel/internal/antidpi"
	"KorisPanel/panel/internal/api"
	"KorisPanel/panel/internal/billing"
	"KorisPanel/panel/internal/bot"
	"KorisPanel/panel/internal/payment"
	"KorisPanel/panel/internal/support"
	"KorisPanel/panel/internal/teleproxy"
)

// initExcludedServices initializes premium services for the full build.
func initExcludedServices(srv *api.Server, database *sql.DB) {
	srv.Billing = billing.New(database)
	srv.Support = support.New(database)
	srv.TeleProxy = teleproxy.New(database)
	srv.AntiDPI = antidpi.New(database)
	srv.PaymentRegistry = payment.NewRegistry()
	loadActiveGateways(database, srv)
}

// startBot launches the Telegram bot (full build only).
// It reads config from the database (with env var overrides), creates the bot
// instance, starts it, and registers the hot-reload restart endpoint on mux.
func startBot(database *sql.DB, srv *api.Server, mux *http.ServeMux) {
	// Load bot config from DB first, env vars override
	dbToken, dbChatIDs := loadBotConfigFromDB(database)
	botToken := os.Getenv("PANEL_TG_BOT_TOKEN")
	if botToken == "" {
		botToken = dbToken
	}
	botEnabled := strings.ToLower(os.Getenv("PANEL_TG_ENABLED")) == "true"
	if !botEnabled && botToken != "" {
		// If token exists (from DB or env) but PANEL_TG_ENABLED is not explicitly set,
		// auto-enable if token is present
		if os.Getenv("PANEL_TG_ENABLED") == "" && botToken != "" {
			botEnabled = true
		}
	}
	var adminChats []int64
	envChatID := os.Getenv("PANEL_TG_CHAT_ID")
	if envChatID != "" {
		for _, s := range strings.Split(envChatID, ",") {
			s = strings.TrimSpace(s)
			if id, err := strconv.ParseInt(s, 10, 64); err == nil && id != 0 {
				adminChats = append(adminChats, id)
			}
		}
	} else {
		adminChats = dbChatIDs
	}
	telegramBot := bot.New(bot.Config{
		Token:      botToken,
		AdminChats: adminChats,
		Enabled:    botEnabled,
	}, database)
	telegramBot.Start()

	// Bot restart endpoint (hot-reload)
	mux.HandleFunc("/api/admin/bot/restart", srv.RequireAdmin(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Re-read config from DB
		newToken, newChatIDs := loadBotConfigFromDB(database)
		envToken := os.Getenv("PANEL_TG_BOT_TOKEN")
		if envToken != "" {
			newToken = envToken
		}
		envChat := os.Getenv("PANEL_TG_CHAT_ID")
		var chats []int64
		if envChat != "" {
			for _, s := range strings.Split(envChat, ",") {
				s = strings.TrimSpace(s)
				if id, err := strconv.ParseInt(s, 10, 64); err == nil && id != 0 {
					chats = append(chats, id)
				}
			}
		} else {
			chats = newChatIDs
		}
		enabled := newToken != ""
		telegramBot.Restart(bot.Config{
			Token:      newToken,
			AdminChats: chats,
			Enabled:    enabled,
		})
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"message":"bot restarted"}`))
	}))
}

// loadActiveGateways reads active payment gateways from the database and registers
// known gateway implementations in the server's PaymentRegistry.
func loadActiveGateways(database *sql.DB, srv *api.Server) {
	rows, err := database.Query(`SELECT name, COALESCE(config_json, '{}') FROM payment_gateways WHERE is_active = 1`)
	if err != nil {
		// Table might not exist on first run before migrations
		return
	}
	defer rows.Close()

	for rows.Next() {
		var name, configJSON string
		if err := rows.Scan(&name, &configJSON); err != nil {
			continue
		}
		switch name {
		case "zarinpal":
			var cfg struct {
				MerchantID string `json:"merchant_id"`
				Sandbox    bool   `json:"sandbox"`
			}
			if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil || cfg.MerchantID == "" {
				continue
			}
			gw := payment.NewZarinpal(cfg.MerchantID, cfg.Sandbox)
			srv.PaymentRegistry.Register(gw)
			logger.Info("payment", "registered gateway at startup", map[string]any{"name": name, "sandbox": cfg.Sandbox})
		}
	}
}

// processPaygBilling deducts wallet credit for customers on pay-as-you-go plans
// based on data usage (per GB) and time (per day).
func processPaygBilling(db *sql.DB) {
	type paygCustomer struct {
		ID               int64
		Username         string
		PlanID           int64
		PricePerGB       float64
		PricePerDay      float64
		DisconnectOnZero bool
		Credit           float64
	}

	rows, err := db.Query(`
		SELECT c.id, c.username, p.id, p.price_per_gb, p.price_per_day, p.disconnect_on_zero, w.credit
		FROM customers c
		JOIN plans p ON p.id = c.plan_id AND p.billing_type = 'payg'
		JOIN wallets w ON w.username = c.username
		WHERE c.status = 'active' AND c.deleted_at IS NULL
	`)
	if err != nil {
		logger.Error("worker", "payg billing query failed", map[string]any{"error": err.Error()})
		return
	}
	defer rows.Close()

	var customers []paygCustomer
	for rows.Next() {
		var c paygCustomer
		var disconn int
		if err := rows.Scan(&c.ID, &c.Username, &c.PlanID, &c.PricePerGB, &c.PricePerDay, &disconn, &c.Credit); err != nil {
			logger.Warn("worker", "payg scan error", map[string]any{"error": err.Error()})
			continue
		}
		c.DisconnectOnZero = disconn == 1
		customers = append(customers, c)
	}

	for _, c := range customers {
		// Get last deduction time for this user
		var lastDeduction time.Time
		err := db.QueryRow(`SELECT COALESCE(MAX(created_at), CAST('2000-01-01' AS DATETIME)) FROM payg_deductions WHERE username = ?`, c.Username).Scan(&lastDeduction)
		if err != nil {
			logger.Warn("worker", "payg last deduction query failed", map[string]any{"username": c.Username, "error": err.Error()})
			continue
		}

		// Calculate data used since last deduction (in bytes from radacct)
		var dataUsedBytes int64
		err = db.QueryRow(`
			SELECT COALESCE(SUM(acctinputoctets + acctoutputoctets), 0)
			FROM radacct
			WHERE username = ? AND (acctstarttime >= ? OR (acctstoptime IS NULL AND acctupdatetime >= ?))
		`, c.Username, lastDeduction, lastDeduction).Scan(&dataUsedBytes)
		if err != nil {
			logger.Warn("worker", "payg data usage query failed", map[string]any{"username": c.Username, "error": err.Error()})
			continue
		}

		// Calculate days since last deduction
		daysSinceLastDeduction := time.Since(lastDeduction).Hours() / 24.0

		// Calculate charges
		dataGB := float64(dataUsedBytes) / (1024 * 1024 * 1024)
		dataCharge := dataGB * c.PricePerGB
		timeCharge := daysSinceLastDeduction * c.PricePerDay
		totalCharge := dataCharge + timeCharge

		// Only deduct if charge is meaningful (> $0.001)
		if totalCharge < 0.001 {
			continue
		}

		balanceBefore := c.Credit
		balanceAfter := balanceBefore - totalCharge

		// Deduct from wallet
		_, err = db.Exec(`UPDATE wallets SET credit = credit - ? WHERE username = ?`, totalCharge, c.Username)
		if err != nil {
			logger.Warn("worker", "payg wallet deduction failed", map[string]any{"username": c.Username, "error": err.Error()})
			continue
		}

		// Record data deduction if applicable
		if dataCharge > 0.001 {
			_, _ = db.Exec(`INSERT INTO payg_deductions(customer_id, username, plan_id, deduction_type, amount, usage_value, balance_before, balance_after) VALUES(?,?,?,?,?,?,?,?)`,
				c.ID, c.Username, c.PlanID, "data", dataCharge, dataGB, balanceBefore, balanceAfter)
		}

		// Record time deduction if applicable
		if timeCharge > 0.001 {
			_, _ = db.Exec(`INSERT INTO payg_deductions(customer_id, username, plan_id, deduction_type, amount, usage_value, balance_before, balance_after) VALUES(?,?,?,?,?,?,?,?)`,
				c.ID, c.Username, c.PlanID, "time", timeCharge, daysSinceLastDeduction, balanceBefore, balanceAfter)
		}

		// If wallet credit <= 0 and disconnect_on_zero, limit the customer
		if balanceAfter <= 0 && c.DisconnectOnZero {
			_, _ = db.Exec(`UPDATE customers SET status = 'limited' WHERE id = ? AND status = 'active'`, c.ID)
			// Disconnect active sessions
			_, _ = db.Exec(`UPDATE radacct SET acctstoptime = NOW(), acctterminatecause = 'PAYG-Zero-Balance' WHERE username = ? AND acctstoptime IS NULL`, c.Username)
			logger.Info("worker", "payg: disconnected user (zero balance)", map[string]any{"username": c.Username})
		}
	}
}
