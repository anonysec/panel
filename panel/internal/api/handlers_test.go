//go:build !lite

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// ═══════════════════════════════════════════════════════════════════════════════
// 1. Payment Gateway Handlers
// ═══════════════════════════════════════════════════════════════════════════════

func TestHandleGatewayList(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		body      string
		setupMock func(sqlmock.Sqlmock)
		wantCode  int
		wantOK    *bool
		wantError string
	}{
		{
			name:   "GET returns empty list",
			method: http.MethodGet,
			setupMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "display_name", "config_json", "is_active", "created_at"})
				mock.ExpectQuery("SELECT id, name, display_name").WillReturnRows(rows)
			},
			wantCode: http.StatusOK,
			wantOK:   boolPtr(true),
		},
		{
			name:   "POST valid create",
			method: http.MethodPost,
			body:   `{"name":"stripe","display_name":"Stripe","config_json":{"key":"sk_test"}}`,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("INSERT INTO payment_gateways").
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			wantCode: http.StatusOK,
			wantOK:   boolPtr(true),
		},
		{
			name:      "POST invalid JSON",
			method:    http.MethodPost,
			body:      `{invalid json`,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusBadRequest,
			wantOK:    boolPtr(false),
			wantError: "bad_json",
		},
		{
			name:      "POST missing name",
			method:    http.MethodPost,
			body:      `{"display_name":"Test"}`,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusBadRequest,
			wantOK:    boolPtr(false),
			wantError: "name_required",
		},
		{
			name:      "PUT method not allowed",
			method:    http.MethodPut,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusMethodNotAllowed,
		},
		{
			name:      "DELETE method not allowed",
			method:    http.MethodDelete,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			defer db.Close()

			s := &Server{DB: db}
			tt.setupMock(mock)

			var body *strings.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			} else {
				body = strings.NewReader("")
			}

			req := httptest.NewRequest(tt.method, "/api/gateways", body)
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			s.handleGatewayList(rr, req)

			assertStatus(t, rr.Code, tt.wantCode)
			if tt.wantOK != nil {
				assertOK(t, rr, *tt.wantOK)
			}
			if tt.wantError != "" {
				assertErrorCode(t, rr, tt.wantError)
			}
		})
	}
}

func TestHandleGatewayByID(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		path      string
		body      string
		setupMock func(sqlmock.Sqlmock)
		wantCode  int
		wantOK    *bool
		wantError string
	}{
		{
			name:   "PATCH valid update",
			method: http.MethodPatch,
			path:   "/api/gateways/1",
			body:   `{"display_name":"Updated Gateway"}`,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("UPDATE payment_gateways").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			wantCode: http.StatusOK,
			wantOK:   boolPtr(true),
		},
		{
			name:      "PATCH invalid JSON",
			method:    http.MethodPatch,
			path:      "/api/gateways/1",
			body:      `{broken`,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusBadRequest,
			wantOK:    boolPtr(false),
			wantError: "bad_json",
		},
		{
			name:   "DELETE valid",
			method: http.MethodDelete,
			path:   "/api/gateways/1",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT name FROM payment_gateways").
					WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("stripe"))
				mock.ExpectExec("DELETE FROM payment_gateways").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			wantCode: http.StatusOK,
			wantOK:   boolPtr(true),
		},
		{
			name:      "GET method not allowed",
			method:    http.MethodGet,
			path:      "/api/gateways/1",
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusMethodNotAllowed,
		},
		{
			name:      "invalid ID returns not_found",
			method:    http.MethodPatch,
			path:      "/api/gateways/abc",
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusNotFound,
			wantOK:    boolPtr(false),
			wantError: "not_found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			defer db.Close()

			s := &Server{DB: db}
			tt.setupMock(mock)

			var body *strings.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			} else {
				body = strings.NewReader("")
			}

			req := httptest.NewRequest(tt.method, tt.path, body)
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			s.handleGatewayByID(rr, req)

			assertStatus(t, rr.Code, tt.wantCode)
			if tt.wantOK != nil {
				assertOK(t, rr, *tt.wantOK)
			}
			if tt.wantError != "" {
				assertErrorCode(t, rr, tt.wantError)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 2. MTProto Handlers
// ═══════════════════════════════════════════════════════════════════════════════

func TestHandleMTProto(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		body      string
		setupMock func(sqlmock.Sqlmock)
		wantCode  int
		wantOK    *bool
		wantError string
	}{
		{
			name:   "GET list returns empty array",
			method: http.MethodGet,
			setupMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "node_id", "port", "secret", "status", "connections",
					"rx_bytes", "tx_bytes", "created_at", "updated_at", "node_ip", "node_name",
				})
				mock.ExpectQuery("SELECT m.id, m.node_id").WillReturnRows(rows)
			},
			wantCode: http.StatusOK,
			wantOK:   boolPtr(true),
		},
		{
			name:      "POST missing node_id",
			method:    http.MethodPost,
			body:      `{"port":443}`,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusBadRequest,
			wantOK:    boolPtr(false),
			wantError: "node_id_required",
		},
		{
			name:      "POST invalid JSON",
			method:    http.MethodPost,
			body:      `not json`,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusBadRequest,
			wantOK:    boolPtr(false),
			wantError: "bad_json",
		},
		{
			name:      "PUT method not allowed",
			method:    http.MethodPut,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			defer db.Close()

			s := &Server{DB: db}
			tt.setupMock(mock)

			var body *strings.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			} else {
				body = strings.NewReader("")
			}

			req := httptest.NewRequest(tt.method, "/api/mtproto", body)
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			s.handleMTProto(rr, req)

			assertStatus(t, rr.Code, tt.wantCode)
			if tt.wantOK != nil {
				assertOK(t, rr, *tt.wantOK)
			}
			if tt.wantError != "" {
				assertErrorCode(t, rr, tt.wantError)
			}
		})
	}
}

