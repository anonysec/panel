//go:build !lite

package teleproxy

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGenerateLinks(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	svc := New(db)

	tests := []struct {
		name      string
		proxy     *Proxy
		nodeIP    string
		wantShare string
		wantTg    string
	}{
		{
			name:      "standard IPv4",
			proxy:     &Proxy{Port: 443, Secret: "abcdef1234567890abcdef1234567890"},
			nodeIP:    "192.168.1.100",
			wantShare: "https://t.me/proxy?server=192.168.1.100&port=443&secret=abcdef1234567890abcdef1234567890",
			wantTg:    "tg://proxy?server=192.168.1.100&port=443&secret=abcdef1234567890abcdef1234567890",
		},
		{
			name:      "different port",
			proxy:     &Proxy{Port: 8443, Secret: "1111222233334444aaaabbbbccccdddd"},
			nodeIP:    "10.0.0.1",
			wantShare: "https://t.me/proxy?server=10.0.0.1&port=8443&secret=1111222233334444aaaabbbbccccdddd",
			wantTg:    "tg://proxy?server=10.0.0.1&port=8443&secret=1111222233334444aaaabbbbccccdddd",
		},
		{
			name:      "public IP with standard secret",
			proxy:     &Proxy{Port: 2096, Secret: "ee00ff11aa22bb33cc44dd55ee66ff77"},
			nodeIP:    "91.107.168.34",
			wantShare: "https://t.me/proxy?server=91.107.168.34&port=2096&secret=ee00ff11aa22bb33cc44dd55ee66ff77",
			wantTg:    "tg://proxy?server=91.107.168.34&port=2096&secret=ee00ff11aa22bb33cc44dd55ee66ff77",
		},
		{
			name:      "domain name as IP",
			proxy:     &Proxy{Port: 443, Secret: "aabbccddeeff00112233445566778899"},
			nodeIP:    "vpn.example.com",
			wantShare: "https://t.me/proxy?server=vpn.example.com&port=443&secret=aabbccddeeff00112233445566778899",
			wantTg:    "tg://proxy?server=vpn.example.com&port=443&secret=aabbccddeeff00112233445566778899",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shareLink, tgLink := svc.GenerateLinks(tt.proxy, tt.nodeIP)

			if shareLink != tt.wantShare {
				t.Errorf("shareLink = %q, want %q", shareLink, tt.wantShare)
			}
			if tgLink != tt.wantTg {
				t.Errorf("tgLink = %q, want %q", tgLink, tt.wantTg)
			}
		})
	}
}

func TestGenerateSecret(t *testing.T) {
	t.Run("returns 32-char hex string", func(t *testing.T) {
		secret, err := generateSecret()
		if err != nil {
			t.Fatalf("generateSecret() error = %v", err)
		}
		if len(secret) != 32 {
			t.Errorf("secret length = %d, want 32", len(secret))
		}
		// Verify it's valid hex
		for _, c := range secret {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("secret contains non-hex char: %c", c)
				break
			}
		}
	})

	t.Run("uniqueness between calls", func(t *testing.T) {
		secrets := make(map[string]bool)
		for i := 0; i < 100; i++ {
			s, err := generateSecret()
			if err != nil {
				t.Fatalf("generateSecret() iteration %d error = %v", i, err)
			}
			if secrets[s] {
				t.Fatalf("duplicate secret generated on iteration %d: %s", i, s)
			}
			secrets[s] = true
		}
	})
}

func TestRotateSecret(t *testing.T) {
	tests := []struct {
		name    string
		proxyID int64
		rowsAff int64
		wantErr bool
	}{
		{
			name:    "successful rotation",
			proxyID: 1,
			rowsAff: 1,
			wantErr: false,
		},
		{
			name:    "proxy not found",
			proxyID: 999,
			rowsAff: 0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to open sqlmock: %v", err)
			}
			defer db.Close()

			svc := New(db)

			mock.ExpectExec(`UPDATE telegram_proxies SET secret = \$1, share_link = NULL, tg_link = NULL WHERE id = \$2`).
				WithArgs(sqlmock.AnyArg(), tt.proxyID).
				WillReturnResult(sqlmock.NewResult(0, tt.rowsAff))

			newSecret, err := svc.RotateSecret(context.Background(), tt.proxyID)

			if tt.wantErr {
				if err == nil {
					t.Errorf("RotateSecret() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("RotateSecret() unexpected error: %v", err)
			}

			// Verify new secret is valid 32-char hex
			if len(newSecret) != 32 {
				t.Errorf("new secret length = %d, want 32", len(newSecret))
			}
			for _, c := range newSecret {
				if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
					t.Errorf("new secret contains non-hex char: %c", c)
					break
				}
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestCheckHealth(t *testing.T) {
	tests := []struct {
		name    string
		proxies []struct {
			id   int64
			ip   string
			port int
		}
	}{
		{
			name: "queries proxies and updates status on failure",
			proxies: []struct {
				id   int64
				ip   string
				port int
			}{
				// Use 127.0.0.1 with unlikely ports so dial fails fast
				{id: 1, ip: "127.0.0.1", port: 19999},
				{id: 2, ip: "127.0.0.1", port: 19998},
			},
		},
		{
			name: "no proxies found",
			proxies: []struct {
				id   int64
				ip   string
				port int
			}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to open sqlmock: %v", err)
			}
			defer db.Close()

			rows := sqlmock.NewRows([]string{"id", "public_ip", "port"})
			for _, p := range tt.proxies {
				rows.AddRow(p.id, p.ip, p.port)
			}

			mock.ExpectQuery("SELECT tp.id, n.public_ip, tp.port FROM telegram_proxies").
				WillReturnRows(rows)

			// Each proxy will attempt TCP dial to 127.0.0.1 on unlikely ports.
			// Connection should be refused fast, triggering error status update.
			for _, p := range tt.proxies {
				mock.ExpectExec("UPDATE telegram_proxies SET status").
					WithArgs(sqlmock.AnyArg(), p.id).
					WillReturnResult(sqlmock.NewResult(0, 1))
			}

			CheckHealth(db)

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}
