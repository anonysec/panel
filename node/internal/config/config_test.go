package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func writeTempEnv(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "node.env")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp env file: %v", err)
	}
	return path
}

func TestLoad_ValidConfig(t *testing.T) {
	envContent := `
# Node configuration
PANEL_URL=https://panel.example.com/
NODE_TOKEN=secret123
NODE_INTERVAL=30
NODE_AUTO_UPDATE=true
LOG_LEVEL=debug
`
	path := writeTempEnv(t, envContent)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.GetPanelURL() != "https://panel.example.com" {
		t.Errorf("PanelURL = %q, want %q", cfg.GetPanelURL(), "https://panel.example.com")
	}
	if cfg.GetNodeToken() != "secret123" {
		t.Errorf("NodeToken = %q, want %q", cfg.GetNodeToken(), "secret123")
	}
	if cfg.GetInterval() != 30 {
		t.Errorf("Interval = %d, want %d", cfg.GetInterval(), 30)
	}
	if cfg.GetAutoUpdate() != true {
		t.Errorf("AutoUpdate = %v, want %v", cfg.GetAutoUpdate(), true)
	}
	if cfg.GetLogLevel() != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.GetLogLevel(), "debug")
	}
}

func TestLoad_Defaults(t *testing.T) {
	envContent := `NODE_TOKEN=tok123`
	path := writeTempEnv(t, envContent)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.GetInterval() != 10 {
		t.Errorf("Interval default = %d, want %d", cfg.GetInterval(), 10)
	}
	if cfg.GetAutoUpdate() != true {
		t.Errorf("AutoUpdate default = %v, want %v", cfg.GetAutoUpdate(), true)
	}
	if cfg.GetLogLevel() != "info" {
		t.Errorf("LogLevel default = %q, want %q", cfg.GetLogLevel(), "info")
	}
}

func TestLoad_MissingNodeToken(t *testing.T) {
	envContent := `PANEL_URL=http://localhost:8080`
	path := writeTempEnv(t, envContent)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing NODE_TOKEN, got nil")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/node.env")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestLoad_QuotedValues(t *testing.T) {
	envContent := `
NODE_TOKEN="my-secret-token"
PANEL_URL='http://panel.local'
`
	path := writeTempEnv(t, envContent)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.GetNodeToken() != "my-secret-token" {
		t.Errorf("NodeToken = %q, want %q", cfg.GetNodeToken(), "my-secret-token")
	}
	if cfg.GetPanelURL() != "http://panel.local" {
		t.Errorf("PanelURL = %q, want %q", cfg.GetPanelURL(), "http://panel.local")
	}
}

func TestLoad_CommentsAndEmptyLines(t *testing.T) {
	envContent := `
# This is a comment
NODE_TOKEN=abc

# Another comment
PANEL_URL=http://example.com

`
	path := writeTempEnv(t, envContent)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.GetNodeToken() != "abc" {
		t.Errorf("NodeToken = %q, want %q", cfg.GetNodeToken(), "abc")
	}
	if cfg.GetPanelURL() != "http://example.com" {
		t.Errorf("PanelURL = %q, want %q", cfg.GetPanelURL(), "http://example.com")
	}
}

func TestReload_AppliesChanges(t *testing.T) {
	// Initial config
	initialContent := `
NODE_TOKEN=tok123
PANEL_URL=http://old.panel.com
NODE_INTERVAL=10
NODE_AUTO_UPDATE=true
LOG_LEVEL=info
`
	path := writeTempEnv(t, initialContent)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Write updated config
	updatedContent := `
NODE_TOKEN=tok123
PANEL_URL=http://new.panel.com
NODE_INTERVAL=30
NODE_AUTO_UPDATE=false
LOG_LEVEL=debug
`
	if err := os.WriteFile(path, []byte(updatedContent), 0644); err != nil {
		t.Fatalf("failed to write updated env: %v", err)
	}

	changes, err := cfg.Reload(path)
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	// Verify changes map
	if len(changes) != 4 {
		t.Errorf("expected 4 changes, got %d: %v", len(changes), changes)
	}

	if c, ok := changes["PANEL_URL"]; !ok || c[0] != "http://old.panel.com" || c[1] != "http://new.panel.com" {
		t.Errorf("PANEL_URL change = %v, want [http://old.panel.com, http://new.panel.com]", c)
	}
	if c, ok := changes["NODE_INTERVAL"]; !ok || c[0] != "10" || c[1] != "30" {
		t.Errorf("NODE_INTERVAL change = %v, want [10, 30]", c)
	}
	if c, ok := changes["NODE_AUTO_UPDATE"]; !ok || c[0] != "true" || c[1] != "false" {
		t.Errorf("NODE_AUTO_UPDATE change = %v, want [true, false]", c)
	}
	if c, ok := changes["LOG_LEVEL"]; !ok || c[0] != "info" || c[1] != "debug" {
		t.Errorf("LOG_LEVEL change = %v, want [info, debug]", c)
	}

	// Verify values applied
	if cfg.GetPanelURL() != "http://new.panel.com" {
		t.Errorf("PanelURL after reload = %q, want %q", cfg.GetPanelURL(), "http://new.panel.com")
	}
	if cfg.GetInterval() != 30 {
		t.Errorf("Interval after reload = %d, want %d", cfg.GetInterval(), 30)
	}
	if cfg.GetAutoUpdate() != false {
		t.Errorf("AutoUpdate after reload = %v, want %v", cfg.GetAutoUpdate(), false)
	}
	if cfg.GetLogLevel() != "debug" {
		t.Errorf("LogLevel after reload = %q, want %q", cfg.GetLogLevel(), "debug")
	}
}

