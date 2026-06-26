package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsLoopback(t *testing.T) {
	tests := []struct {
		addr     string
		expected bool
	}{
		// IPv4 loopback
		{"127.0.0.1:8080", true},
		{"127.0.0.1:443", true},
		{"127.0.0.1", true},
		{"127.0.1.1:9000", true},

		// IPv6 loopback
		{"[::1]:8080", true},
		{"::1", true},

		// Localhost
		{"localhost:8080", true},

		// Non-loopback
		{"192.168.1.100:8080", false},
		{"10.0.0.1:443", false},
		{"0.0.0.0:8080", false},
		{"8.8.8.8:80", false},
		{"[2001:db8::1]:443", false},

		// Empty / all interfaces
		{":8080", false},
	}

	for _, tc := range tests {
		t.Run(tc.addr, func(t *testing.T) {
			result := isLoopback(tc.addr)
			if result != tc.expected {
				t.Errorf("isLoopback(%q) = %v, want %v", tc.addr, result, tc.expected)
			}
		})
	}
}

func TestRedirectToHTTPS_NonLoopback(t *testing.T) {
	// Create a simple handler that should NOT be reached for non-loopback requests
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("served"))
	})

	middleware := redirectToHTTPS(inner)

	tests := []struct {
		name       string
		remoteAddr string
		host       string
		path       string
		wantCode   int
		wantLoc    string
	}{
		{
			name:       "external IP gets redirected",
			remoteAddr: "192.168.1.100:12345",
			host:       "example.com",
			path:       "/dashboard/",
			wantCode:   http.StatusMovedPermanently,
			wantLoc:    "https://example.com/dashboard/",
		},
		{
			name:       "external IP with port in host gets clean redirect",
			remoteAddr: "10.0.0.5:54321",
			host:       "example.com:8080",
			path:       "/api/status",
			wantCode:   http.StatusMovedPermanently,
			wantLoc:    "https://example.com/api/status",
		},
		{
			name:       "external IP with query params",
			remoteAddr: "8.8.8.8:12345",
			host:       "panel.example.com",
			path:       "/api/admin/nodes?page=2",
			wantCode:   http.StatusMovedPermanently,
			wantLoc:    "https://panel.example.com/api/admin/nodes?page=2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://"+tc.host+tc.path, nil)
			req.RemoteAddr = tc.remoteAddr
			// Ensure TLS is nil (plain HTTP)
			req.TLS = nil

			rec := httptest.NewRecorder()
			middleware.ServeHTTP(rec, req)

			if rec.Code != tc.wantCode {
				t.Errorf("got status %d, want %d", rec.Code, tc.wantCode)
			}
			loc := rec.Header().Get("Location")
			if loc != tc.wantLoc {
				t.Errorf("got Location %q, want %q", loc, tc.wantLoc)
			}
		})
	}
}

func TestRedirectToHTTPS_LoopbackServedNormally(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("served"))
	})

	middleware := redirectToHTTPS(inner)

	tests := []struct {
		name       string
		remoteAddr string
	}{
		{"IPv4 loopback", "127.0.0.1:12345"},
		{"IPv6 loopback", "[::1]:12345"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://localhost:8080/dashboard/", nil)
			req.RemoteAddr = tc.remoteAddr
			req.TLS = nil

			rec := httptest.NewRecorder()
			middleware.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("got status %d, want %d (loopback should be served normally)", rec.Code, http.StatusOK)
			}
			if rec.Body.String() != "served" {
				t.Errorf("got body %q, want %q", rec.Body.String(), "served")
			}
		})
	}
}

func TestRedirectToHTTPS_TLSRequestPassesThrough(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("tls-served"))
	})

	middleware := redirectToHTTPS(inner)

	// Simulate a TLS request from external IP — should pass through
	req := httptest.NewRequest("GET", "https://example.com/dashboard/", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	// httptest.NewRequest with https:// sets TLS state automatically

	rec := httptest.NewRecorder()
	middleware.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want %d (TLS request should pass through)", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "tls-served" {
		t.Errorf("got body %q, want %q", rec.Body.String(), "tls-served")
	}
}
