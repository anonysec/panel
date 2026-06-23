//go:build !lite

package api

import (
	"fmt"
	"strings"
)

// validateAntiDPIConfig validates Anti-DPI configuration based on the technique type.
// It returns a descriptive error if validation fails.
func validateAntiDPIConfig(technique string, configJSON map[string]any) error {
	switch technique {
	case "reality":
		return validateReality(configJSON)
	case "fragment":
		return validateFragment(configJSON)
	case "domain_fronting":
		return validateDomainFronting(configJSON)
	case "warp":
		return validateWarp(configJSON)
	default:
		return fmt.Errorf("unsupported technique: %s", technique)
	}
}

func validateReality(cfg map[string]any) error {
	// server_name: required, non-empty string
	sn, ok := cfg["server_name"]
	if !ok {
		return fmt.Errorf("reality: server_name is required")
	}
	snStr, ok := sn.(string)
	if !ok || snStr == "" {
		return fmt.Errorf("reality: server_name is required")
	}

	// private_key: required, non-empty string
	pk, ok := cfg["private_key"]
	if !ok {
		return fmt.Errorf("reality: private_key is required")
	}
	pkStr, ok := pk.(string)
	if !ok || pkStr == "" {
		return fmt.Errorf("reality: private_key is required")
	}

	// short_ids: required, non-empty []string or string
	sid, ok := cfg["short_ids"]
	if !ok {
		return fmt.Errorf("reality: short_ids is required")
	}
	switch v := sid.(type) {
	case string:
		if v == "" {
			return fmt.Errorf("reality: short_ids is required")
		}
	case []any:
		if len(v) == 0 {
			return fmt.Errorf("reality: short_ids is required")
		}
		for i, item := range v {
			s, ok := item.(string)
			if !ok || s == "" {
				return fmt.Errorf("reality: short_ids[%d] must be a non-empty string", i)
			}
		}
	case []string:
		if len(v) == 0 {
			return fmt.Errorf("reality: short_ids is required")
		}
	default:
		return fmt.Errorf("reality: short_ids must be a string or array of strings")
	}

	return nil
}

func validateFragment(cfg map[string]any) error {
	// length: required, positive integer or string of format "min-max"
	l, ok := cfg["length"]
	if !ok {
		return fmt.Errorf("fragment: length is required")
	}
	if !isPositiveIntOrRange(l) {
		return fmt.Errorf("fragment: length must be a positive integer or range string")
	}

	// interval: required, positive integer or string of format "min-max"
	iv, ok := cfg["interval"]
	if !ok {
		return fmt.Errorf("fragment: interval is required")
	}
	if !isPositiveIntOrRange(iv) {
		return fmt.Errorf("fragment: interval must be a positive integer or range string")
	}

	return nil
}

func validateDomainFronting(cfg map[string]any) error {
	// cdn_domain: required, non-empty string
	cd, ok := cfg["cdn_domain"]
	if !ok {
		return fmt.Errorf("domain_fronting: cdn_domain is required")
	}
	cdStr, ok := cd.(string)
	if !ok || cdStr == "" {
		return fmt.Errorf("domain_fronting: cdn_domain is required")
	}

	// backend_address: required, non-empty string
	ba, ok := cfg["backend_address"]
	if !ok {
		return fmt.Errorf("domain_fronting: backend_address is required")
	}
	baStr, ok := ba.(string)
	if !ok || baStr == "" {
		return fmt.Errorf("domain_fronting: backend_address is required")
	}

	return nil
}

func validateWarp(cfg map[string]any) error {
	// endpoint: required, non-empty string
	ep, ok := cfg["endpoint"]
	if !ok {
		return fmt.Errorf("warp: endpoint is required")
	}
	epStr, ok := ep.(string)
	if !ok || epStr == "" {
		return fmt.Errorf("warp: endpoint is required")
	}

	return nil
}

// isPositiveIntOrRange checks if a value is a positive integer or a string in "min-max" format.
func isPositiveIntOrRange(v any) bool {
	switch val := v.(type) {
	case float64:
		return val > 0 && val == float64(int(val))
	case int:
		return val > 0
	case int64:
		return val > 0
	case string:
		parts := strings.SplitN(val, "-", 2)
		if len(parts) != 2 {
			return false
		}
		// Validate that both parts are non-empty (basic format check)
		return parts[0] != "" && parts[1] != ""
	default:
		return false
	}
}