func TestHandleMTProtoByID(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		path      string
		setupMock func(sqlmock.Sqlmock)
		wantCode  int
		wantOK    *bool
		wantError string
	}{
		{
			name:   "GET link returns valid link",
			method: http.MethodGet,
			path:   "/api/mtproto/1/link",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT COALESCE").
					WillReturnRows(sqlmock.NewRows([]string{"node_ip", "port", "secret"}).
						AddRow("1.2.3.4", 443, "abc123"))
			},
			wantCode: http.StatusOK,
			wantOK:   boolPtr(true),
		},
		{
			name:      "invalid ID returns not_found",
			method:    http.MethodGet,
			path:      "/api/mtproto/abc/link",
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusNotFound,
			wantOK:    boolPtr(false),
			wantError: "not_found",
		},
		{
			name:      "unknown action returns not_found",
			method:    http.MethodPost,
			path:      "/api/mtproto/1/unknown",
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusNotFound,
			wantOK:    boolPtr(false),
			wantError: "not_found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			defer db.Close()

			s := &Server{DB: db}
			tt.setupMock(mock)

			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			s.handleMTProtoByID(rr, req)

			assertStatus(t, rr.Code, tt.wantCode)
			if tt.wantOK != nil {
				assertOK(t, rr, *tt.wantOK)
			}
			if tt.wantError != "" {
				assertErrorCode(t, rr, tt.wantError)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 3. Xray Inbound Handlers
// ═══════════════════════════════════════════════════════════════════════════════

func TestHandleXrayInbound(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		body      string
		setupMock func(sqlmock.Sqlmock)
		wantCode  int
		wantOK    *bool
		wantError string
	}{
		{
			name:      "POST invalid protocol returns error",
			method:    http.MethodPost,
			body:      `{"customer_id":1,"node_id":1,"protocol":"invalid","transport":"tcp","security":"none","port":443}`,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusBadRequest,
			wantOK:    boolPtr(false),
			wantError: "invalid_protocol",
		},
		{
			name:      "POST invalid JSON",
			method:    http.MethodPost,
			body:      `{bad}`,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusBadRequest,
			wantOK:    boolPtr(false),
			wantError: "bad_json",
		},
		{
			name:      "POST missing customer_id",
			method:    http.MethodPost,
			body:      `{"node_id":1,"protocol":"vless","transport":"tcp","security":"none","port":443}`,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusBadRequest,
			wantOK:    boolPtr(false),
			wantError: "customer_id_required",
		},
		{
			name:      "PUT method not allowed",
			method:    http.MethodPut,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			defer db.Close()

			s := &Server{DB: db}
			tt.setupMock(mock)

			var body *strings.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			} else {
				body = strings.NewReader("")
			}

			req := httptest.NewRequest(tt.method, "/api/xray/inbounds", body)
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			s.handleXrayInbound(rr, req)

			assertStatus(t, rr.Code, tt.wantCode)
			if tt.wantOK != nil {
				assertOK(t, rr, *tt.wantOK)
			}
			if tt.wantError != "" {
				assertErrorCode(t, rr, tt.wantError)
			}
		})
	}
}

