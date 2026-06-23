//go:build !lite

package api

import (
	"strings"
	"testing"
)

func TestValidateAntiDPIConfig(t *testing.T) {
	tests := []struct {
		name      string
		technique string
		config    map[string]any
		wantErr   string
	}{
		// --- reality ---
		{
			name:      "reality valid",
			technique: "reality",
			config: map[string]any{
				"server_name": "google.com",
				"private_key": "abc123key",
				"short_ids":   []any{"deadbeef"},
			},
			wantErr: "",
		},
		{
			name:      "reality valid with string short_ids",
			technique: "reality",
			config: map[string]any{
				"server_name": "google.com",
				"private_key": "abc123key",
				"short_ids":   "deadbeef",
			},
			wantErr: "",
		},
		{
			name:      "reality missing server_name",
			technique: "reality",
			config: map[string]any{
				"private_key": "abc123key",
				"short_ids":   []any{"deadbeef"},
			},
			wantErr: "reality: server_name is required",
		},
		{
			name:      "reality empty server_name",
			technique: "reality",
			config: map[string]any{
				"server_name": "",
				"private_key": "abc123key",
				"short_ids":   []any{"deadbeef"},
			},
			wantErr: "reality: server_name is required",
		},
		{
			name:      "reality missing private_key",
			technique: "reality",
			config: map[string]any{
				"server_name": "google.com",
				"short_ids":   []any{"deadbeef"},
			},
			wantErr: "reality: private_key is required",
		},
		{
			name:      "reality missing short_ids",
			technique: "reality",
			config: map[string]any{
				"server_name": "google.com",
				"private_key": "abc123key",
			},
			wantErr: "reality: short_ids is required",
		},
		{
			name:      "reality empty short_ids array",
			technique: "reality",
			config: map[string]any{
				"server_name": "google.com",
				"private_key": "abc123key",
				"short_ids":   []any{},
			},
			wantErr: "reality: short_ids is required",
		},
		{
			name:      "reality empty short_ids string",
			technique: "reality",
			config: map[string]any{
				"server_name": "google.com",
				"private_key": "abc123key",
				"short_ids":   "",
			},
			wantErr: "reality: short_ids is required",
		},

		// --- fragment ---
		{
			name:      "fragment valid with integers",
			technique: "fragment",
			config: map[string]any{
				"length":   float64(100),
				"interval": float64(50),
			},
			wantErr: "",
		},
		{
			name:      "fragment valid with range strings",
			technique: "fragment",
			config: map[string]any{
				"length":   "10-100",
				"interval": "5-50",
			},
			wantErr: "",
		},
		{
			name:      "fragment missing length",
			technique: "fragment",
			config: map[string]any{
				"interval": float64(50),
			},
			wantErr: "fragment: length is required",
		},
		{
			name:      "fragment missing interval",
			technique: "fragment",
			config: map[string]any{
				"length": float64(100),
			},
			wantErr: "fragment: interval is required",
		},
		{
			name:      "fragment zero length",
			technique: "fragment",
			config: map[string]any{
				"length":   float64(0),
				"interval": float64(50),
			},
			wantErr: "fragment: length must be a positive integer or range string",
		},
		{
			name:      "fragment negative interval",
			technique: "fragment",
			config: map[string]any{
				"length":   float64(100),
				"interval": float64(-5),
			},
			wantErr: "fragment: interval must be a positive integer or range string",
		},

		// --- domain_fronting ---
		{
			name:      "domain_fronting valid",
			technique: "domain_fronting",
			config: map[string]any{
				"cdn_domain":      "cdn.example.com",
				"backend_address": "10.0.0.1:443",
			},
			wantErr: "",
		},
		{
			name:      "domain_fronting missing cdn_domain",
			technique: "domain_fronting",
			config: map[string]any{
				"backend_address": "10.0.0.1:443",
			},
			wantErr: "domain_fronting: cdn_domain is required",
		},
		{
			name:      "domain_fronting empty cdn_domain",
			technique: "domain_fronting",
			config: map[string]any{
				"cdn_domain":      "",
				"backend_address": "10.0.0.1:443",
			},
			wantErr: "domain_fronting: cdn_domain is required",
		},
		{
			name:      "domain_fronting missing backend_address",
			technique: "domain_fronting",
			config: map[string]any{
				"cdn_domain": "cdn.example.com",
			},
			wantErr: "domain_fronting: backend_address is required",
		},

		// --- warp ---
		{
			name:      "warp valid",
			technique: "warp",
			config: map[string]any{
				"endpoint": "engage.cloudflareclient.com:2408",
			},
			wantErr: "",
		},
		{
			name:      "warp with optional fields",
			technique: "warp",
			config: map[string]any{
				"endpoint":        "engage.cloudflareclient.com:2408",
				"private_key":     "somekey",
				"peer_public_key": "peerkey",
				"reserved":        []any{float64(1), float64(2), float64(3)},
			},
			wantErr: "",
		},
		{
			name:      "warp missing endpoint",
			technique: "warp",
			config:    map[string]any{},
			wantErr:   "warp: endpoint is required",
		},
		{
			name:      "warp empty endpoint",
			technique: "warp",
			config: map[string]any{
				"endpoint": "",
			},
			wantErr: "warp: endpoint is required",
		},

		// --- unsupported ---
		{
			name:      "unsupported technique",
			technique: "shadowtls",
			config:    map[string]any{},
			wantErr:   "unsupported technique: shadowtls",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAntiDPIConfig(tt.technique, tt.config)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
				}
			}
		})
	}
}

func TestIsPositiveIntOrRange(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want bool
	}{
		{"positive float64", float64(10), true},
		{"zero float64", float64(0), false},
		{"negative float64", float64(-1), false},
		{"non-integer float64", float64(1.5), false},
		{"positive int", 5, true},
		{"zero int", 0, false},
		{"valid range", "10-100", true},
		{"invalid range no dash", "100", false},
		{"empty string", "", false},
		{"invalid type bool", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPositiveIntOrRange(tt.val)
			if got != tt.want {
				t.Errorf("isPositiveIntOrRange(%v) = %v, want %v", tt.val, got, tt.want)
			}
		})
	}
}
