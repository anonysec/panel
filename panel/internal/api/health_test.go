package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"KorisPanel/panel/internal/config"
)

func TestHealth_ReturnsExpectedFields(t *testing.T) {
	s := &Server{
		Config: config.Config{
			Version:  "1.2.3",
			WorkerID: "test-host-9999",
		},
		StartedAt: time.Now().Add(-10 * time.Second),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()

	s.health(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Check required fields
	if resp["ok"] != true {
		t.Errorf("expected ok=true, got %v", resp["ok"])
	}
	if resp["service"] != "panel" {
		t.Errorf("expected service=panel, got %v", resp["service"])
	}
	if resp["version"] != "1.2.3" {
		t.Errorf("expected version=1.2.3, got %v", resp["version"])
	}
	if resp["worker_id"] != "test-host-9999" {
		t.Errorf("expected worker_id=test-host-9999, got %v", resp["worker_id"])
	}

	uptime, ok := resp["uptime_seconds"].(float64)
	if !ok {
		t.Fatalf("uptime_seconds missing or not a number: %v", resp["uptime_seconds"])
	}
	if uptime < 10 {
		t.Errorf("expected uptime >= 10, got %v", uptime)
	}

	if _, exists := resp["time"]; !exists {
		t.Error("expected time field to be present")
	}
}

func TestHealth_UptimeIncreasesOverTime(t *testing.T) {
	s := &Server{
		Config: config.Config{
			Version:  "1.0.0",
			WorkerID: "w-1",
		},
		StartedAt: time.Now().Add(-60 * time.Second),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()

	s.health(w, req)

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	uptime := resp["uptime_seconds"].(float64)
	if uptime < 60 {
		t.Errorf("expected uptime >= 60 for a server started 60s ago, got %v", uptime)
	}
}
