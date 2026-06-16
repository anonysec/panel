package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNoCacheMiddleware_APIPaths(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := noCacheMiddleware(inner)

	tests := []struct {
		name     string
		path     string
		wantHdr  string
	}{
		{
			name:    "api path gets no-store",
			path:    "/api/health",
			wantHdr: "no-store",
		},
		{
			name:    "api subpath gets no-store",
			path:    "/api/customers/123",
			wantHdr: "no-store",
		},
		{
			name:    "non-api path has no cache-control",
			path:    "/dashboard/",
			wantHdr: "",
		},
		{
			name:    "portal path has no cache-control",
			path:    "/portal/",
			wantHdr: "",
		},
		{
			name:    "root path has no cache-control",
			path:    "/",
			wantHdr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			got := rec.Header().Get("Cache-Control")
			if got != tt.wantHdr {
				t.Errorf("Cache-Control = %q, want %q", got, tt.wantHdr)
			}
		})
	}
}
