package api

import (
	"net/http"
	"testing"

	"KorisPanel/panel/internal/config"
)

func TestCheckWSOrigin(t *testing.T) {
	srv := &Server{
		Config: config.Config{
			PublicBase:     "https://panel.example.com/dashboard",
			AllowedOrigins: []string{"https://extra.example.org", "custom.local:8080"},
		},
	}

	tests := []struct {
		name   string
		origin string
		host   string
		want   bool
	}{
		{
			name:   "empty origin allowed (same-origin behavior)",
			origin: "",
			host:   "panel.example.com",
			want:   true,
		},
		{
			name:   "origin matching PublicBase host allowed",
			origin: "https://panel.example.com",
			host:   "other.example.com",
			want:   true,
		},
		{
			name:   "origin matching AllowedOrigins entry allowed",
			origin: "https://extra.example.org",
			host:   "other.example.com",
			want:   true,
		},
		{
			name:   "origin matching AllowedOrigins direct host allowed",
			origin: "http://custom.local:8080",
			host:   "other.example.com",
			want:   true,
		},
		{
			name:   "origin matching request Host allowed (same-origin)",
			origin: "https://myhost.example.com",
			host:   "myhost.example.com",
			want:   true,
		},
		{
			name:   "unknown origin rejected",
			origin: "https://evil.attacker.com",
			host:   "panel.example.com",
			want:   false,
		},
		{
			name:   "invalid origin URL rejected",
			origin: "://not-a-url",
			host:   "panel.example.com",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := http.NewRequest("GET", "/ws", nil)
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			r.Host = tt.host

			got := srv.checkWSOrigin(r)
			if got != tt.want {
				t.Errorf("checkWSOrigin() = %v, want %v", got, tt.want)
			}
		})
	}
}
