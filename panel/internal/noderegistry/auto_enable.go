package noderegistry

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
)

// CoreEnabler abstracts the gRPC core management operations needed by AutoEnableCores
// and ReenableStoppedCores. The grpcclient.CoreManager satisfies this interface.
type CoreEnabler interface {
	EnableCore(ctx context.Context, nodeID int64, coreType string, listenPort int, extraConfig json.RawMessage) error
	AllCoreStatuses(ctx context.Context, nodeID int64) ([]CoreStatusInfo, error)
}

// CoreStatusInfo contains the state info returned by AllCoreStatuses.
// This mirrors grpcclient.CoreStatus but avoids a circular import.
type CoreStatusInfo struct {
	Type  string
	State string
}

// CoreEnableResult holds the outcome of enabling a single VPN core.
type CoreEnableResult struct {
	Core    string // protocol type: "openvpn", "wireguard", "l2tp", "ikev2", "ssh"
	Success bool
	Error   string
}

// coreDefault describes a core's default configuration for auto-enablement.
type coreDefault struct {
	Core  string
	Port  int
	Extra json.RawMessage
}

// defaultCores returns the 5 VPN protocol defaults for auto-enablement.
// Requirement 1.4: OpenVPN 1194/UDP, WireGuard 51820/UDP, L2TP 1701/UDP, IKEv2 500/UDP, SSH 2222/TCP.
func defaultCores(domain string) []coreDefault {
	return []coreDefault{
		{"openvpn", 1194, json.RawMessage(`{"auth_mode":"userpass","cipher":"AES-256-GCM","protocol":"udp"}`)},
		{"wireguard", 51820, json.RawMessage(`{"subnet":"10.8.0.0/24","dns":"8.8.8.8,1.1.1.1"}`)},
		{"l2tp", 1701, json.RawMessage(`{"auth_type":"psk","ip_pool":"10.9.0.0/24"}`)},
		{"ikev2", 500, buildIKEv2Extra(domain)},
		{"ssh", 2222, nil},
	}
}

// buildIKEv2Extra constructs the IKEv2 Extra_Config JSON, injecting the domain
// if provided. If domain is empty, it still includes the field (knode will return
// an error when it validates the config).
func buildIKEv2Extra(domain string) json.RawMessage {
	cfg := map[string]string{
		"domain":      domain,
		"cert_source": "letsencrypt",
		"ip_pool":     "10.10.0.0/24",
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		// Should never fail for a simple string map.
		return json.RawMessage(fmt.Sprintf(`{"domain":%q,"cert_source":"letsencrypt","ip_pool":"10.10.0.0/24"}`, domain))
	}
	return data
}

// AutoEnableCores is called after successful node registration and connection test.
// It sends EnableCore RPCs for all 5 VPN protocols with default ports and configurations.
//
// On individual failure: the error is logged, the core is marked as "error" via the
// CoreEnabler (which upserts node_services), and enablement continues for remaining cores.
//
// Requirements: 1.1 (auto-enable on registration), 1.3 (partial failure resilience), 1.4 (default ports).
func AutoEnableCores(ctx context.Context, enabler CoreEnabler, nodeID int64, domain string) []CoreEnableResult {
	defaults := defaultCores(domain)
	results := make([]CoreEnableResult, 0, len(defaults))

	for _, d := range defaults {
		err := enabler.EnableCore(ctx, nodeID, d.Core, d.Port, d.Extra)
		if err != nil {
			log.Printf("[noderegistry] AutoEnableCores: failed to enable %q on node %d: %v", d.Core, nodeID, err)
			results = append(results, CoreEnableResult{
				Core:    d.Core,
				Success: false,
				Error:   err.Error(),
			})
			continue
		}

		log.Printf("[noderegistry] AutoEnableCores: enabled %q on node %d (port %d)", d.Core, nodeID, d.Port)
		results = append(results, CoreEnableResult{
			Core:    d.Core,
			Success: true,
		})
	}

	return results
}

// ReenableStoppedCores is called on node reconnect to re-enable cores that are not
// in "running" state. It first calls AllCoreStatuses to get the current states, then
// only re-enables cores that are stopped, crashed, error, or unknown.
//
// Requirement 1.2: On reconnect, verify core states and re-enable non-running cores.
func ReenableStoppedCores(ctx context.Context, enabler CoreEnabler, nodeID int64, domain string) []CoreEnableResult {
	statuses, err := enabler.AllCoreStatuses(ctx, nodeID)
	if err != nil {
		log.Printf("[noderegistry] ReenableStoppedCores: failed to get core statuses for node %d: %v", nodeID, err)
		// If we can't get statuses, fall back to full auto-enable
		return AutoEnableCores(ctx, enabler, nodeID, domain)
	}

	// Build a map of current core states
	stateMap := make(map[string]string, len(statuses))
	for _, cs := range statuses {
		stateMap[cs.Type] = cs.State
	}

	defaults := defaultCores(domain)
	results := make([]CoreEnableResult, 0, len(defaults))

	for _, d := range defaults {
		state, exists := stateMap[d.Core]
		if exists && state == "running" {
			// Core is already running, skip re-enablement
			log.Printf("[noderegistry] ReenableStoppedCores: core %q on node %d already running, skipping", d.Core, nodeID)
			results = append(results, CoreEnableResult{
				Core:    d.Core,
				Success: true,
			})
			continue
		}

		// Core is not running (stopped, crashed, error, unknown, or not reported) — re-enable
		err := enabler.EnableCore(ctx, nodeID, d.Core, d.Port, d.Extra)
		if err != nil {
			log.Printf("[noderegistry] ReenableStoppedCores: failed to re-enable %q on node %d: %v", d.Core, nodeID, err)
			results = append(results, CoreEnableResult{
				Core:    d.Core,
				Success: false,
				Error:   err.Error(),
			})
			continue
		}

		log.Printf("[noderegistry] ReenableStoppedCores: re-enabled %q on node %d (was %q)", d.Core, nodeID, state)
		results = append(results, CoreEnableResult{
			Core:    d.Core,
			Success: true,
		})
	}

	return results
}
