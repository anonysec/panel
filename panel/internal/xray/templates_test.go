//go:build !lite

package xray

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestListTemplates(t *testing.T) {
	t.Run("returns all templates", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)
		now := time.Now()

		rows := sqlmock.NewRows([]string{"id", "name", "description", "config_json", "category", "created_at", "updated_at"}).
			AddRow(1, "VLESS + Reality", "Basic VLESS config", `{"inbounds":[]}`, "protocol", now, now).
			AddRow(2, "VMess WebSocket", "Simple VMess-WS", `{"inbounds":[]}`, "protocol", now, now)

		mock.ExpectQuery("SELECT id, name, COALESCE\\(description,''\\), config_json, COALESCE\\(category,'general'\\)").
			WillReturnRows(rows)

		templates, err := svc.ListTemplates(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(templates) != 2 {
			t.Fatalf("expected 2 templates, got %d", len(templates))
		}
		if templates[0].Name != "VLESS + Reality" {
			t.Errorf("expected first template name 'VLESS + Reality', got %q", templates[0].Name)
		}
		if templates[1].Name != "VMess WebSocket" {
			t.Errorf("expected second template name 'VMess WebSocket', got %q", templates[1].Name)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("returns empty slice when no templates", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)
		rows := sqlmock.NewRows([]string{"id", "name", "description", "config_json", "category", "created_at", "updated_at"})

		mock.ExpectQuery("SELECT id, name, COALESCE\\(description,''\\), config_json, COALESCE\\(category,'general'\\)").
			WillReturnRows(rows)

		templates, err := svc.ListTemplates(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if templates == nil {
			t.Fatal("expected non-nil empty slice")
		}
		if len(templates) != 0 {
			t.Fatalf("expected 0 templates, got %d", len(templates))
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})
}

func TestGetTemplate(t *testing.T) {
	t.Run("returns template by ID", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)
		now := time.Now()

		configJSON := `{"inbounds":[{"protocol":"vless","port":443}]}`
		rows := sqlmock.NewRows([]string{"id", "name", "description", "config_json", "category", "created_at", "updated_at"}).
			AddRow(1, "VLESS + Reality", "Basic VLESS config", configJSON, "protocol", now, now)

		mock.ExpectQuery("SELECT id, name, COALESCE\\(description,''\\), config_json, COALESCE\\(category,'general'\\)").
			WithArgs(int64(1)).
			WillReturnRows(rows)

		tmpl, err := svc.GetTemplate(context.Background(), 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tmpl.ID != 1 {
			t.Errorf("expected ID 1, got %d", tmpl.ID)
		}
		if tmpl.Name != "VLESS + Reality" {
			t.Errorf("expected name 'VLESS + Reality', got %q", tmpl.Name)
		}
		if tmpl.Category != "protocol" {
			t.Errorf("expected category 'protocol', got %q", tmpl.Category)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("returns error for non-existent template", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)
		rows := sqlmock.NewRows([]string{"id", "name", "description", "config_json", "category", "created_at", "updated_at"})

		mock.ExpectQuery("SELECT id, name, COALESCE\\(description,''\\), config_json, COALESCE\\(category,'general'\\)").
			WithArgs(int64(999)).
			WillReturnRows(rows)

		_, err = svc.GetTemplate(context.Background(), 999)
		if err == nil {
			t.Fatal("expected error for non-existent template")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})
}

func TestCreateTemplate(t *testing.T) {
	t.Run("creates template successfully", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		mock.ExpectExec("INSERT INTO xray_templates").
			WithArgs("Test Template", "A test", `{"inbounds":[]}`, "general").
			WillReturnResult(sqlmock.NewResult(5, 1))

		tmpl := &XrayTemplate{
			Name:        "Test Template",
			Description: "A test",
			ConfigJSON:  json.RawMessage(`{"inbounds":[]}`),
			Category:    "general",
		}

		id, err := svc.CreateTemplate(context.Background(), tmpl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != 5 {
			t.Errorf("expected id 5, got %d", id)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("error on empty name", func(t *testing.T) {
		db, _, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		tmpl := &XrayTemplate{
			Name:       "",
			ConfigJSON: json.RawMessage(`{"inbounds":[]}`),
		}

		_, err = svc.CreateTemplate(context.Background(), tmpl)
		if err == nil {
			t.Fatal("expected error for empty name")
		}
	})

	t.Run("error on empty config_json", func(t *testing.T) {
		db, _, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		tmpl := &XrayTemplate{
			Name:       "Test",
			ConfigJSON: json.RawMessage{},
		}

		_, err = svc.CreateTemplate(context.Background(), tmpl)
		if err == nil {
			t.Fatal("expected error for empty config_json")
		}
	})

	t.Run("error on invalid JSON", func(t *testing.T) {
		db, _, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		tmpl := &XrayTemplate{
			Name:       "Test",
			ConfigJSON: json.RawMessage(`{invalid json`),
		}

		_, err = svc.CreateTemplate(context.Background(), tmpl)
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})

	t.Run("defaults category to general", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		mock.ExpectExec("INSERT INTO xray_templates").
			WithArgs("Test", "", `{"inbounds":[]}`, "general").
			WillReturnResult(sqlmock.NewResult(1, 1))

		tmpl := &XrayTemplate{
			Name:       "Test",
			ConfigJSON: json.RawMessage(`{"inbounds":[]}`),
			Category:   "", // empty — should default
		}

		_, err = svc.CreateTemplate(context.Background(), tmpl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})
}

func TestUpdateTemplate(t *testing.T) {
	t.Run("updates template successfully", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		mock.ExpectExec("UPDATE xray_templates").
			WithArgs("Updated Name", "New description", `{"inbounds":[{"protocol":"vmess"}]}`, "protocol", int64(1)).
			WillReturnResult(sqlmock.NewResult(0, 1))

		tmpl := &XrayTemplate{
			Name:        "Updated Name",
			Description: "New description",
			ConfigJSON:  json.RawMessage(`{"inbounds":[{"protocol":"vmess"}]}`),
			Category:    "protocol",
		}

		err = svc.UpdateTemplate(context.Background(), 1, tmpl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("error when template not found", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		mock.ExpectExec("UPDATE xray_templates").
			WithArgs("Name", "", `{"inbounds":[]}`, "general", int64(999)).
			WillReturnResult(sqlmock.NewResult(0, 0))

		tmpl := &XrayTemplate{
			Name:       "Name",
			ConfigJSON: json.RawMessage(`{"inbounds":[]}`),
		}

		err = svc.UpdateTemplate(context.Background(), 999, tmpl)
		if err == nil {
			t.Fatal("expected error for non-existent template")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("error on empty name", func(t *testing.T) {
		db, _, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		tmpl := &XrayTemplate{
			Name:       "",
			ConfigJSON: json.RawMessage(`{"inbounds":[]}`),
		}

		err = svc.UpdateTemplate(context.Background(), 1, tmpl)
		if err == nil {
			t.Fatal("expected error for empty name")
		}
	})
}

func TestDeleteTemplate(t *testing.T) {
	t.Run("deletes template successfully", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		mock.ExpectExec("DELETE FROM xray_templates WHERE id").
			WithArgs(int64(1)).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err = svc.DeleteTemplate(context.Background(), 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("error when template not found", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		mock.ExpectExec("DELETE FROM xray_templates WHERE id").
			WithArgs(int64(999)).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = svc.DeleteTemplate(context.Background(), 999)
		if err == nil {
			t.Fatal("expected error for non-existent template")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})
}

func TestApplyTemplate(t *testing.T) {
	t.Run("applies template to node", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)
		now := time.Now()

		configJSON := `{"inbounds":[{"protocol":"vless","port":443,"transport":"tcp","tag":"vless-main"}],"routing":{"domain_strategy":"AsIs","rules":[]},"tls":{}}`

		// GetTemplate query.
		mock.ExpectQuery("SELECT id, name, COALESCE\\(description,''\\), config_json, COALESCE\\(category,'general'\\)").
			WithArgs(int64(1)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "config_json", "category", "created_at", "updated_at"}).
				AddRow(1, "VLESS + Reality", "Basic VLESS", configJSON, "protocol", now, now))

		// SaveConfig upsert.
		mock.ExpectExec("INSERT INTO xray_configs").
			WithArgs(
				int64(10),        // node_id
				true,             // enabled
				sqlmock.AnyArg(), // config_json
				sqlmock.AnyArg(), // reality_config_json (nil here)
			).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err = svc.ApplyTemplate(context.Background(), 10, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("applies template with reality config", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)
		now := time.Now()

		configJSON := `{"inbounds":[{"protocol":"vless","port":443,"transport":"tcp","tag":"vless-reality"}],"routing":{"domain_strategy":"AsIs","rules":[]},"tls":{},"reality":{"server_names":["www.google.com"],"private_key":"test-key","public_key":"test-pub","short_ids":["abcdef12"]}}`

		mock.ExpectQuery("SELECT id, name, COALESCE\\(description,''\\), config_json, COALESCE\\(category,'general'\\)").
			WithArgs(int64(2)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "config_json", "category", "created_at", "updated_at"}).
				AddRow(2, "VLESS + Reality", "With Reality", configJSON, "protocol", now, now))

		mock.ExpectExec("INSERT INTO xray_configs").
			WithArgs(
				int64(5),         // node_id
				true,             // enabled
				sqlmock.AnyArg(), // config_json
				sqlmock.AnyArg(), // reality_config_json (should be non-nil)
			).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err = svc.ApplyTemplate(context.Background(), 5, 2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("error when template not found", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		mock.ExpectQuery("SELECT id, name, COALESCE\\(description,''\\), config_json, COALESCE\\(category,'general'\\)").
			WithArgs(int64(999)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "config_json", "category", "created_at", "updated_at"}))

		err = svc.ApplyTemplate(context.Background(), 10, 999)
		if err == nil {
			t.Fatal("expected error for non-existent template")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})
}