func TestReload_NoChanges(t *testing.T) {
	content := `
NODE_TOKEN=tok123
PANEL_URL=http://panel.com
NODE_INTERVAL=10
NODE_AUTO_UPDATE=true
LOG_LEVEL=info
`
	path := writeTempEnv(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	changes, err := cfg.Reload(path)
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	if len(changes) != 0 {
		t.Errorf("expected 0 changes, got %d: %v", len(changes), changes)
	}
}

func TestReload_MissingNodeToken_RetainsConfig(t *testing.T) {
	initialContent := `
NODE_TOKEN=tok123
PANEL_URL=http://panel.com
NODE_INTERVAL=10
`
	path := writeTempEnv(t, initialContent)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Write config without NODE_TOKEN
	badContent := `
PANEL_URL=http://evil.com
NODE_INTERVAL=99
`
	if err := os.WriteFile(path, []byte(badContent), 0644); err != nil {
		t.Fatalf("failed to write bad env: %v", err)
	}

	_, err = cfg.Reload(path)
	if err == nil {
		t.Fatal("expected error for missing NODE_TOKEN, got nil")
	}

	// Original values should be retained
	if cfg.GetPanelURL() != "http://panel.com" {
		t.Errorf("PanelURL should be retained, got %q", cfg.GetPanelURL())
	}
	if cfg.GetInterval() != 10 {
		t.Errorf("Interval should be retained, got %d", cfg.GetInterval())
	}
}

func TestReload_FileNotFound_RetainsConfig(t *testing.T) {
	initialContent := `
NODE_TOKEN=tok123
PANEL_URL=http://panel.com
`
	path := writeTempEnv(t, initialContent)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	_, err = cfg.Reload("/nonexistent/path/node.env")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}

	// Original values should be retained
	if cfg.GetPanelURL() != "http://panel.com" {
		t.Errorf("PanelURL should be retained, got %q", cfg.GetPanelURL())
	}
}

func TestReload_NodeTokenNotReloadable(t *testing.T) {
	initialContent := `
NODE_TOKEN=original-token
PANEL_URL=http://panel.com
`
	path := writeTempEnv(t, initialContent)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Even if NODE_TOKEN changes in the file, it should not be updated
	updatedContent := `
NODE_TOKEN=new-token
PANEL_URL=http://panel.com
`
	if err := os.WriteFile(path, []byte(updatedContent), 0644); err != nil {
		t.Fatalf("failed to write updated env: %v", err)
	}

	_, err = cfg.Reload(path)
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	// NODE_TOKEN should remain unchanged
	if cfg.GetNodeToken() != "original-token" {
		t.Errorf("NodeToken should not change on reload, got %q", cfg.GetNodeToken())
	}
}

func TestIsReloadable(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"PANEL_URL", true},
		{"NODE_INTERVAL", true},
		{"NODE_AUTO_UPDATE", true},
		{"LOG_LEVEL", true},
		{"NODE_TOKEN", false},
		{"UNKNOWN_KEY", false},
	}

	for _, tt := range tests {
		if got := IsReloadable(tt.key); got != tt.want {
			t.Errorf("IsReloadable(%q) = %v, want %v", tt.key, got, tt.want)
		}
	}
}

func TestConcurrentAccess(t *testing.T) {
	envContent := `
NODE_TOKEN=tok123
PANEL_URL=http://panel.com
NODE_INTERVAL=10
NODE_AUTO_UPDATE=true
LOG_LEVEL=info
`
	path := writeTempEnv(t, envContent)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Updated content for reloads
	updatedContent := `
NODE_TOKEN=tok123
PANEL_URL=http://new.panel.com
NODE_INTERVAL=20
NODE_AUTO_UPDATE=false
LOG_LEVEL=debug
`
	updatedPath := writeTempEnv(t, updatedContent)

	var wg sync.WaitGroup
	// Multiple concurrent readers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cfg.GetPanelURL()
			_ = cfg.GetNodeToken()
			_ = cfg.GetInterval()
			_ = cfg.GetAutoUpdate()
			_ = cfg.GetLogLevel()
		}()
	}

	// Concurrent writer
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = cfg.Reload(updatedPath)
		}()
	}

	wg.Wait()
}

func TestLoad_InvalidInterval(t *testing.T) {
	envContent := `
NODE_TOKEN=tok123
NODE_INTERVAL=notanumber
`
	path := writeTempEnv(t, envContent)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should fall back to default (10)
	if cfg.GetInterval() != 10 {
		t.Errorf("Interval = %d, want default 10 for invalid input", cfg.GetInterval())
	}
}

func TestLoad_AutoUpdateFalse(t *testing.T) {
	envContent := `
NODE_TOKEN=tok123
NODE_AUTO_UPDATE=false
`
	path := writeTempEnv(t, envContent)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.GetAutoUpdate() != false {
		t.Errorf("AutoUpdate = %v, want false", cfg.GetAutoUpdate())
	}
}
