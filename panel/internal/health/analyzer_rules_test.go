package health

import (
	"strings"
	"testing"
)

func TestRuleBasedAnalyzer_AllHealthy(t *testing.T) {
	rba := &RuleBasedAnalyzer{}
	input := AnalysisInput{
		CheckResults: []CheckResult{
			{Name: "database_connectivity", Category: "database", Severity: SeverityHealthy},
			{Name: "node_online_status", Category: "nodes", Severity: SeverityHealthy},
		},
	}
	out, err := rba.Analyze(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.RootCause != "All checks healthy" {
		t.Errorf("expected 'All checks healthy', got %q", out.RootCause)
	}
	if out.Confidence != 1.0 {
		t.Errorf("expected confidence 1.0, got %f", out.Confidence)
	}
}

func TestRuleBasedAnalyzer_EmptyInput(t *testing.T) {
	rba := &RuleBasedAnalyzer{}
	out, err := rba.Analyze(AnalysisInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.RootCause != "No issues detected" {
		t.Errorf("expected 'No issues detected', got %q", out.RootCause)
	}
}

func TestRuleBasedAnalyzer_NodeOfflineAndVPNDown(t *testing.T) {
	rba := &RuleBasedAnalyzer{}
	input := AnalysisInput{
		CheckResults: []CheckResult{
			{Name: "node_online_status", Category: "nodes", Severity: SeverityCritical, Message: "Nodes offline"},
			{Name: "vpn_service_health", Category: "vpn", Severity: SeverityCritical, Message: "VPN services not running"},
		},
	}
	out, err := rba.Analyze(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.RootCause, "Node failure") {
		t.Errorf("expected root cause containing 'Node failure', got %q", out.RootCause)
	}
	if out.Confidence != 0.9 {
		t.Errorf("expected confidence 0.9, got %f", out.Confidence)
	}
	if len(out.AffectedComponents) != 2 {
		t.Errorf("expected 2 affected components, got %d", len(out.AffectedComponents))
	}
}

func TestRuleBasedAnalyzer_HighDiskAndVPNCrash(t *testing.T) {
	rba := &RuleBasedAnalyzer{}
	input := AnalysisInput{
		CheckResults: []CheckResult{
			{Name: "disk_usage", Category: "resources", Severity: SeverityCritical, Message: "Disk full"},
			{Name: "vpn_service_health", Category: "vpn", Severity: SeverityCritical, Message: "VPN crashed"},
		},
	}
	out, err := rba.Analyze(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.RootCause, "Disk space causing service failure") {
		t.Errorf("expected root cause about disk causing service failure, got %q", out.RootCause)
	}
	if out.Confidence != 0.85 {
		t.Errorf("expected confidence 0.85, got %f", out.Confidence)
	}
}

func TestRuleBasedAnalyzer_MultipleCritical(t *testing.T) {
	rba := &RuleBasedAnalyzer{}
	input := AnalysisInput{
		CheckResults: []CheckResult{
			{Name: "stale_sessions", Category: "sessions", Severity: SeverityCritical, Message: "Stale sessions"},
			{Name: "expired_subscriptions", Category: "subscriptions", Severity: SeverityCritical, Message: "Expired subs"},
			{Name: "dns_failover_status", Category: "failover", Severity: SeverityCritical, Message: "Failover issues"},
		},
	}
	out, err := rba.Analyze(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.RootCause, "Multiple system degradation") {
		t.Errorf("expected root cause about multiple system degradation, got %q", out.RootCause)
	}
	if out.Confidence != 0.7 {
		t.Errorf("expected confidence 0.7, got %f", out.Confidence)
	}
}

func TestRuleBasedAnalyzer_DatabaseDown(t *testing.T) {
	rba := &RuleBasedAnalyzer{}
	input := AnalysisInput{
		CheckResults: []CheckResult{
			{Name: "database_connectivity", Category: "database", Severity: SeverityCritical, Message: "DB unreachable"},
		},
	}
	out, err := rba.Analyze(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.RootCause, "Database connectivity failure") {
		t.Errorf("expected database connectivity failure, got %q", out.RootCause)
	}
	if out.Confidence != 0.95 {
		t.Errorf("expected confidence 0.95, got %f", out.Confidence)
	}
}

func TestRuleBasedAnalyzer_SingleWarning(t *testing.T) {
	rba := &RuleBasedAnalyzer{}
	input := AnalysisInput{
		CheckResults: []CheckResult{
			{Name: "stale_sessions", Category: "sessions", Severity: SeverityWarning, Message: "2 stale sessions", SuggestedActions: []string{"Clear stale sessions"}},
		},
	}
	out, err := rba.Analyze(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Single issue falls through to default summary
	if out.RootCause != "2 stale sessions" {
		t.Errorf("expected single issue message, got %q", out.RootCause)
	}
	if len(out.SuggestedActions) == 0 {
		t.Error("expected at least one suggested action")
	}
}

func TestDeduplicateStrings(t *testing.T) {
	input := []string{"a", "b", "a", "c", "b"}
	result := deduplicateStrings(input)
	if len(result) != 3 {
		t.Errorf("expected 3, got %d", len(result))
	}
	expected := []string{"a", "b", "c"}
	for i, v := range expected {
		if result[i] != v {
			t.Errorf("index %d: expected %q, got %q", i, v, result[i])
		}
	}
}
