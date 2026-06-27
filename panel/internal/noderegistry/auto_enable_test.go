package noderegistry

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

// mockCoreEnabler implements CoreEnabler for testing.
type mockCoreEnabler struct {
	enableCalls []enableCall
	statuses    []CoreStatusInfo
	statusErr   error
	failCores   map[string]error // cores that should fail on EnableCore
}

type enableCall struct {
	NodeID      int64
	CoreType    string
	ListenPort  int
	ExtraConfig json.RawMessage
}

func (m *mockCoreEnabler) EnableCore(ctx context.Context, nodeID int64, coreType string, listenPort int, extraConfig json.RawMessage) error {
	m.enableCalls = append(m.enableCalls, enableCall{
		NodeID:      nodeID,
		CoreType:    coreType,
		ListenPort:  listenPort,
		ExtraConfig: extraConfig,
	})
	if m.failCores != nil {
		if err, ok := m.failCores[coreType]; ok {
			return err
		}
	}
	return nil
}

func (m *mockCoreEnabler) AllCoreStatuses(ctx context.Context, nodeID int64) ([]CoreStatusInfo, error) {
	if m.statusErr != nil {
		return nil, m.statusErr
	}
	return m.statuses, nil
}

func TestAutoEnableCores_AllSuccess(t *testing.T) {
	mock := &mockCoreEnabler{}
	results := AutoEnableCores(context.Background(), mock, 42, "vpn.example.com")

	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}

	for _, r := range results {
		if !r.Success {
			t.Errorf("expected success for core %q, got error: %s", r.Core, r.Error)
		}
	}

	if len(mock.enableCalls) != 5 {
		t.Fatalf("expected 5 EnableCore calls, got %d", len(mock.enableCalls))
	}

	// Verify correct protocols and ports (Requirement 1.4)
	expectedCores := []struct {
		core string
		port int
	}{
		{"openvpn", 1194},
		{"wireguard", 51820},
		{"l2tp", 1701},
		{"ikev2", 500},
		{"ssh", 2222},
	}

	for i, exp := range expectedCores {
		call := mock.enableCalls[i]
		if call.CoreType != exp.core {
			t.Errorf("call[%d]: expected core %q, got %q", i, exp.core, call.CoreType)
		}
		if call.ListenPort != exp.port {
			t.Errorf("call[%d]: expected port %d, got %d", i, exp.port, call.ListenPort)
		}
		if call.NodeID != 42 {
			t.Errorf("call[%d]: expected nodeID 42, got %d", i, call.NodeID)
		}
	}
}

func TestAutoEnableCores_PartialFailure(t *testing.T) {
	mock := &mockCoreEnabler{
		failCores: map[string]error{
			"wireguard": fmt.Errorf("package install failed"),
			"ikev2":     fmt.Errorf("domain resolution failed"),
		},
	}

	results := AutoEnableCores(context.Background(), mock, 1, "vpn.example.com")

	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}

	// All 5 EnableCore calls should have been made (resilience to partial failure)
	if len(mock.enableCalls) != 5 {
		t.Fatalf("expected 5 EnableCore calls despite failures, got %d", len(mock.enableCalls))
	}

	// Check individual results
	for _, r := range results {
		switch r.Core {
		case "wireguard":
			if r.Success {
				t.Error("expected wireguard to fail")
			}
			if r.Error != "package install failed" {
				t.Errorf("expected specific error for wireguard, got %q", r.Error)
			}
		case "ikev2":
			if r.Success {
				t.Error("expected ikev2 to fail")
			}
		case "openvpn", "l2tp", "ssh":
			if !r.Success {
				t.Errorf("expected %q to succeed, got error: %s", r.Core, r.Error)
			}
		}
	}
}

func TestAutoEnableCores_AllFail(t *testing.T) {
	mock := &mockCoreEnabler{
		failCores: map[string]error{
			"openvpn":   fmt.Errorf("fail"),
			"wireguard": fmt.Errorf("fail"),
			"l2tp":      fmt.Errorf("fail"),
			"ikev2":     fmt.Errorf("fail"),
			"ssh":       fmt.Errorf("fail"),
		},
	}

	results := AutoEnableCores(context.Background(), mock, 1, "")

	// All should still attempt
	if len(mock.enableCalls) != 5 {
		t.Fatalf("expected 5 EnableCore calls even when all fail, got %d", len(mock.enableCalls))
	}

	for _, r := range results {
		if r.Success {
			t.Errorf("expected failure for core %q", r.Core)
		}
	}
}

