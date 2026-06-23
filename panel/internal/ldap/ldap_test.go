//go:build !lite

package ldap

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestLDAPConfig_Validate_Disabled(t *testing.T) {
	cfg := LDAPConfig{Enabled: false}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error for disabled config, got: %v", err)
	}
}

func TestLDAPConfig_Validate_RequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		cfg     LDAPConfig
		wantErr string
	}{
		{
			name:    "missing server_url",
			cfg:     LDAPConfig{Enabled: true},
			wantErr: "server_url is required",
		},
		{
			name: "invalid server_url scheme",
			cfg: LDAPConfig{
				Enabled:      true,
				ServerURL:    "http://bad.example.com",
				BindDN:       "cn=admin,dc=example,dc=com",
				BindPassword: "secret",
				BaseDN:       "dc=example,dc=com",
				UserFilter:   "(uid=%s)",
				UsernameAttr: "uid",
			},
			wantErr: "server_url must start with ldap:// or ldaps://",
		},
		{
			name:    "missing bind_dn",
			cfg:     LDAPConfig{Enabled: true, ServerURL: "ldap://dc.example.com:389"},
			wantErr: "bind_dn is required",
		},
		{
			name:    "missing bind_password",
			cfg:     LDAPConfig{Enabled: true, ServerURL: "ldap://dc.example.com:389", BindDN: "cn=admin,dc=example,dc=com"},
			wantErr: "bind_password is required",
		},
		{
			name: "missing base_dn",
			cfg: LDAPConfig{
				Enabled:      true,
				ServerURL:    "ldap://dc.example.com:389",
				BindDN:       "cn=admin,dc=example,dc=com",
				BindPassword: "secret",
			},
			wantErr: "base_dn is required",
		},
		{
			name: "missing user_filter",
			cfg: LDAPConfig{
				Enabled:      true,
				ServerURL:    "ldap://dc.example.com:389",
				BindDN:       "cn=admin,dc=example,dc=com",
				BindPassword: "secret",
				BaseDN:       "dc=example,dc=com",
			},
			wantErr: "user_filter is required",
		},
		{
			name: "missing username_attr",
			cfg: LDAPConfig{
				Enabled:      true,
				ServerURL:    "ldap://dc.example.com:389",
				BindDN:       "cn=admin,dc=example,dc=com",
				BindPassword: "secret",
				BaseDN:       "dc=example,dc=com",
				UserFilter:   "(&(objectClass=user)(sAMAccountName=%s))",
			},
			wantErr: "username_attr is required",
		},
		{
			name: "valid config",
			cfg: LDAPConfig{
				Enabled:      true,
				ServerURL:    "ldaps://dc.example.com:636",
				BindDN:       "cn=admin,dc=example,dc=com",
				BindPassword: "secret",
				BaseDN:       "dc=example,dc=com",
				UserFilter:   "(&(objectClass=user)(sAMAccountName=%s))",
				UsernameAttr: "sAMAccountName",
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
				}
			}
		})
	}
}

func TestLDAPConfig_MaskedConfig(t *testing.T) {
	cfg := LDAPConfig{
		Enabled:      true,
		ServerURL:    "ldap://dc.example.com:389",
		BindDN:       "cn=admin,dc=example,dc=com",
		BindPassword: "supersecretpassword",
		BaseDN:       "dc=example,dc=com",
		UserFilter:   "(&(objectClass=user)(sAMAccountName=%s))",
		UsernameAttr: "sAMAccountName",
		EmailAttr:    "mail",
	}

	masked := cfg.MaskedConfig()
	if masked.BindPassword != "********" {
		t.Fatalf("expected masked password, got: %s", masked.BindPassword)
	}
	if masked.ServerURL != cfg.ServerURL {
		t.Fatal("other fields should not be masked")
	}
	// Original should be unchanged
	if cfg.BindPassword != "supersecretpassword" {
		t.Fatal("original config should not be mutated")
	}
}

