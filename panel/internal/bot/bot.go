// Package bot provides a native Telegram Bot integration for KorisPanel.
// Supports both webhook and long-polling modes.
// Admin commands: /stats, /users, /find, /create, /renew, /disable, /enable, /traffic
// Customer commands: /me, /usage, /plans
// Notifications: payments, node status, subscription expiry
package bot

import (
	"bytes"
	crypto_rand "crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config holds bot configuration.
type Config struct {
	Token      string // Telegram Bot API token
	AdminChats []int64 // Chat IDs that receive admin notifications
	WebhookURL string // If set, uses webhook mode; otherwise long-polling
	Enabled    bool
}

// Bot is the Telegram bot instance.
type Bot struct {
	cfg    Config
	db     *sql.DB
	client *http.Client
	apiURL string
	stopCh chan struct{}
	mu     sync.Mutex
}

// New creates a new Bot instance.
func New(cfg Config, db *sql.DB) *Bot {
	return &Bot{
		cfg:    cfg,
		db:     db,
		client: &http.Client{Timeout: 30 * time.Second},
		apiURL: "https://api.telegram.org/bot" + cfg.Token,
		stopCh: make(chan struct{}),
	}
}

// Start begins the bot in the configured mode (webhook or polling).
func (b *Bot) Start() {
	if !b.cfg.Enabled || b.cfg.Token == "" {
		log.Println("[bot] disabled or no token configured")
		return
	}

	if b.cfg.WebhookURL != "" {
		b.setupWebhook()
		log.Printf("[bot] webhook mode: %s", b.cfg.WebhookURL)
	} else {
		go b.pollLoop()
		log.Println("[bot] long-polling mode started")
	}
}

// Stop gracefully stops the bot.
func (b *Bot) Stop() {
	close(b.stopCh)
}

// WebhookHandler returns an HTTP handler for webhook mode.
func (b *Bot) WebhookHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var update Update
		if err := json.Unmarshal(body, &update); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		go b.handleUpdate(update)
		w.WriteHeader(http.StatusOK)
	}
}

// Notify sends a message to all admin chats.
func (b *Bot) Notify(message string) {
	if !b.cfg.Enabled || b.cfg.Token == "" {
		return
	}
	for _, chatID := range b.cfg.AdminChats {
		go b.sendMessage(chatID, message, "Markdown")
	}
}

// NotifyHTML sends an HTML-formatted message to admin chats.
func (b *Bot) NotifyHTML(message string) {
	if !b.cfg.Enabled || b.cfg.Token == "" {
		return
	}
	for _, chatID := range b.cfg.AdminChats {
		go b.sendMessage(chatID, message, "HTML")
	}
}

// ========== Telegram API Types ==========

type Update struct {
	ID            int64          `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

type Message struct {
	ID   int64  `json:"message_id"`
	From *User  `json:"from,omitempty"`
	Chat *Chat  `json:"chat"`
	Text string `json:"text"`
	Date int64  `json:"date"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    *User    `json:"from"`
	Message *Message `json:"message,omitempty"`
	Data    string   `json:"data"`
}

type User struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type InlineKeyboard struct {
	InlineKeyboard [][]InlineButton `json:"inline_keyboard"`
}

type InlineButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
}

// ========== Internal Methods ==========

func (b *Bot) setupWebhook() {
	payload := map[string]any{"url": b.cfg.WebhookURL}
	body, _ := json.Marshal(payload)
	resp, err := b.client.Post(b.apiURL+"/setWebhook", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[bot] setWebhook error: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		log.Printf("[bot] setWebhook failed: %s", resp.Status)
	}
}