func TestAutoEnableCores_IKEv2DomainInjection(t *testing.T) {
	mock := &mockCoreEnabler{}
	AutoEnableCores(context.Background(), mock, 1, "vpn.myhost.com")

	// Find the IKEv2 call
	var ikev2Call *enableCall
	for i := range mock.enableCalls {
		if mock.enableCalls[i].CoreType == "ikev2" {
			ikev2Call = &mock.enableCalls[i]
			break
		}
	}
	if ikev2Call == nil {
		t.Fatal("no IKEv2 EnableCore call found")
	}

	// Verify domain is in the extra config
	var cfg map[string]string
	if err := json.Unmarshal(ikev2Call.ExtraConfig, &cfg); err != nil {
		t.Fatalf("failed to parse IKEv2 extra config: %v", err)
	}
	if cfg["domain"] != "vpn.myhost.com" {
		t.Errorf("expected domain %q in IKEv2 config, got %q", "vpn.myhost.com", cfg["domain"])
	}
	if cfg["cert_source"] != "letsencrypt" {
		t.Errorf("expected cert_source %q, got %q", "letsencrypt", cfg["cert_source"])
	}
}

func TestAutoEnableCores_EmptyDomain(t *testing.T) {
	mock := &mockCoreEnabler{}
	AutoEnableCores(context.Background(), mock, 1, "")

	// IKEv2 should still be called (knode will reject it if domain is required)
	var ikev2Call *enableCall
	for i := range mock.enableCalls {
		if mock.enableCalls[i].CoreType == "ikev2" {
			ikev2Call = &mock.enableCalls[i]
			break
		}
	}
	if ikev2Call == nil {
		t.Fatal("no IKEv2 EnableCore call found even with empty domain")
	}

	var cfg map[string]string
	if err := json.Unmarshal(ikev2Call.ExtraConfig, &cfg); err != nil {
		t.Fatalf("failed to parse IKEv2 extra config: %v", err)
	}
	if cfg["domain"] != "" {
		t.Errorf("expected empty domain in IKEv2 config, got %q", cfg["domain"])
	}
}

func TestReenableStoppedCores_SkipsRunning(t *testing.T) {
	mock := &mockCoreEnabler{
		statuses: []CoreStatusInfo{
			{Type: "openvpn", State: "running"},
			{Type: "wireguard", State: "running"},
			{Type: "l2tp", State: "stopped"},
			{Type: "ikev2", State: "crashed"},
			{Type: "ssh", State: "running"},
		},
	}

	results := ReenableStoppedCores(context.Background(), mock, 1, "vpn.example.com")

	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}

	// Only l2tp and ikev2 should have been re-enabled (not running)
	if len(mock.enableCalls) != 2 {
		t.Fatalf("expected 2 EnableCore calls (only non-running), got %d", len(mock.enableCalls))
	}

	enabledCores := make(map[string]bool)
	for _, call := range mock.enableCalls {
		enabledCores[call.CoreType] = true
	}

	if !enabledCores["l2tp"] {
		t.Error("expected l2tp to be re-enabled (was stopped)")
	}
	if !enabledCores["ikev2"] {
		t.Error("expected ikev2 to be re-enabled (was crashed)")
	}
	if enabledCores["openvpn"] {
		t.Error("openvpn should not be re-enabled (was running)")
	}
	if enabledCores["wireguard"] {
		t.Error("wireguard should not be re-enabled (was running)")
	}
	if enabledCores["ssh"] {
		t.Error("ssh should not be re-enabled (was running)")
	}
}