func TestHandleXrayInboundByID(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		path      string
		setupMock func(sqlmock.Sqlmock)
		wantCode  int
		wantOK    *bool
		wantError string
	}{
		{
			name:      "invalid ID returns not_found",
			method:    http.MethodGet,
			path:      "/api/xray/inbounds/abc",
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusNotFound,
			wantOK:    boolPtr(false),
			wantError: "not_found",
		},
		{
			name:      "PUT method not allowed",
			method:    http.MethodPut,
			path:      "/api/xray/inbounds/1",
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			defer db.Close()

			s := &Server{DB: db}
			tt.setupMock(mock)

			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			s.handleXrayInboundByID(rr, req)

			assertStatus(t, rr.Code, tt.wantCode)
			if tt.wantOK != nil {
				assertOK(t, rr, *tt.wantOK)
			}
			if tt.wantError != "" {
				assertErrorCode(t, rr, tt.wantError)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 4. Node Groups Handlers
// ═══════════════════════════════════════════════════════════════════════════════

func TestHandleNodeGroups(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		body      string
		setupMock func(sqlmock.Sqlmock)
		wantCode  int
		wantOK    *bool
		wantError string
	}{
		{
			name:   "GET list returns groups",
			method: http.MethodGet,
			setupMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "name", "region", "description",
					"load_balancing_enabled", "max_load_percent", "created_at", "node_count",
				}).AddRow(1, "Europe", "eu-west", "EU nodes", 1, 85, "2024-01-01T00:00:00Z", 3)
				mock.ExpectQuery("SELECT ng.id, ng.name").WillReturnRows(rows)
			},
			wantCode: http.StatusOK,
			wantOK:   boolPtr(true),
		},
		{
			name:   "POST valid create",
			method: http.MethodPost,
			body:   `{"name":"Asia","region":"ap-east"}`,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("INSERT INTO node_groups").
					WillReturnResult(sqlmock.NewResult(2, 1))
			},
			wantCode: http.StatusOK,
			wantOK:   boolPtr(true),
		},
		{
			name:      "POST missing name",
			method:    http.MethodPost,
			body:      `{"region":"eu-west"}`,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusBadRequest,
			wantOK:    boolPtr(false),
			wantError: "name_required",
		},
		{
			name:      "POST invalid JSON",
			method:    http.MethodPost,
			body:      `{bad`,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusBadRequest,
			wantOK:    boolPtr(false),
			wantError: "bad_json",
		},
		{
			name:      "PUT method not allowed",
			method:    http.MethodPut,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			defer db.Close()

			s := &Server{DB: db}
			tt.setupMock(mock)

			var body *strings.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			} else {
				body = strings.NewReader("")
			}

			req := httptest.NewRequest(tt.method, "/api/node-groups", body)
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			s.handleNodeGroups(rr, req)

			assertStatus(t, rr.Code, tt.wantCode)
			if tt.wantOK != nil {
				assertOK(t, rr, *tt.wantOK)
			}
			if tt.wantError != "" {
				assertErrorCode(t, rr, tt.wantError)
			}
		})
	}
}

