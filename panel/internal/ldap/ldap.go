//go:build !lite

package ldap

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	goldap "github.com/go-ldap/ldap/v3"
)

// LDAPConfig holds the LDAP/AD connection and search configuration.
type LDAPConfig struct {
	Enabled       bool   `json:"enabled"`
	ServerURL     string `json:"server_url"`    // e.g. ldap://dc.example.com:389 or ldaps://dc.example.com:636
	BindDN        string `json:"bind_dn"`       // e.g. cn=admin,dc=example,dc=com
	BindPassword  string `json:"bind_password"` // service account password
	BaseDN        string `json:"base_dn"`       // e.g. dc=example,dc=com
	UserFilter    string `json:"user_filter"`   // e.g. (&(objectClass=user)(sAMAccountName=%s))
	GroupFilter   string `json:"group_filter"`  // e.g. (&(objectClass=group)(member=%s))
	UsernameAttr  string `json:"username_attr"` // e.g. sAMAccountName or uid
	EmailAttr     string `json:"email_attr"`    // e.g. mail
	TLSEnabled    bool   `json:"tls_enabled"`   // use StartTLS (for ldap://) or connect via ldaps://
	TLSSkipVerify bool   `json:"tls_skip_verify"`
}

// LDAPUser represents a user authenticated via LDAP.
type LDAPUser struct {
	Username    string   `json:"username"`
	Email       string   `json:"email"`
	DisplayName string   `json:"display_name"`
	Groups      []string `json:"groups"`
	DN          string   `json:"dn"`
}

// LDAPService provides LDAP authentication with fallback to local DB auth.
type LDAPService struct {
	config LDAPConfig
	db     *sql.DB
}

// New creates a new LDAPService with the given config.
func New(config LDAPConfig, db *sql.DB) *LDAPService {
	return &LDAPService{config: config, db: db}
}

// Config returns the current LDAP configuration.
func (s *LDAPService) Config() LDAPConfig {
	return s.config
}

// IsEnabled returns whether LDAP authentication is enabled.
func (s *LDAPService) IsEnabled() bool {
	return s.config.Enabled
}

// Validate checks that required fields are present when LDAP is enabled.
func (c *LDAPConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.ServerURL == "" {
		return errors.New("server_url is required when LDAP is enabled")
	}
	if c.BindDN == "" {
		return errors.New("bind_dn is required when LDAP is enabled")
	}
	if c.BindPassword == "" {
		return errors.New("bind_password is required when LDAP is enabled")
	}
	if c.BaseDN == "" {
		return errors.New("base_dn is required when LDAP is enabled")
	}
	if c.UserFilter == "" {
		return errors.New("user_filter is required when LDAP is enabled")
	}
	if c.UsernameAttr == "" {
		return errors.New("username_attr is required when LDAP is enabled")
	}
	if !strings.HasPrefix(c.ServerURL, "ldap://") && !strings.HasPrefix(c.ServerURL, "ldaps://") {
		return errors.New("server_url must start with ldap:// or ldaps://")
	}
	return nil
}

