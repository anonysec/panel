package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestNodeBulk_MethodNotAllowed(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/nodes/bulk", nil)
	rec := httptest.NewRecorder()
	s.nodeBulk(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestNodeBulk_InvalidJSON(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/nodes/bulk", strings.NewReader("{bad"))
	rec := httptest.NewRecorder()
	s.nodeBulk(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestNodeBulk_InvalidAction(t *testing.T) {
	s := &Server{}
	body := `{"action":"destroy_all","node_ids":[1]}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/nodes/bulk", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.nodeBulk(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestNodeBulk_EmptyNodeIDs(t *testing.T) {
	s := &Server{}
	body := `{"action":"restart_openvpn","node_ids":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/nodes/bulk", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.nodeBulk(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestNodeBulk_TooManyNodes(t *testing.T) {
	s := &Server{}
	ids := make([]int64, 51)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	idsJSON, _ := json.Marshal(ids)
	body := `{"action":"restart_openvpn","node_ids":` + string(idsJSON) + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/nodes/bulk", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.nodeBulk(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestNodeBulk_EnableProtocolMissingParam(t *testing.T) {
	s := &Server{}
	body := `{"action":"enable_protocol","node_ids":[1],"params":{}}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/nodes/bulk", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.nodeBulk(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestNodeBulk_RunCommandMissingParam(t *testing.T) {
	s := &Server{}
	body := `{"action":"run_command","node_ids":[1],"params":{}}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/nodes/bulk", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.nodeBulk(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestNodeBulk_MaintenanceOn(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	s := &Server{DB: db}
	mock.ExpectQuery("SELECT 1 FROM nodes WHERE id").WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))
	mock.ExpectExec("UPDATE nodes SET maintenance_mode").WithArgs(true, int64(3)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO audit_logs").WillReturnResult(sqlmock.NewResult(1, 1))
	body := `{"action":"maintenance_on","node_ids":[3]}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/nodes/bulk", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.nodeBulk(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Errorf("ok = %v, want true", resp["ok"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

func TestNodeBulk_EnableProtocolNoGRPC(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	s := &Server{DB: db}
	mock.ExpectQuery("SELECT 1 FROM nodes WHERE id").WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))
	mock.ExpectExec("INSERT INTO audit_logs").WillReturnResult(sqlmock.NewResult(1, 1))
	body := `{"action":"enable_protocol","node_ids":[1],"params":{"protocol":"wireguard"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/nodes/bulk", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.nodeBulk(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Errorf("ok = %v, want true", resp["ok"])
	}
	results := resp["results"].([]any)
	r1 := results[0].(map[string]any)
	if r1["success"] != false {
		t.Errorf("result[0].success = %v, want false", r1["success"])
	}
	if r1["error"] != "grpc not configured" {
		t.Errorf("result[0].error = %v, want 'grpc not configured'", r1["error"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}