func TestReenableStoppedCores_AllRunning(t *testing.T) {
	mock := &mockCoreEnabler{
		statuses: []CoreStatusInfo{
			{Type: "openvpn", State: "running"},
			{Type: "wireguard", State: "running"},
			{Type: "l2tp", State: "running"},
			{Type: "ikev2", State: "running"},
			{Type: "ssh", State: "running"},
		},
	}

	results := ReenableStoppedCores(context.Background(), mock, 1, "vpn.example.com")

	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}

	// No EnableCore calls should be made since all are running
	if len(mock.enableCalls) != 0 {
		t.Fatalf("expected 0 EnableCore calls (all running), got %d", len(mock.enableCalls))
	}

	for _, r := range results {
		if !r.Success {
			t.Errorf("expected success for running core %q", r.Core)
		}
	}
}

func TestReenableStoppedCores_StatusFetchFails_FallsBackToFullEnable(t *testing.T) {
	mock := &mockCoreEnabler{
		statusErr: fmt.Errorf("node unreachable"),
	}

	results := ReenableStoppedCores(context.Background(), mock, 1, "vpn.example.com")

	// Should fall back to AutoEnableCores (all 5 calls)
	if len(mock.enableCalls) != 5 {
		t.Fatalf("expected 5 EnableCore calls on status fetch failure (fallback), got %d", len(mock.enableCalls))
	}

	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}
}

func TestReenableStoppedCores_CoreNotReportedByNode(t *testing.T) {
	// If a core isn't reported in statuses, it should be re-enabled
	mock := &mockCoreEnabler{
		statuses: []CoreStatusInfo{
			{Type: "openvpn", State: "running"},
			{Type: "wireguard", State: "running"},
			// l2tp, ikev2, ssh not reported → should be re-enabled
		},
	}

	results := ReenableStoppedCores(context.Background(), mock, 1, "vpn.example.com")

	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}

	// l2tp, ikev2, ssh should be re-enabled (not in status map)
	if len(mock.enableCalls) != 3 {
		t.Fatalf("expected 3 EnableCore calls (unreported cores), got %d", len(mock.enableCalls))
	}

	enabledCores := make(map[string]bool)
	for _, call := range mock.enableCalls {
		enabledCores[call.CoreType] = true
	}

	if !enabledCores["l2tp"] {
		t.Error("expected l2tp to be re-enabled (not reported)")
	}
	if !enabledCores["ikev2"] {
		t.Error("expected ikev2 to be re-enabled (not reported)")
	}
	if !enabledCores["ssh"] {
		t.Error("expected ssh to be re-enabled (not reported)")
	}
}

func TestBuildIKEv2Extra_WithDomain(t *testing.T) {
	extra := buildIKEv2Extra("vpn.test.com")

	var cfg map[string]string
	if err := json.Unmarshal(extra, &cfg); err != nil {
		t.Fatalf("failed to parse IKEv2 extra: %v", err)
	}

	if cfg["domain"] != "vpn.test.com" {
		t.Errorf("expected domain %q, got %q", "vpn.test.com", cfg["domain"])
	}
	if cfg["cert_source"] != "letsencrypt" {
		t.Errorf("expected cert_source %q, got %q", "letsencrypt", cfg["cert_source"])
	}
	if cfg["ip_pool"] != "10.10.0.0/24" {
		t.Errorf("expected ip_pool %q, got %q", "10.10.0.0/24", cfg["ip_pool"])
	}
}

func TestBuildIKEv2Extra_EmptyDomain(t *testing.T) {
	extra := buildIKEv2Extra("")

	var cfg map[string]string
	if err := json.Unmarshal(extra, &cfg); err != nil {
		t.Fatalf("failed to parse IKEv2 extra: %v", err)
	}

	if cfg["domain"] != "" {
		t.Errorf("expected empty domain, got %q", cfg["domain"])
	}
}

func TestDefaultCores_ReturnsAllFiveProtocols(t *testing.T) {
	cores := defaultCores("test.com")

	if len(cores) != 5 {
		t.Fatalf("expected 5 default cores, got %d", len(cores))
	}

	expected := map[string]int{
		"openvpn":   1194,
		"wireguard": 51820,
		"l2tp":      1701,
		"ikev2":     500,
		"ssh":       2222,
	}

	for _, c := range cores {
		expectedPort, ok := expected[c.Core]
		if !ok {
			t.Errorf("unexpected core %q in defaults", c.Core)
			continue
		}
		if c.Port != expectedPort {
			t.Errorf("core %q: expected port %d, got %d", c.Core, expectedPort, c.Port)
		}
	}
}
