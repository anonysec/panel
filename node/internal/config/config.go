package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
)

// Config holds the node agent configuration with thread-safe access.
type Config struct {
	mu         sync.RWMutex
	PanelURL   string
	NodeToken  string
	Interval   int // seconds
	AutoUpdate bool
	LogLevel   string
}

// reloadableKeys defines which configuration keys can be changed without restart.
var reloadableKeys = map[string]bool{
	"PANEL_URL":        true,
	"NODE_INTERVAL":    true,
	"NODE_AUTO_UPDATE": true,
	"LOG_LEVEL":        true,
}

// Load reads the given env file and returns an initial Config.
// The file format is KEY=VALUE lines; comments (#) and empty lines are ignored.
func Load(envFile string) (*Config, error) {
	values, err := parseEnvFile(envFile)
	if err != nil {
		return nil, fmt.Errorf("config load: %w", err)
	}

	token := values["NODE_TOKEN"]
	if token == "" {
		return nil, errors.New("config load: NODE_TOKEN is required")
	}

	interval := 10
	if v, ok := values["NODE_INTERVAL"]; ok && v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			interval = parsed
		}
	}

	autoUpdate := true
	if v, ok := values["NODE_AUTO_UPDATE"]; ok {
		autoUpdate = parseBool(v, true)
	}

	logLevel := "info"
	if v, ok := values["LOG_LEVEL"]; ok && v != "" {
		logLevel = strings.ToLower(v)
	}

	panelURL := strings.TrimRight(values["PANEL_URL"], "/")

	return &Config{
		PanelURL:   panelURL,
		NodeToken:  token,
		Interval:   interval,
		AutoUpdate: autoUpdate,
		LogLevel:   logLevel,
	}, nil
}

// Reload re-reads the env file, validates it, and applies reloadable key changes.
// Returns a map of changes (key -> [old, new]) or an error if validation fails.
// On validation error, the current configuration is retained.
func (c *Config) Reload(envFile string) (changes map[string][2]string, err error) {
	values, err := parseEnvFile(envFile)
	if err != nil {
		return nil, fmt.Errorf("config reload: %w", err)
	}

	// Validate required keys
	if values["NODE_TOKEN"] == "" {
		return nil, errors.New("config reload: NODE_TOKEN must exist in config file")
	}

	// Read current values under read lock
	c.mu.RLock()
	oldPanelURL := c.PanelURL
	oldInterval := c.Interval
	oldAutoUpdate := c.AutoUpdate
	oldLogLevel := c.LogLevel
	c.mu.RUnlock()

	// Parse new values for reloadable keys
	newPanelURL := strings.TrimRight(values["PANEL_URL"], "/")

	newInterval := oldInterval
	if v, ok := values["NODE_INTERVAL"]; ok && v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			newInterval = parsed
		}
	}

	newAutoUpdate := oldAutoUpdate
	if v, ok := values["NODE_AUTO_UPDATE"]; ok {
		newAutoUpdate = parseBool(v, oldAutoUpdate)
	}

	newLogLevel := oldLogLevel
	if v, ok := values["LOG_LEVEL"]; ok && v != "" {
		newLogLevel = strings.ToLower(v)
	}

	// Compare and record changes
	changes = make(map[string][2]string)

	if newPanelURL != oldPanelURL {
		changes["PANEL_URL"] = [2]string{oldPanelURL, newPanelURL}
	}
	if newInterval != oldInterval {
		changes["NODE_INTERVAL"] = [2]string{strconv.Itoa(oldInterval), strconv.Itoa(newInterval)}
	}
	if newAutoUpdate != oldAutoUpdate {
		changes["NODE_AUTO_UPDATE"] = [2]string{strconv.FormatBool(oldAutoUpdate), strconv.FormatBool(newAutoUpdate)}
	}
	if newLogLevel != oldLogLevel {
		changes["LOG_LEVEL"] = [2]string{oldLogLevel, newLogLevel}
	}

	// Apply new values under write lock
	c.mu.Lock()
	c.PanelURL = newPanelURL
	c.Interval = newInterval
	c.AutoUpdate = newAutoUpdate
	c.LogLevel = newLogLevel
	c.mu.Unlock()

	return changes, nil
}

// IsReloadable returns true if the given key supports hot-reload.
func IsReloadable(key string) bool {
	return reloadableKeys[key]
}

// GetPanelURL returns the current panel URL (thread-safe).
func (c *Config) GetPanelURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.PanelURL
}

// GetNodeToken returns the node token (thread-safe).
func (c *Config) GetNodeToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.NodeToken
}

// GetInterval returns the current push interval in seconds (thread-safe).
func (c *Config) GetInterval() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Interval
}

// GetAutoUpdate returns whether auto-update is enabled (thread-safe).
func (c *Config) GetAutoUpdate() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.AutoUpdate
}

// GetLogLevel returns the current log level string (thread-safe).
func (c *Config) GetLogLevel() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.LogLevel
}

// parseEnvFile reads a KEY=VALUE file, ignoring comments and empty lines.
func parseEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split on first '='
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}

		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])

		// Strip surrounding quotes if present
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		values[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return values, nil
}

// parseBool interprets common boolean string values.
func parseBool(s string, defaultVal bool) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	default:
		return defaultVal
	}
}
