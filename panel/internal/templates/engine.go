package templates

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

// TemplateVars holds all configuration fields for VPN service templates.
type TemplateVars struct {
	Port        int
	Protocol    string
	Network     string
	NetworkIPv6 string
	ServerNet   string // Derived: network portion of CIDR
	ServerMask  string // Derived: mask portion for OpenVPN
	DNS1        string
	DNS2        string
	DNS1v6      string
	DNS2v6      string
	IPSecPSK    string
	ServerIP    string
	ExtraJSON   map[string]any
}

// TemplateEngine renders VPN service configs from Go templates.
type TemplateEngine struct {
	templates map[string]*template.Template
	basePath  string
}

// timeNowUnix returns current unix timestamp (extracted for testing).
var timeNowUnix = func() int64 {
	return time.Now().Unix()
}

// NewEngine creates a template engine loading templates from basePath.
// If basePath is empty or templates don't exist, uses embedded defaults.
func NewEngine(basePath string) *TemplateEngine {
	e := &TemplateEngine{
		templates: make(map[string]*template.Template),
		basePath:  basePath,
	}
	// Load templates from filesystem or use embedded defaults
	protocols := []string{"openvpn", "strongswan", "xl2tpd", "wireguard"}
	for _, proto := range protocols {
		tmplPath := filepath.Join(basePath, proto+".conf.tmpl")
		if basePath != "" {
			if content, err := os.ReadFile(tmplPath); err == nil {
				if t, err := template.New(proto).Parse(string(content)); err == nil {
					e.templates[proto] = t
					continue
				}
			}
		}
		// Fall back to embedded default
		if defaultTmpl, ok := defaultTemplates[proto]; ok {
			if t, err := template.New(proto).Parse(defaultTmpl); err == nil {
				e.templates[proto] = t
			}
		}
	}
	return e
}

// Render executes the template for the given protocol with the provided vars.
func (e *TemplateEngine) Render(protocol string, vars TemplateVars) (string, error) {
	tmpl, ok := e.templates[protocol]
	if !ok {
		return "", fmt.Errorf("no template for protocol: %s", protocol)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("template execution failed: %w", err)
	}
	return buf.String(), nil
}

// Validate performs protocol-specific syntax checking on rendered config.
func (e *TemplateEngine) Validate(protocol, rendered string) error {
	if strings.TrimSpace(rendered) == "" {
		return fmt.Errorf("rendered config is empty")
	}
	switch protocol {
	case "openvpn":
		// Check for required directives
		required := []string{"port", "proto", "dev"}
		for _, req := range required {
			if !strings.Contains(rendered, req+" ") && !strings.Contains(rendered, req+"\t") && !strings.Contains(rendered, req+"\n") {
				return fmt.Errorf("missing required directive: %s", req)
			}
		}
	case "wireguard":
		if !strings.Contains(rendered, "[Interface]") {
			return fmt.Errorf("missing [Interface] section")
		}
	}
	// Check for unresolved template directives
	if strings.Contains(rendered, "{{") || strings.Contains(rendered, "}}") {
		return fmt.Errorf("unresolved template directives in output")
	}
	return nil
}

// Diff returns a simple line-by-line diff between current and proposed config.
func (e *TemplateEngine) Diff(current, proposed string) string {
	currentLines := strings.Split(current, "\n")
	proposedLines := strings.Split(proposed, "\n")
	var diff strings.Builder
	maxLen := len(currentLines)
	if len(proposedLines) > maxLen {
		maxLen = len(proposedLines)
	}
	for i := 0; i < maxLen; i++ {
		var cl, pl string
		if i < len(currentLines) {
			cl = currentLines[i]
		}
		if i < len(proposedLines) {
			pl = proposedLines[i]
		}
		if cl != pl {
			if cl != "" {
				diff.WriteString(fmt.Sprintf("- %s\n", cl))
			}
			if pl != "" {
				diff.WriteString(fmt.Sprintf("+ %s\n", pl))
			}
		}
	}
	return diff.String()
}

// Apply validates, renders, backs up, and writes a config file.
func (e *TemplateEngine) Apply(protocol, confPath string, vars TemplateVars) error {
	// 1. Validate network
	if vars.Network != "" {
		if err := ValidatePrivateNetwork(vars.Network, protocol == "wireguard"); err != nil {
			return fmt.Errorf("network validation: %w", err)
		}
	}
	if vars.NetworkIPv6 != "" {
		if err := ValidatePrivateNetwork(vars.NetworkIPv6, true); err != nil {
			return fmt.Errorf("ipv6 network validation: %w", err)
		}
	}

	// 2. Render template
	rendered, err := e.Render(protocol, vars)
	if err != nil {
		return err
	}

	// 3. Validate rendered output
	if err := e.Validate(protocol, rendered); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// 4. Backup current config
	if current, readErr := os.ReadFile(confPath); readErr == nil && len(current) > 0 {
		backup := fmt.Sprintf("%s.bak.%d", confPath, timeNowUnix())
		if writeErr := os.WriteFile(backup, current, 0600); writeErr != nil {
			return fmt.Errorf("backup failed: %w", writeErr)
		}
	}

	// 5. Write new config
	if err := os.WriteFile(confPath, []byte(rendered), 0644); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}

	return nil
}
