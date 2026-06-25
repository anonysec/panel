package logger

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNew_JSONFormat(t *testing.T) {
	l := New("json")
	if l.Format() != "json" {
		t.Errorf("expected format 'json', got %q", l.Format())
	}
}

func TestNew_TextFormat(t *testing.T) {
	l := New("text")
	if l.Format() != "text" {
		t.Errorf("expected format 'text', got %q", l.Format())
	}
}

func TestNew_EmptyDefaultsToText(t *testing.T) {
	l := New("")
	if l.Format() != "text" {
		t.Errorf("expected format 'text', got %q", l.Format())
	}
}

func TestInfo_JSON(t *testing.T) {
	var buf bytes.Buffer
	l := New("json")
	l.SetOutput(&buf)

	l.Info("database", "connected to PostgreSQL")

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse JSON log: %v\nraw: %s", err, buf.String())
	}

	if entry.Level != "info" {
		t.Errorf("expected level 'info', got %q", entry.Level)
	}
	if entry.Component != "database" {
		t.Errorf("expected component 'database', got %q", entry.Component)
	}
	if entry.Message != "connected to PostgreSQL" {
		t.Errorf("expected message 'connected to PostgreSQL', got %q", entry.Message)
	}
	if entry.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestWarn_JSON(t *testing.T) {
	var buf bytes.Buffer
	l := New("json")
	l.SetOutput(&buf)

	l.Warn("grpc-client", "connection lost", map[string]any{"node": "node-1"})

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse JSON log: %v", err)
	}

	if entry.Level != "warn" {
		t.Errorf("expected level 'warn', got %q", entry.Level)
	}
	if entry.Component != "grpc-client" {
		t.Errorf("expected component 'grpc-client', got %q", entry.Component)
	}
	if entry.Fields["node"] != "node-1" {
		t.Errorf("expected field node='node-1', got %v", entry.Fields["node"])
	}
}

func TestError_JSON(t *testing.T) {
	var buf bytes.Buffer
	l := New("json")
	l.SetOutput(&buf)

	l.Error("http-server", "bind failed", map[string]any{"port": 443})

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse JSON log: %v", err)
	}

	if entry.Level != "error" {
		t.Errorf("expected level 'error', got %q", entry.Level)
	}
	if entry.Component != "http-server" {
		t.Errorf("expected component 'http-server', got %q", entry.Component)
	}
	// JSON numbers are float64 after unmarshal
	if entry.Fields["port"] != float64(443) {
		t.Errorf("expected field port=443, got %v", entry.Fields["port"])
	}
}

func TestInfo_Text(t *testing.T) {
	var buf bytes.Buffer
	l := New("text")
	l.SetOutput(&buf)

	l.Info("database", "migrations complete")

	line := strings.TrimSpace(buf.String())

	// Should contain [info] [database] migrations complete
	if !strings.Contains(line, "[info]") {
		t.Errorf("expected [info] in text output, got: %s", line)
	}
	if !strings.Contains(line, "[database]") {
		t.Errorf("expected [database] in text output, got: %s", line)
	}
	if !strings.Contains(line, "migrations complete") {
		t.Errorf("expected message in text output, got: %s", line)
	}
	// Should start with a timestamp (ISO8601/RFC3339)
	if !strings.Contains(line[:20], "T") {
		t.Errorf("expected RFC3339 timestamp at start, got: %s", line)
	}
}

func TestInfo_TextWithFields(t *testing.T) {
	var buf bytes.Buffer
	l := New("text")
	l.SetOutput(&buf)

	l.Info("worker", "started", map[string]any{"id": "w-1"})

	line := strings.TrimSpace(buf.String())

	if !strings.Contains(line, "[worker]") {
		t.Errorf("expected [worker] in text output, got: %s", line)
	}
	if !strings.Contains(line, "id=w-1") {
		t.Errorf("expected field 'id=w-1' in text output, got: %s", line)
	}
}

func TestReady_JSON(t *testing.T) {
	var buf bytes.Buffer
	l := New("json")
	l.SetOutput(&buf)

	l.Ready("0.96.0", "full", ":443", 3, 2)

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse JSON log: %v\nraw: %s", err, buf.String())
	}

	if entry.Level != "info" {
		t.Errorf("expected level 'info', got %q", entry.Level)
	}
	if entry.Component != "ready" {
		t.Errorf("expected component 'ready', got %q", entry.Component)
	}
	if entry.Fields["version"] != "0.96.0" {
		t.Errorf("expected version '0.96.0', got %v", entry.Fields["version"])
	}
	if entry.Fields["edition"] != "full" {
		t.Errorf("expected edition 'full', got %v", entry.Fields["edition"])
	}
	if entry.Fields["addr"] != ":443" {
		t.Errorf("expected addr ':443', got %v", entry.Fields["addr"])
	}
	if entry.Fields["node_count"] != float64(3) {
		t.Errorf("expected node_count 3, got %v", entry.Fields["node_count"])
	}
	if entry.Fields["worker_count"] != float64(2) {
		t.Errorf("expected worker_count 2, got %v", entry.Fields["worker_count"])
	}
}

func TestReady_Text(t *testing.T) {
	var buf bytes.Buffer
	l := New("text")
	l.SetOutput(&buf)

	l.Ready("0.96.0", "full", ":443", 3, 2)

	line := strings.TrimSpace(buf.String())

	if !strings.Contains(line, "[ready]") {
		t.Errorf("expected [ready] in text output, got: %s", line)
	}
	if !strings.Contains(line, "KorisPanel") {
		t.Errorf("expected 'KorisPanel' in text output, got: %s", line)
	}
	if !strings.Contains(line, "0.96.0") {
		t.Errorf("expected version in text output, got: %s", line)
	}
}

func TestComponentTags(t *testing.T) {
	// Verify all expected component tags are emittable
	components := []string{
		ComponentDatabase,
		ComponentGRPCClient,
		ComponentNodeChecker,
		ComponentTelegramBot,
		ComponentWorker,
		ComponentHTTPServer,
	}

	var buf bytes.Buffer
	l := New("json")
	l.SetOutput(&buf)

	for _, comp := range components {
		buf.Reset()
		l.Info(comp, "initializing")

		var entry LogEntry
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			t.Fatalf("failed to parse JSON for component %q: %v", comp, err)
		}
		if entry.Component != comp {
			t.Errorf("expected component %q, got %q", comp, entry.Component)
		}
	}
}

func TestJSON_NoFields_OmitsFieldsKey(t *testing.T) {
	var buf bytes.Buffer
	l := New("json")
	l.SetOutput(&buf)

	l.Info("database", "ping ok")

	raw := strings.TrimSpace(buf.String())
	if strings.Contains(raw, `"fields"`) {
		t.Errorf("expected 'fields' to be omitted when nil, got: %s", raw)
	}
}

func TestJSON_ValidJSON(t *testing.T) {
	var buf bytes.Buffer
	l := New("json")
	l.SetOutput(&buf)

	l.Info("grpc-client", "connecting", map[string]any{
		"addr":    "10.0.0.1",
		"port":    62050,
		"timeout": "10s",
	})

	raw := strings.TrimSpace(buf.String())
	if !json.Valid([]byte(raw)) {
		t.Errorf("output is not valid JSON: %s", raw)
	}
}