// connect dials the LDAP server and applies TLS settings.
func (s *LDAPService) connect() (*goldap.Conn, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: s.config.TLSSkipVerify,
	}

	var conn *goldap.Conn
	var err error

	if strings.HasPrefix(s.config.ServerURL, "ldaps://") {
		conn, err = goldap.DialURL(s.config.ServerURL, goldap.DialWithTLSConfig(tlsConfig))
	} else {
		conn, err = goldap.DialURL(s.config.ServerURL)
		if err == nil && s.config.TLSEnabled {
			err = conn.StartTLS(tlsConfig)
			if err != nil {
				conn.Close()
				return nil, fmt.Errorf("StartTLS failed: %w", err)
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("ldap dial: %w", err)
	}
	return conn, nil
}

// TestConnection tests the LDAP connection with the configured bind credentials.
func (s *LDAPService) TestConnection(ctx context.Context) error {
	if !s.config.Enabled {
		return errors.New("ldap is not enabled")
	}
	if err := s.config.Validate(); err != nil {
		return err
	}

	conn, err := s.connect()
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer conn.Close()

	err = conn.Bind(s.config.BindDN, s.config.BindPassword)
	if err != nil {
		return fmt.Errorf("bind failed: %w", err)
	}

	log.Printf("[ldap] connection test successful to %s", s.config.ServerURL)
	return nil
}

// Authenticate attempts to authenticate a user via LDAP.
// It first binds with the service account, searches for the user, then re-binds as the user.
func (s *LDAPService) Authenticate(ctx context.Context, username, password string) (*LDAPUser, error) {
	if !s.config.Enabled {
		return nil, errors.New("ldap is not enabled")
	}

	conn, err := s.connect()
	if err != nil {
		return nil, fmt.Errorf("ldap connect: %w", err)
	}
	defer conn.Close()

	// Bind with service account to search
	if err := conn.Bind(s.config.BindDN, s.config.BindPassword); err != nil {
		return nil, fmt.Errorf("service bind: %w", err)
	}

	// Build user search filter — replace %s with the username
	filter := strings.ReplaceAll(s.config.UserFilter, "%s", goldap.EscapeFilter(username))

	attrs := []string{"dn", s.config.UsernameAttr}
	if s.config.EmailAttr != "" {
		attrs = append(attrs, s.config.EmailAttr)
	}
	attrs = append(attrs, "displayName", "cn")

	searchReq := goldap.NewSearchRequest(
		s.config.BaseDN,
		goldap.ScopeWholeSubtree,
		goldap.NeverDerefAliases,
		1,  // size limit
		10, // time limit seconds
		false,
		filter,
		attrs,
		nil,
	)

	result, err := conn.Search(searchReq)
	if err != nil {
		return nil, fmt.Errorf("ldap search: %w", err)
	}
	if len(result.Entries) == 0 {
		return nil, errors.New("user not found in LDAP")
	}

	entry := result.Entries[0]
	userDN := entry.DN

	// Bind as the user to verify password
	if err := conn.Bind(userDN, password); err != nil {
		return nil, fmt.Errorf("user bind failed: %w", err)
	}

	// Extract user attributes
	user := &LDAPUser{
		Username: entry.GetAttributeValue(s.config.UsernameAttr),
		DN:       userDN,
	}
	if s.config.EmailAttr != "" {
		user.Email = entry.GetAttributeValue(s.config.EmailAttr)
	}
	displayName := entry.GetAttributeValue("displayName")
	if displayName == "" {
		displayName = entry.GetAttributeValue("cn")
	}
	user.DisplayName = displayName

	// Fetch groups if group filter is configured
	if s.config.GroupFilter != "" {
		user.Groups = s.fetchGroups(conn, userDN)
	}

	log.Printf("[ldap] user %q authenticated successfully", username)
	return user, nil
}

// fetchGroups searches for groups the user belongs to.
func (s *LDAPService) fetchGroups(conn *goldap.Conn, userDN string) []string {
	// Re-bind as service account for group search
	if err := conn.Bind(s.config.BindDN, s.config.BindPassword); err != nil {
		log.Printf("[ldap] re-bind for group search failed: %v", err)
		return nil
	}

	filter := strings.ReplaceAll(s.config.GroupFilter, "%s", goldap.EscapeFilter(userDN))
	searchReq := goldap.NewSearchRequest(
		s.config.BaseDN,
		goldap.ScopeWholeSubtree,
		goldap.NeverDerefAliases,
		100, // max groups
		10,
		false,
		filter,
		[]string{"cn"},
		nil,
	)

	result, err := conn.Search(searchReq)
	if err != nil {
		log.Printf("[ldap] group search failed: %v", err)
		return nil
	}

	groups := make([]string, 0, len(result.Entries))
	for _, entry := range result.Entries {
		cn := entry.GetAttributeValue("cn")
		if cn != "" {
			groups = append(groups, cn)
		}
	}
	return groups
}

// LoadConfigFromDB reads LDAP configuration from the panel_settings table.
func LoadConfigFromDB(db *sql.DB) LDAPConfig {
	var raw string
	err := db.QueryRow(`SELECT setting_value FROM panel_settings WHERE setting_key = 'ldap_config'`).Scan(&raw)
	if err != nil {
		return LDAPConfig{}
	}
	var cfg LDAPConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		log.Printf("[ldap] failed to parse config from DB: %v", err)
		return LDAPConfig{}
	}
	return cfg
}

// SaveConfigToDB persists LDAP configuration to the panel_settings table.
func SaveConfigToDB(db *sql.DB, cfg LDAPConfig) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal ldap config: %w", err)
	}
	_, err = db.Exec(
		`INSERT INTO panel_settings (setting_key, setting_value) VALUES ('ldap_config', ?) ON DUPLICATE KEY UPDATE setting_value = VALUES(setting_value)`,
		string(data),
	)
	return err
}

// MaskedConfig returns the config with the bind password masked for safe API responses.
func (c *LDAPConfig) MaskedConfig() LDAPConfig {
	masked := *c
	if masked.BindPassword != "" {
		masked.BindPassword = "********"
	}
	return masked
}