func TestHandleNodeGroupByID(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		path      string
		body      string
		setupMock func(sqlmock.Sqlmock)
		wantCode  int
		wantOK    *bool
		wantError string
	}{
		{
			name:   "PATCH valid update",
			method: http.MethodPatch,
			path:   "/api/node-groups/1",
			body:   `{"name":"Updated"}`,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("UPDATE node_groups").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			wantCode: http.StatusOK,
			wantOK:   boolPtr(true),
		},
		{
			name:   "DELETE valid",
			method: http.MethodDelete,
			path:   "/api/node-groups/1",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("DELETE FROM node_groups").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			wantCode: http.StatusOK,
			wantOK:   boolPtr(true),
		},
		{
			name:   "DELETE not found",
			method: http.MethodDelete,
			path:   "/api/node-groups/999",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("DELETE FROM node_groups").
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			wantCode:  http.StatusNotFound,
			wantOK:    boolPtr(false),
			wantError: "not_found",
		},
		{
			name:      "GET method not allowed",
			method:    http.MethodGet,
			path:      "/api/node-groups/1",
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusMethodNotAllowed,
		},
		{
			name:      "invalid ID not found",
			method:    http.MethodPatch,
			path:      "/api/node-groups/abc",
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusNotFound,
			wantOK:    boolPtr(false),
			wantError: "not_found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			defer db.Close()

			s := &Server{DB: db}
			tt.setupMock(mock)

			var body *strings.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			} else {
				body = strings.NewReader("")
			}

			req := httptest.NewRequest(tt.method, tt.path, body)
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			s.handleNodeGroupByID(rr, req)

			assertStatus(t, rr.Code, tt.wantCode)
			if tt.wantOK != nil {
				assertOK(t, rr, *tt.wantOK)
			}
			if tt.wantError != "" {
				assertErrorCode(t, rr, tt.wantError)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 5. Canned Responses Handlers
// ═══════════════════════════════════════════════════════════════════════════════

func TestAdminCannedResponses(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		path      string
		body      string
		setupMock func(sqlmock.Sqlmock)
		wantCode  int
		wantOK    *bool
		wantError string
	}{
		{
			name:   "GET list returns responses",
			method: http.MethodGet,
			path:   "/api/canned-responses",
			setupMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "title", "body", "category", "usage_count", "created_at", "updated_at",
				})
				mock.ExpectQuery("SELECT id, title, body").WillReturnRows(rows)
			},
			wantCode: http.StatusOK,
			wantOK:   boolPtr(true),
		},
		{
			name:   "POST valid create",
			method: http.MethodPost,
			path:   "/api/canned-responses",
			body:   `{"title":"Greeting","body":"Hello {{customer_name}}","category":"general"}`,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("INSERT INTO canned_responses").
					WillReturnResult(sqlmock.NewResult(1, 1))
				// logAudit call
				mock.ExpectExec("INSERT INTO audit_logs").
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			wantCode: http.StatusOK,
			wantOK:   boolPtr(true),
		},
		{
			name:      "POST empty title returns error",
			method:    http.MethodPost,
			path:      "/api/canned-responses",
			body:      `{"title":"","body":"some body"}`,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusBadRequest,
			wantOK:    boolPtr(false),
			wantError: "title_and_body_required",
		},
		{
			name:      "POST invalid JSON",
			method:    http.MethodPost,
			path:      "/api/canned-responses",
			body:      `{nope`,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusBadRequest,
			wantOK:    boolPtr(false),
			wantError: "bad_json",
		},
		{
			name:      "PUT method not allowed",
			method:    http.MethodPut,
			path:      "/api/canned-responses",
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			defer db.Close()

			s := &Server{DB: db}
			tt.setupMock(mock)

			var body *strings.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			} else {
				body = strings.NewReader("")
			}

			req := httptest.NewRequest(tt.method, tt.path, body)
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			s.adminCannedResponses(rr, req)

			assertStatus(t, rr.Code, tt.wantCode)
			if tt.wantOK != nil {
				assertOK(t, rr, *tt.wantOK)
			}
			if tt.wantError != "" {
				assertErrorCode(t, rr, tt.wantError)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 6. SLA Config Handlers
// ═══════════════════════════════════════════════════════════════════════════════

func TestHandleSLAConfig(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		body      string
		setupMock func(sqlmock.Sqlmock)
		wantCode  int
		wantOK    *bool
		wantError string
	}{
		{
			name:   "GET returns config array",
			method: http.MethodGet,
			setupMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"priority", "response_minutes"}).
					AddRow("urgent", 60).
					AddRow("high", 240).
					AddRow("normal", 1440).
					AddRow("low", 4320)
				mock.ExpectQuery("SELECT priority, response_minutes FROM sla_config").WillReturnRows(rows)
			},
			wantCode: http.StatusOK,
			wantOK:   boolPtr(true),
		},
		{
			name:      "PATCH invalid priority returns error",
			method:    http.MethodPatch,
			body:      `[{"priority":"critical","response_minutes":30}]`,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusBadRequest,
			wantOK:    boolPtr(false),
			wantError: "invalid_priority",
		},
		{
			name:      "PATCH invalid JSON",
			method:    http.MethodPatch,
			body:      `not json`,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusBadRequest,
			wantOK:    boolPtr(false),
			wantError: "bad_json",
		},
		{
			name:      "PATCH empty config returns error",
			method:    http.MethodPatch,
			body:      `[]`,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusBadRequest,
			wantOK:    boolPtr(false),
			wantError: "empty_config",
		},
		{
			name:      "PATCH invalid response_minutes",
			method:    http.MethodPatch,
			body:      `[{"priority":"urgent","response_minutes":0}]`,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusBadRequest,
			wantOK:    boolPtr(false),
			wantError: "invalid_response_minutes",
		},
		{
			name:      "PUT method not allowed",
			method:    http.MethodPut,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusMethodNotAllowed,
			wantOK:    boolPtr(false),
			wantError: "method_not_allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			defer db.Close()

			s := &Server{DB: db}
			tt.setupMock(mock)

			var body *strings.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			} else {
				body = strings.NewReader("")
			}

			req := httptest.NewRequest(tt.method, "/api/sla/config", body)
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			s.handleSLAConfig(rr, req)

			assertStatus(t, rr.Code, tt.wantCode)
			if tt.wantOK != nil {
				assertOK(t, rr, *tt.wantOK)
			}
			if tt.wantError != "" {
				assertErrorCode(t, rr, tt.wantError)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 7. Knowledge Base Handlers
// ═══════════════════════════════════════════════════════════════════════════════

func TestHandleKBArticles(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		body      string
		setupMock func(sqlmock.Sqlmock)
		wantCode  int
		wantOK    *bool
		wantError string
	}{
		{
			name:   "POST valid create",
			method: http.MethodPost,
			body:   `{"title":"Getting Started","body":"# Welcome\nThis is a guide.","category":"general"}`,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("INSERT INTO kb_articles").
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			wantCode: http.StatusOK,
			wantOK:   boolPtr(true),
		},
		{
			name:      "POST empty title returns error",
			method:    http.MethodPost,
			body:      `{"title":"","body":"content"}`,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusBadRequest,
			wantOK:    boolPtr(false),
			wantError: "title_required",
		},
		{
			name:      "POST empty body returns error",
			method:    http.MethodPost,
			body:      `{"title":"Test","body":""}`,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusBadRequest,
			wantOK:    boolPtr(false),
			wantError: "body_required",
		},
		{
			name:      "POST invalid JSON",
			method:    http.MethodPost,
			body:      `{bad`,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusBadRequest,
			wantOK:    boolPtr(false),
			wantError: "bad_json",
		},
		{
			name:      "POST invalid status",
			method:    http.MethodPost,
			body:      `{"title":"T","body":"B","status":"archived"}`,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusBadRequest,
			wantOK:    boolPtr(false),
			wantError: "invalid_status",
		},
		{
			name:      "PUT method not allowed",
			method:    http.MethodPut,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			defer db.Close()

			s := &Server{DB: db}
			tt.setupMock(mock)

			var body *strings.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			} else {
				body = strings.NewReader("")
			}

			req := httptest.NewRequest(tt.method, "/api/kb/articles", body)
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			s.handleKBArticles(rr, req)

			assertStatus(t, rr.Code, tt.wantCode)
			if tt.wantOK != nil {
				assertOK(t, rr, *tt.wantOK)
			}
			if tt.wantError != "" {
				assertErrorCode(t, rr, tt.wantError)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 8. User Tags Handlers
// ═══════════════════════════════════════════════════════════════════════════════

func TestHandleTags(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		body      string
		setupMock func(sqlmock.Sqlmock)
		wantCode  int
		wantOK    *bool
		wantError string
	}{
		{
			name:   "GET list tags",
			method: http.MethodGet,
			setupMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "color", "created_at"}).
					AddRow(1, "VIP", "#ff0000", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
				mock.ExpectQuery("SELECT").WillReturnRows(rows)
			},
			wantCode: http.StatusOK,
			wantOK:   boolPtr(true),
		},
		{
			name:   "POST valid create",
			method: http.MethodPost,
			body:   `{"name":"Premium","color":"#00ff00"}`,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("INSERT INTO user_tags").
					WillReturnResult(sqlmock.NewResult(2, 1))
			},
			wantCode: http.StatusOK,
			wantOK:   boolPtr(true),
		},
		{
			name:      "POST empty name returns error",
			method:    http.MethodPost,
			body:      `{"name":"","color":"#ff0000"}`,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusBadRequest,
			wantOK:    boolPtr(false),
			wantError: "name_required",
		},
		{
			name:      "POST invalid JSON",
			method:    http.MethodPost,
			body:      `{bad`,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusBadRequest,
			wantOK:    boolPtr(false),
			wantError: "bad_json",
		},
		{
			name:      "PATCH method not allowed",
			method:    http.MethodPatch,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			defer db.Close()

			s := &Server{DB: db}
			tt.setupMock(mock)

			var body *strings.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			} else {
				body = strings.NewReader("")
			}

			req := httptest.NewRequest(tt.method, "/api/tags", body)
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			s.handleTags(rr, req)

			assertStatus(t, rr.Code, tt.wantCode)
			if tt.wantOK != nil {
				assertOK(t, rr, *tt.wantOK)
			}
			if tt.wantError != "" {
				assertErrorCode(t, rr, tt.wantError)
			}
		})
	}
}