func (b *Bot) pollLoop() {
	offset := int64(0)
	for {
		select {
		case <-b.stopCh:
			return
		default:
		}

		updates, err := b.getUpdates(offset)
		if err != nil {
			log.Printf("[bot] poll error: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}

		for _, u := range updates {
			if u.ID >= offset {
				offset = u.ID + 1
			}
			b.handleUpdate(u)
		}
	}
}

func (b *Bot) getUpdates(offset int64) ([]Update, error) {
	url := fmt.Sprintf("%s/getUpdates?offset=%d&timeout=25&allowed_updates=[\"message\",\"callback_query\"]", b.apiURL, offset)
	resp, err := b.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		OK     bool     `json:"ok"`
		Result []Update `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Result, nil
}

func (b *Bot) handleUpdate(u Update) {
	if u.CallbackQuery != nil {
		b.handleCallback(u.CallbackQuery)
		return
	}
	if u.Message == nil || u.Message.Text == "" {
		return
	}

	text := strings.TrimSpace(u.Message.Text)
	chatID := u.Message.Chat.ID

	// Check if this is an admin chat
	isAdmin := b.isAdminChat(chatID)

	parts := strings.Fields(text)
	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "/start":
		b.cmdStart(chatID, isAdmin)
	case "/help":
		b.cmdHelp(chatID, isAdmin)
	case "/stats":
		if isAdmin {
			b.cmdStats(chatID)
		}
	case "/users":
		if isAdmin {
			b.cmdUsers(chatID)
		}
	case "/find":
		if isAdmin && len(args) > 0 {
			b.cmdFind(chatID, args[0])
		}
	case "/create":
		if isAdmin && len(args) >= 2 {
			b.cmdCreate(chatID, args[0], args[1])
		} else if isAdmin {
			b.sendMessage(chatID, "Usage: `/create username password`", "Markdown")
		}
	case "/enable":
		if isAdmin && len(args) > 0 {
			b.cmdSetStatus(chatID, args[0], "active")
		}
	case "/disable":
		if isAdmin && len(args) > 0 {
			b.cmdSetStatus(chatID, args[0], "disabled")
		}
	case "/traffic":
		if isAdmin && len(args) > 0 {
			b.cmdTraffic(chatID, args[0])
		}
	case "/renew":
		if isAdmin && len(args) >= 2 {
			b.cmdRenew(chatID, args[0], args[1])
		} else if isAdmin {
			b.sendMessage(chatID, "Usage: `/renew username days`", "Markdown")
		}
	case "/broadcast":
		if isAdmin && len(args) > 0 {
			b.cmdBroadcast(chatID, strings.Join(args, " "))
		} else if isAdmin {
			b.sendMessage(chatID, "Usage: `/broadcast message`", "Markdown")
		}
	case "/online":
		if isAdmin {
			b.cmdOnline(chatID)
		}
	case "/backup":
		if isAdmin {
			b.cmdBackup(chatID)
		}
	case "/nodes":
		if isAdmin {
			b.cmdNodes(chatID)
		}
	case "/me":
		b.cmdMe(chatID, u.Message.From)
	case "/usage":
		b.cmdUsage(chatID, u.Message.From)
	case "/plans":
		b.cmdPlans(chatID)
	default:
		if strings.HasPrefix(cmd, "/") {
			b.sendMessage(chatID, "Unknown command. Use /help", "")
		}
	}
}

func (b *Bot) handleCallback(cb *CallbackQuery) {
	// Acknowledge callback
	b.answerCallback(cb.ID, "")

	chatID := cb.Message.Chat.ID
	data := cb.Data
	isAdmin := b.isAdminChat(chatID)

	if strings.HasPrefix(data, "user:") {
		if isAdmin {
			username := strings.TrimPrefix(data, "user:")
			b.cmdFind(chatID, username)
		}
	} else if strings.HasPrefix(data, "enable:") {
		username := strings.TrimPrefix(data, "enable:")
		if isAdmin {
			b.cmdSetStatus(chatID, username, "active")
		}
	} else if strings.HasPrefix(data, "disable:") {
		username := strings.TrimPrefix(data, "disable:")
		if isAdmin {
			b.cmdSetStatus(chatID, username, "disabled")
		}
	} else if strings.HasPrefix(data, "reset:") {
		username := strings.TrimPrefix(data, "reset:")
		if isAdmin {
			b.cmdTraffic(chatID, username)
		}
	} else if strings.HasPrefix(data, "menu:") {
		action := strings.TrimPrefix(data, "menu:")
		if isAdmin {
			switch action {
			case "stats":
				b.cmdStats(chatID)
			case "users":
				b.cmdUsers(chatID)
			case "online":
				b.cmdOnline(chatID)
			case "nodes":
				b.cmdNodes(chatID)
			case "backup":
				b.cmdBackup(chatID)
			case "me":
				b.cmdMe(chatID, cb.From)
			case "usage":
				b.cmdUsage(chatID, cb.From)
			case "plans":
				b.cmdPlans(chatID)
			}
		} else {
			// Customer menu actions
			switch action {
			case "me":
				b.cmdMe(chatID, cb.From)
			case "usage":
				b.cmdUsage(chatID, cb.From)
			case "plans":
				b.cmdPlans(chatID)
			}
		}
	}
}

// ========== Commands ==========

func (b *Bot) cmdStart(chatID int64, isAdmin bool) {
	if isAdmin {
		msg := "Welcome to *KorisPanel Bot*\n\nAdmin commands:\n" +
			"/stats — Panel statistics\n" +
			"/users — Recent users\n" +
			"/find `username` — User details\n" +
			"/create `user pass` — Create user\n" +
			"/enable `username` — Enable user\n" +
			"/disable `username` — Disable user\n" +
			"/traffic `username` — Reset traffic\n" +
			"/renew `username days` — Extend subscription\n" +
			"/broadcast `msg` — Message all admins\n" +
			"/online — Online users\n" +
			"/backup — Export backup\n" +
			"/nodes — Node status\n" +
			"/help — Show all commands"

		keyboard := InlineKeyboard{InlineKeyboard: [][]InlineButton{
			{{Text: "📊 Stats", CallbackData: "menu:stats"}, {Text: "👥 Users", CallbackData: "menu:users"}},
			{{Text: "🟢 Online", CallbackData: "menu:online"}, {Text: "🖥 Nodes", CallbackData: "menu:nodes"}},
			{{Text: "💾 Backup", CallbackData: "menu:backup"}},
		}}
		b.sendMessageWithKeyboard(chatID, msg, "Markdown", keyboard)
	} else {
		keyboard := InlineKeyboard{InlineKeyboard: [][]InlineButton{
			{{Text: "👤 My Account", CallbackData: "menu:me"}, {Text: "📊 Usage", CallbackData: "menu:usage"}},
			{{Text: "📋 Plans", CallbackData: "menu:plans"}},
		}}
		b.sendMessageWithKeyboard(chatID, "Welcome to *KorisPanel Bot*\n\nCommands:\n/me — Account info\n/usage — Data usage\n/plans — Available plans\n/help — Help", "Markdown", keyboard)
	}
}

func (b *Bot) cmdHelp(chatID int64, isAdmin bool) {
	b.cmdStart(chatID, isAdmin)
}

func (b *Bot) cmdStats(chatID int64) {
	var customers, active, nodes, pending int
	_ = b.db.QueryRow(`SELECT COUNT(*) FROM customers WHERE deleted_at IS NULL`).Scan(&customers)
	_ = b.db.QueryRow(`SELECT COUNT(*) FROM customers WHERE deleted_at IS NULL AND status='active'`).Scan(&active)
	_ = b.db.QueryRow(`SELECT COUNT(*) FROM nodes WHERE status IN('online','stale')`).Scan(&nodes)
	_ = b.db.QueryRow(`SELECT COUNT(*) FROM payments WHERE status='pending'`).Scan(&pending)

	var online int
	_ = b.db.QueryRow(`SELECT COUNT(DISTINCT username) FROM radacct WHERE acctstoptime IS NULL`).Scan(&online)

	msg := fmt.Sprintf("📊 *Panel Statistics*\n\n"+
		"👥 Users: %d (active: %d)\n"+
		"🟢 Online now: %d\n"+
		"🖥 Nodes: %d\n"+
		"💰 Pending payments: %d",
		customers, active, online, nodes, pending)
	b.sendMessage(chatID, msg, "Markdown")
}

func (b *Bot) cmdUsers(chatID int64) {
	rows, err := b.db.Query(`SELECT username, status FROM customers WHERE deleted_at IS NULL ORDER BY id DESC LIMIT 10`)
	if err != nil {
		b.sendMessage(chatID, "Error fetching users", "")
		return
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("👥 *Recent Users*\n\n")
	buttons := [][]InlineButton{}
	for rows.Next() {
		var username, status string
		if rows.Scan(&username, &status) == nil {
			icon := "🟢"
			if status == "disabled" {
				icon = "🔴"
			} else if status == "expired" {
				icon = "🟡"
			} else if status == "limited" {
				icon = "🟠"
			}
			sb.WriteString(fmt.Sprintf("%s `%s` — %s\n", icon, username, status))
			buttons = append(buttons, []InlineButton{{Text: username, CallbackData: "user:" + username}})
		}
	}
	b.sendMessageWithKeyboard(chatID, sb.String(), "Markdown", InlineKeyboard{InlineKeyboard: buttons})
}

func (b *Bot) cmdFind(chatID int64, username string) {
	username = strings.TrimSpace(username)
	var id int64
	var displayName, status, plan string
	var credit float64
	var created sql.NullTime
	err := b.db.QueryRow(`SELECT c.id, COALESCE(c.display_name,''), c.status, COALESCE(p.name,'Free'), COALESCE(w.credit,0), c.created_at
		FROM customers c
		LEFT JOIN plans p ON p.id=c.plan_id
		LEFT JOIN wallets w ON w.username=c.username
		WHERE c.username=? AND c.deleted_at IS NULL LIMIT 1`, username).Scan(&id, &displayName, &status, &plan, &credit, &created)
	if err == sql.ErrNoRows {
		b.sendMessage(chatID, fmt.Sprintf("User `%s` not found", username), "Markdown")
		return
	}
	if err != nil {
		b.sendMessage(chatID, "Database error", "")
		return
	}

	var totalBytes int64
	_ = b.db.QueryRow(`SELECT COALESCE(SUM(acctinputoctets+acctoutputoctets),0) FROM radacct WHERE username=?`, username).Scan(&totalBytes)

	var online int
	_ = b.db.QueryRow(`SELECT COUNT(*) FROM radacct WHERE username=? AND acctstoptime IS NULL`, username).Scan(&online)

	statusIcon := "🟢"
	if status == "disabled" {
		statusIcon = "🔴"
	} else if status == "expired" || status == "limited" {
		statusIcon = "🟡"
	}

	createdStr := "—"
	if created.Valid {
		createdStr = created.Time.Format("2006-01-02")
	}

	msg := fmt.Sprintf("👤 *User: %s*\n\n"+
		"%s Status: %s\n"+
		"📋 Plan: %s\n"+
		"💰 Balance: %.0f IRT\n"+
		"📊 Usage: %s\n"+
		"🔌 Online: %d session(s)\n"+
		"📅 Created: %s",
		username, statusIcon, status, plan, credit, formatBytesBot(totalBytes), online, createdStr)

	keyboard := InlineKeyboard{InlineKeyboard: [][]InlineButton{
		{{Text: "✅ Enable", CallbackData: "enable:" + username}, {Text: "🚫 Disable", CallbackData: "disable:" + username}},
		{{Text: "🔄 Reset Traffic", CallbackData: "reset:" + username}},
	}}
	b.sendMessageWithKeyboard(chatID, msg, "Markdown", keyboard)
}

func (b *Bot) cmdCreate(chatID int64, username, password string) {
	username = strings.TrimSpace(username)
	if len(username) < 3 || len(password) < 4 {
		b.sendMessage(chatID, "Username min 3 chars, password min 4 chars", "")
		return
	}

	tx, err := b.db.Begin()
	if err != nil {
		b.sendMessage(chatID, "Database error", "")
		return
	}
	defer tx.Rollback()

	res, err := tx.Exec(`INSERT INTO customers(username, sub_token, created_by) VALUES(?, ?, 'telegram_bot')`, username, randomHex(24))
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("Failed: %s", err.Error()), "")
		return
	}
	customerID, _ := res.LastInsertId()
	_, _ = tx.Exec(`INSERT INTO wallets(customer_id, username, credit) VALUES(?,?,0)`, customerID, username)
	_, _ = tx.Exec(`INSERT INTO radcheck(username, attribute, op, value) VALUES(?,'Cleartext-Password',':=',?)`, username, password)
	_, _ = tx.Exec(`INSERT INTO radcheck(username, attribute, op, value) VALUES(?,'Simultaneous-Use',':=','1')`, username)

	if err := tx.Commit(); err != nil {
		b.sendMessage(chatID, "Failed to commit", "")
		return
	}

	b.sendMessage(chatID, fmt.Sprintf("✅ User created\n\nUsername: `%s`\nPassword: `%s`", username, password), "Markdown")
}

func (b *Bot) cmdSetStatus(chatID int64, username, status string) {
	result, err := b.db.Exec(`UPDATE customers SET status=? WHERE username=? AND deleted_at IS NULL`, status, username)
	if err != nil {
		b.sendMessage(chatID, "Error: "+err.Error(), "")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		b.sendMessage(chatID, fmt.Sprintf("User `%s` not found", username), "Markdown")
		return
	}
	icon := "✅"
	if status == "disabled" {
		icon = "🚫"
	}
	b.sendMessage(chatID, fmt.Sprintf("%s User `%s` is now *%s*", icon, username, status), "Markdown")
}

func (b *Bot) cmdTraffic(chatID int64, username string) {
	result, err := b.db.Exec(`UPDATE radacct SET acctstoptime=COALESCE(acctstoptime, NOW()), acctterminatecause=COALESCE(acctterminatecause, 'Admin-Reset') WHERE username=?`, username)
	if err != nil {
		b.sendMessage(chatID, "Error: "+err.Error(), "")
		return
	}
	affected, _ := result.RowsAffected()
	_, _ = b.db.Exec(`UPDATE customers SET status='active' WHERE username=? AND status='limited' AND deleted_at IS NULL`, username)
	b.sendMessage(chatID, fmt.Sprintf("🔄 Traffic reset for `%s`\n%d sessions archived", username, affected), "Markdown")
}

func (b *Bot) cmdMe(chatID int64, from *User) {
	if from == nil {
		b.sendMessage(chatID, "Could not identify you", "")
		return
	}
	// Try to find user by telegram username
	tgUsername := strings.ToLower(from.Username)
	if tgUsername == "" {
		b.sendMessage(chatID, "Set a Telegram username first, or use /find in admin mode", "")
		return
	}

	var username, status, plan string
	var credit float64
	err := b.db.QueryRow(`SELECT c.username, c.status, COALESCE(p.name,'Free'), COALESCE(w.credit,0)
		FROM customers c
		LEFT JOIN plans p ON p.id=c.plan_id
		LEFT JOIN wallets w ON w.username=c.username
		WHERE c.username=? AND c.deleted_at IS NULL LIMIT 1`, tgUsername).Scan(&username, &status, &plan, &credit)
	if err != nil {
		b.sendMessage(chatID, "No account linked to your Telegram username. Contact admin.", "")
		return
	}

	msg := fmt.Sprintf("👤 *Your Account*\n\n"+
		"Username: `%s`\n"+
		"Status: %s\n"+
		"Plan: %s\n"+
		"Balance: %.0f IRT", username, status, plan, credit)
	b.sendMessage(chatID, msg, "Markdown")
}

func (b *Bot) cmdUsage(chatID int64, from *User) {
	if from == nil || from.Username == "" {
		b.sendMessage(chatID, "Cannot identify your account", "")
		return
	}
	username := strings.ToLower(from.Username)

	var totalBytes int64
	_ = b.db.QueryRow(`SELECT COALESCE(SUM(acctinputoctets+acctoutputoctets),0) FROM radacct WHERE username=?`, username).Scan(&totalBytes)

	var maxData int64
	_ = b.db.QueryRow(`SELECT COALESCE(CAST(value AS UNSIGNED),0) FROM radcheck WHERE username=? AND attribute='Max-Data' ORDER BY id DESC LIMIT 1`, username).Scan(&maxData)

	remaining := "Unlimited"
	pct := ""
	if maxData > 0 {
		left := maxData - totalBytes
		if left < 0 {
			left = 0
		}
		remaining = formatBytesBot(left)
		pct = fmt.Sprintf(" (%.1f%%)", float64(totalBytes)/float64(maxData)*100)
	}

	msg := fmt.Sprintf("📊 *Your Usage*\n\n"+
		"Total used: %s%s\n"+
		"Remaining: %s\n"+
		"Limit: %s",
		formatBytesBot(totalBytes), pct, remaining, func() string {
			if maxData > 0 {
				return formatBytesBot(maxData)
			}
			return "Unlimited"
		}())
	b.sendMessage(chatID, msg, "Markdown")
}

func (b *Bot) cmdPlans(chatID int64) {
	rows, err := b.db.Query(`SELECT name, data_gb, speed_mbps, duration_days, price FROM plans WHERE is_active=1 ORDER BY sort_order, id`)
	if err != nil {
		b.sendMessage(chatID, "Error loading plans", "")
		return
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("📋 *Available Plans*\n\n")
	for rows.Next() {
		var name string
		var dataGB, speedMbps, price float64
		var days int
		if rows.Scan(&name, &dataGB, &speedMbps, &days, &price) == nil {
			dataStr := "Unlimited"
			if dataGB > 0 {
				dataStr = fmt.Sprintf("%.0f GB", dataGB)
			}
			speedStr := "Unlimited"
			if speedMbps > 0 {
				speedStr = fmt.Sprintf("%.0f Mbps", speedMbps)
			}
			sb.WriteString(fmt.Sprintf("• *%s*\n  %s · %s · %dd · %.0f IRT\n\n", name, dataStr, speedStr, days, price))
		}
	}
	b.sendMessage(chatID, sb.String(), "Markdown")
}

func (b *Bot) cmdRenew(chatID int64, username string, daysStr string) {
	days, err := strconv.Atoi(daysStr)
	if err != nil || days <= 0 {
		b.sendMessage(chatID, "Days must be a positive number", "")
		return
	}
	username = strings.TrimSpace(username)

	// Try to extend active subscription
	result, err := b.db.Exec(
		`UPDATE subscriptions SET expires_at = DATE_ADD(expires_at, INTERVAL ? DAY)
		 WHERE username = ? AND status = 'active' ORDER BY id DESC LIMIT 1`,
		days, username,
	)
	if err != nil {
		b.sendMessage(chatID, "Error: "+err.Error(), "")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		// No active subscription, try to reactivate expired one
		result, err = b.db.Exec(
			`UPDATE subscriptions SET expires_at = DATE_ADD(NOW(), INTERVAL ? DAY), status = 'active'
			 WHERE username = ? ORDER BY id DESC LIMIT 1`,
			days, username,
		)
		if err != nil {
			b.sendMessage(chatID, "Error: "+err.Error(), "")
			return
		}
		affected, _ = result.RowsAffected()
		if affected == 0 {
			b.sendMessage(chatID, fmt.Sprintf("User `%s` not found or has no subscription", username), "Markdown")
			return
		}
		// Also reactivate customer
		_, _ = b.db.Exec(`UPDATE customers SET status = 'active' WHERE username = ? AND deleted_at IS NULL`, username)
	}
	b.sendMessage(chatID, fmt.Sprintf("✅ Extended `%s` subscription by *%d* day(s)", username, days), "Markdown")
}

func (b *Bot) cmdBroadcast(chatID int64, message string) {
	if message == "" {
		b.sendMessage(chatID, "Message cannot be empty", "")
		return
	}
	count := 0
	for _, adminChat := range b.cfg.AdminChats {
		b.sendMessage(adminChat, "📢 *Broadcast*\n\n"+message, "Markdown")
		count++
	}
	b.sendMessage(chatID, fmt.Sprintf("✅ Broadcast sent to %d admin chat(s)", count), "")
}

func (b *Bot) cmdOnline(chatID int64) {
	rows, err := b.db.Query(
		`SELECT username, COUNT(*) as sessions, COALESCE(SUM(acctinputoctets+acctoutputoctets), 0) as bytes
		 FROM radacct WHERE acctstoptime IS NULL
		 GROUP BY username ORDER BY sessions DESC LIMIT 20`)
	if err != nil {
		b.sendMessage(chatID, "Error fetching online users", "")
		return
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("🟢 *Online Users*\n\n")
	total := 0
	for rows.Next() {
		var username string
		var sessions int
		var totalBytes int64
		if rows.Scan(&username, &sessions, &totalBytes) == nil {
			sb.WriteString(fmt.Sprintf("• `%s` — %d session(s) — %s\n", username, sessions, formatBytesBot(totalBytes)))
			total++
		}
	}
	if total == 0 {
		sb.WriteString("No users currently online.")
	} else {
		sb.WriteString(fmt.Sprintf("\n*Total: %d user(s)*", total))
	}
	b.sendMessage(chatID, sb.String(), "Markdown")
}

func (b *Bot) cmdBackup(chatID int64) {
	b.sendMessage(chatID, "💾 Generating backup...\n\nPlease use the admin panel Settings > Backup to download the full JSON backup. Telegram file sending is not supported in this version.", "Markdown")
}

func (b *Bot) cmdNodes(chatID int64) {
	rows, err := b.db.Query(
		`SELECT n.name, n.status, n.public_ip, COALESCE(n.last_seen_at, n.created_at)
		 FROM nodes n ORDER BY n.id`)
	if err != nil {
		b.sendMessage(chatID, "Error fetching nodes", "")
		return
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("🖥 *Nodes*\n\n")
	count := 0
	for rows.Next() {
		var name, status, ip string
		var lastSeen sql.NullTime
		if rows.Scan(&name, &status, &ip, &lastSeen) == nil {
			icon := "🟢"
			if status == "offline" {
				icon = "🔴"
			} else if status == "stale" {
				icon = "🟡"
			}
			seenStr := "never"
			if lastSeen.Valid {
				seenStr = lastSeen.Time.Format("01-02 15:04")
			}
			sb.WriteString(fmt.Sprintf("%s *%s* (%s)\n   IP: `%s` | Last seen: %s\n\n", icon, name, status, ip, seenStr))
			count++
		}
	}
	if count == 0 {
		sb.WriteString("No nodes configured.")
	}
	b.sendMessage(chatID, sb.String(), "Markdown")
}

// NotifyPayment sends a payment notification to admin chats.
func (b *Bot) NotifyPayment(username string, amount float64, method string) {
	msg := fmt.Sprintf("💰 *New Payment*\n\nUser: `%s`\nAmount: %.2f\nMethod: %s\nStatus: pending",
		username, amount, method)
	b.Notify(msg)
}

// NotifyNodeDown sends a node-down notification.
func (b *Bot) NotifyNodeDown(nodeName, nodeIP string) {
	msg := fmt.Sprintf("🔴 *Node Offline*\n\nNode: *%s*\nIP: `%s`\nDetected: %s",
		nodeName, nodeIP, time.Now().Format("15:04:05"))
	b.Notify(msg)
}

// NotifyExpiryWarning sends a subscription expiry warning to admins.
func (b *Bot) NotifyExpiryWarning(username string, daysLeft int) {
	msg := fmt.Sprintf("⏰ *Expiry Warning*\n\nUser: `%s`\nDays remaining: %d",
		username, daysLeft)
	b.Notify(msg)
}

// ========== Telegram API Calls ==========

func (b *Bot) sendMessage(chatID int64, text, parseMode string) {
	payload := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}
	if parseMode != "" {
		payload["parse_mode"] = parseMode
	}
	b.apiCall("sendMessage", payload)
}

func (b *Bot) sendMessageWithKeyboard(chatID int64, text, parseMode string, keyboard InlineKeyboard) {
	payload := map[string]any{
		"chat_id":      chatID,
		"text":         text,
		"reply_markup": keyboard,
	}
	if parseMode != "" {
		payload["parse_mode"] = parseMode
	}
	b.apiCall("sendMessage", payload)
}

func (b *Bot) answerCallback(callbackID, text string) {
	payload := map[string]any{
		"callback_query_id": callbackID,
	}
	if text != "" {
		payload["text"] = text
	}
	b.apiCall("answerCallbackQuery", payload)
}

func (b *Bot) apiCall(method string, payload map[string]any) {
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	resp, err := b.client.Post(b.apiURL+"/"+method, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[bot] API %s error: %v", method, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		log.Printf("[bot] API %s failed %s: %s", method, resp.Status, string(respBody))
	}
}

func (b *Bot) isAdminChat(chatID int64) bool {
	for _, id := range b.cfg.AdminChats {
		if id == chatID {
			return true
		}
	}
	return false
}

// ========== Helpers ==========

func formatBytesBot(n int64) string {
	if n >= 1<<40 {
		return fmt.Sprintf("%.2f TB", float64(n)/float64(1<<40))
	}
	if n >= 1<<30 {
		return fmt.Sprintf("%.2f GB", float64(n)/float64(1<<30))
	}
	if n >= 1<<20 {
		return fmt.Sprintf("%.2f MB", float64(n)/float64(1<<20))
	}
	if n >= 1<<10 {
		return fmt.Sprintf("%.1f KB", float64(n)/float64(1<<10))
	}
	return strconv.FormatInt(n, 10) + " B"
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := crypto_rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}