func TestLDAPConfig_MaskedConfig_EmptyPassword(t *testing.T) {
	cfg := LDAPConfig{Enabled: false, BindPassword: ""}
	masked := cfg.MaskedConfig()
	if masked.BindPassword != "" {
		t.Fatalf("empty password should remain empty when masked, got: %s", masked.BindPassword)
	}
}

func TestTestConnection_NotEnabled(t *testing.T) {
	svc := New(LDAPConfig{Enabled: false}, nil)
	err := svc.TestConnection(nil)
	if err == nil {
		t.Fatal("expected error when LDAP is not enabled")
	}
	if !contains(err.Error(), "not enabled") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTestConnection_InvalidConfig(t *testing.T) {
	cfg := LDAPConfig{Enabled: true, ServerURL: ""}
	svc := New(cfg, nil)
	err := svc.TestConnection(nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !contains(err.Error(), "server_url is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthenticate_NotEnabled(t *testing.T) {
	svc := New(LDAPConfig{Enabled: false}, nil)
	_, err := svc.Authenticate(nil, "user", "pass")
	if err == nil {
		t.Fatal("expected error when LDAP is not enabled")
	}
	if !contains(err.Error(), "not enabled") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthenticate_ConnectionError(t *testing.T) {
	cfg := LDAPConfig{
		Enabled:      true,
		ServerURL:    "ldap://127.0.0.1:1",
		BindDN:       "cn=admin,dc=test,dc=com",
		BindPassword: "pass",
		BaseDN:       "dc=test,dc=com",
		UserFilter:   "(uid=%s)",
		UsernameAttr: "uid",
	}
	svc := New(cfg, nil)
	_, err := svc.Authenticate(nil, "user", "pass")
	if err == nil {
		t.Fatal("expected connection error")
	}
}

func TestLoadConfigFromDB_NoRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT setting_value FROM panel_settings").
		WillReturnRows(sqlmock.NewRows([]string{"setting_value"}))

	cfg := LoadConfigFromDB(db)
	if cfg.Enabled {
		t.Fatal("expected disabled config when no rows")
	}
}

func TestLoadConfigFromDB_ValidJSON(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	configJSON := `{"enabled":true,"server_url":"ldap://dc.example.com:389","bind_dn":"cn=admin,dc=example,dc=com","bind_password":"secret","base_dn":"dc=example,dc=com","user_filter":"(uid=%s)","username_attr":"uid"}`
	mock.ExpectQuery("SELECT setting_value FROM panel_settings").
		WillReturnRows(sqlmock.NewRows([]string{"setting_value"}).AddRow(configJSON))

	cfg := LoadConfigFromDB(db)
	if !cfg.Enabled {
		t.Fatal("expected enabled config")
	}
	if cfg.ServerURL != "ldap://dc.example.com:389" {
		t.Fatalf("unexpected server_url: %s", cfg.ServerURL)
	}
	if cfg.BindPassword != "secret" {
		t.Fatalf("unexpected bind_password: %s", cfg.BindPassword)
	}
}

func TestSaveConfigToDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cfg := LDAPConfig{
		Enabled:      true,
		ServerURL:    "ldap://dc.example.com:389",
		BindDN:       "cn=admin,dc=example,dc=com",
		BindPassword: "secret",
		BaseDN:       "dc=example,dc=com",
		UserFilter:   "(uid=%s)",
		UsernameAttr: "uid",
	}

	mock.ExpectExec("INSERT INTO panel_settings").
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := SaveConfigToDB(db, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestIsEnabled(t *testing.T) {
	svc := New(LDAPConfig{Enabled: true}, nil)
	if !svc.IsEnabled() {
		t.Fatal("expected IsEnabled to return true")
	}
	svc2 := New(LDAPConfig{Enabled: false}, nil)
	if svc2.IsEnabled() {
		t.Fatal("expected IsEnabled to return false")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