func TestHandleTagByID(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		path      string
		setupMock func(sqlmock.Sqlmock)
		wantCode  int
		wantOK    *bool
		wantError string
	}{
		{
			name:   "DELETE valid tag",
			method: http.MethodDelete,
			path:   "/api/tags/1",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("DELETE FROM user_tags").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			wantCode: http.StatusOK,
			wantOK:   boolPtr(true),
		},
		{
			name:   "DELETE non-existent returns 404",
			method: http.MethodDelete,
			path:   "/api/tags/999",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("DELETE FROM user_tags").
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			wantCode:  http.StatusNotFound,
			wantOK:    boolPtr(false),
			wantError: "not_found",
		},
		{
			name:      "invalid ID returns not_found",
			method:    http.MethodDelete,
			path:      "/api/tags/abc",
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusNotFound,
			wantOK:    boolPtr(false),
			wantError: "not_found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			defer db.Close()

			s := &Server{DB: db}
			tt.setupMock(mock)

			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			s.handleTagByID(rr, req)

			assertStatus(t, rr.Code, tt.wantCode)
			if tt.wantOK != nil {
				assertOK(t, rr, *tt.wantOK)
			}
			if tt.wantError != "" {
				assertErrorCode(t, rr, tt.wantError)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 9. Anti-DPI Handlers
// ═══════════════════════════════════════════════════════════════════════════════

func TestHandleNodeAntiDPI(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		body      string
		technique string
		setupMock func(sqlmock.Sqlmock)
		wantCode  int
		wantOK    *bool
		wantError string
	}{
		{
			name:   "GET returns empty configs",
			method: http.MethodGet,
			setupMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "technique", "config_json", "is_active", "created_at", "updated_at",
				})
				mock.ExpectQuery("SELECT id, technique, config_json").WillReturnRows(rows)
			},
			wantCode: http.StatusOK,
			wantOK:   boolPtr(true),
		},
		{
			name:      "POST invalid technique returns validation error",
			method:    http.MethodPost,
			body:      `{"technique":"unknown_technique","config_json":{}}`,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusBadRequest,
			wantOK:    boolPtr(false),
		},
		{
			name:      "POST missing technique",
			method:    http.MethodPost,
			body:      `{"config_json":{}}`,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusBadRequest,
			wantOK:    boolPtr(false),
			wantError: "technique_required",
		},
		{
			name:      "POST invalid JSON",
			method:    http.MethodPost,
			body:      `{bad`,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusBadRequest,
			wantOK:    boolPtr(false),
			wantError: "bad_json",
		},
		{
			name:      "DELETE without technique returns error",
			method:    http.MethodDelete,
			technique: "",
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusBadRequest,
			wantOK:    boolPtr(false),
			wantError: "technique_required",
		},
		{
			name:      "DELETE non-existent technique returns not_found",
			method:    http.MethodDelete,
			technique: "warp",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("DELETE FROM node_antidpi").
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			wantCode:  http.StatusNotFound,
			wantOK:    boolPtr(false),
			wantError: "not_found",
		},
		{
			name:      "PATCH method not allowed",
			method:    http.MethodPatch,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			defer db.Close()

			s := &Server{DB: db}
			tt.setupMock(mock)

			var body *strings.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			} else {
				body = strings.NewReader("")
			}

			req := httptest.NewRequest(tt.method, "/api/nodes/1/antidpi", body)
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			s.handleNodeAntiDPI(rr, req, 1, tt.technique)

			assertStatus(t, rr.Code, tt.wantCode)
			if tt.wantOK != nil {
				assertOK(t, rr, *tt.wantOK)
			}
			if tt.wantError != "" {
				assertErrorCode(t, rr, tt.wantError)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 10. Invoices Handlers
// ═══════════════════════════════════════════════════════════════════════════════

func TestHandleInvoices(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		setupMock func(sqlmock.Sqlmock)
		wantCode  int
		wantOK    *bool
		wantError string
	}{
		{
			name:   "GET list with no invoices returns empty array",
			method: http.MethodGet,
			setupMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "invoice_number", "customer_id", "amount", "tax",
					"total", "currency", "plan_name", "payment_method",
					"status", "refunded_amount", "created_at",
				})
				mock.ExpectQuery("SELECT").WillReturnRows(rows)
			},
			wantCode: http.StatusOK,
			wantOK:   boolPtr(true),
		},
		{
			name:      "POST method not allowed",
			method:    http.MethodPost,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusMethodNotAllowed,
		},
		{
			name:      "PUT method not allowed",
			method:    http.MethodPut,
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			defer db.Close()

			s := &Server{DB: db}
			tt.setupMock(mock)

			req := httptest.NewRequest(tt.method, "/api/invoices", nil)
			rr := httptest.NewRecorder()
			s.handleInvoices(rr, req)

			assertStatus(t, rr.Code, tt.wantCode)
			if tt.wantOK != nil {
				assertOK(t, rr, *tt.wantOK)
			}
			if tt.wantError != "" {
				assertErrorCode(t, rr, tt.wantError)
			}
		})
	}
}

func TestHandleInvoiceByID(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		path      string
		setupMock func(sqlmock.Sqlmock)
		wantCode  int
		wantOK    *bool
		wantError string
	}{
		{
			name:      "invalid ID returns not_found",
			method:    http.MethodGet,
			path:      "/api/invoices/abc",
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusNotFound,
			wantOK:    boolPtr(false),
			wantError: "not_found",
		},
		{
			name:      "PUT method not allowed",
			method:    http.MethodPut,
			path:      "/api/invoices/1",
			setupMock: func(mock sqlmock.Sqlmock) {},
			wantCode:  http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			defer db.Close()

			s := &Server{DB: db}
			tt.setupMock(mock)

			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			s.handleInvoiceByID(rr, req)

			assertStatus(t, rr.Code, tt.wantCode)
			if tt.wantOK != nil {
				assertOK(t, rr, *tt.wantOK)
			}
			if tt.wantError != "" {
				assertErrorCode(t, rr, tt.wantError)
			}
		})
	}
}
