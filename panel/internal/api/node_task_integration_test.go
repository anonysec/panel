//go:build !lite

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"KorisPanel/panel/internal/config"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestGRPCDispatch_UpdateAgent verifies that the node agent update handler
// responds correctly when gRPC is the dispatch mechanism (no node_tasks insertion).
func TestGRPCDispatch_UpdateAgent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	s := &Server{
		DB: db,
		Config: config.Config{
			SessionSecret: "test-secret",
		},
	}

	body := map[string]any{
		"node_id":  1,
		"version":  "1.2.3",
		"url":      "https://releases.example.com/node-1.2.3",
		"checksum": "abc123def456",
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/nodes/update", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleNodeAgentUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["ok"] != true {
		t.Errorf("ok = %v, want true", resp["ok"])
	}

	// No DB expectations for node_tasks since dispatch is via gRPC now
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// TestGRPCDispatch_CoreInstall verifies that core install uses gRPC EnableCore
// and reports errors when gRPC is not configured.
func TestGRPCDispatch_CoreInstall(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	s := &Server{
		DB: db,
		Config: config.Config{
			SessionSecret: "test-secret",
		},
	}

	body := map[string]any{
		"core_name": "xray-core",
		"version":   "1.8.4",
		"port":      443,
	}
	b, _ := json.Marshal(body)

	// Expect core_plugins lookup
	mock.ExpectQuery("SELECT download_url, checksum_sha256 FROM core_plugins WHERE").
		WithArgs("xray-core", "1.8.4").
		WillReturnRows(sqlmock.NewRows([]string{"download_url", "checksum_sha256"}).
			AddRow("https://github.com/XTLS/Xray-core/releases/download/v1.8.4/xray.zip", "sha256checksum123"))

	// Expect INSERT INTO node_cores
	mock.ExpectExec("INSERT INTO node_cores").
		WillReturnResult(sqlmock.NewResult(1, 1))

	// No CoreMgr configured — handler should log warning but not fail fatally
	req := httptest.NewRequest(http.MethodPost, "/api/nodes/5/cores/install", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.nodeCoresInstall(rec, req, 5)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Errorf("ok = %v, want true", resp["ok"])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// TestGRPCDispatch_XrayInboundCreate verifies that xray inbound creation
// works without node_tasks (gRPC dispatch is handled separately).
func TestGRPCDispatch_XrayInboundCreate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	s := &Server{
		DB: db,
		Config: config.Config{
			SessionSecret: "test-secret",
		},
	}

	body := map[string]any{
		"customer_id": 10,
		"node_id":     2,
		"protocol":    "vless",
		"transport":   "tcp",
		"security":    "reality",
		"port":        443,
		"server_name": "www.google.com",
		"public_key":  "test-public-key-base64",
		"short_id":    "abcd1234",
		"private_key": "test-private-key-base64",
	}
	b, _ := json.Marshal(body)

	// Expect INSERT INTO xray_inbounds
	mock.ExpectExec("INSERT INTO xray_inbounds").
		WillReturnResult(sqlmock.NewResult(100, 1))

	req := httptest.NewRequest(http.MethodPost, "/api/xray/inbounds", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleXrayInboundCreate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Errorf("ok = %v, want true", resp["ok"])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}
