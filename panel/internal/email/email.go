package email

import (
	"fmt"
	"net/smtp"
	"os"
	"strings"
)

type Config struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
	Enabled  bool
}

func LoadConfig() Config {
	return Config{
		Host:     os.Getenv("PANEL_SMTP_HOST"),
		Port:     os.Getenv("PANEL_SMTP_PORT"),
		Username: os.Getenv("PANEL_SMTP_USER"),
		Password: os.Getenv("PANEL_SMTP_PASS"),
		From:     os.Getenv("PANEL_SMTP_FROM"),
		Enabled:  strings.ToLower(os.Getenv("PANEL_SMTP_ENABLED")) == "true",
	}
}

type Sender struct {
	cfg Config
}

func New(cfg Config) *Sender {
	return &Sender{cfg: cfg}
}

func (s *Sender) Send(to, subject, body string) error {
	if !s.cfg.Enabled || s.cfg.Host == "" {
		return nil
	}
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s",
		s.cfg.From, to, subject, body)
	addr := s.cfg.Host + ":" + s.cfg.Port
	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	return smtp.SendMail(addr, auth, s.cfg.From, []string{to}, []byte(msg))
}

func (s *Sender) SendExpiryWarning(to, username string, daysLeft int) error {
	subject := fmt.Sprintf("KorisPanel: Your subscription expires in %d day(s)", daysLeft)
	body := fmt.Sprintf(`<h2>Subscription Expiring Soon</h2>
<p>Hi <b>%s</b>,</p>
<p>Your VPN subscription will expire in <b>%d day(s)</b>.</p>
<p>Please renew your plan to avoid service interruption.</p>
<p>— KorisPanel</p>`, username, daysLeft)
	return s.Send(to, subject, body)
}

func (s *Sender) SendPaymentReceipt(to, username string, amount float64, method string) error {
	subject := "KorisPanel: Payment Confirmed"
	body := fmt.Sprintf(`<h2>Payment Received</h2>
<p>Hi <b>%s</b>,</p>
<p>Your payment of <b>%.0f IRT</b> via <b>%s</b> has been approved.</p>
<p>Your wallet has been credited.</p>
<p>— KorisPanel</p>`, username, amount, method)
	return s.Send(to, subject, body)
}
