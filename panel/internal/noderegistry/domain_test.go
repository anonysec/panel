package noderegistry

import (
	"strings"
	"testing"
)

func TestValidateNodeDomain_EmptyDomain(t *testing.T) {
	_, err := ValidateNodeDomain("", "1.2.3.4")
	if err == nil {
		t.Fatal("expected error for empty domain")
	}
	if !strings.Contains(err.Error(), "domain is empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateNodeDomain_EmptyNodeIP(t *testing.T) {
	_, err := ValidateNodeDomain("example.com", "")
	if err == nil {
		t.Fatal("expected error for empty node IP")
	}
	if !strings.Contains(err.Error(), "node IP is empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateNodeDomain_NonexistentDomain(t *testing.T) {
	_, err := ValidateNodeDomain("this-domain-does-not-exist-xyz123.invalid", "1.2.3.4")
	if err == nil {
		t.Fatal("expected error for non-existent domain")
	}
	if !strings.Contains(err.Error(), "DNS resolution failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateNodeDomain_MatchingIP(t *testing.T) {
	// Use localhost which should resolve to 127.0.0.1
	warnings, err := ValidateNodeDomain("localhost", "127.0.0.1")
	if err != nil {
		// Some systems may not resolve localhost via DNS — skip if resolution fails
		t.Skipf("localhost DNS resolution not available: %v", err)
	}
	if len(warnings) > 0 {
		t.Fatalf("expected no warnings for matching IP, got: %v", warnings)
	}
}

func TestValidateNodeDomain_MismatchedIP(t *testing.T) {
	// Use a well-known domain that won't resolve to our test IP
	warnings, err := ValidateNodeDomain("localhost", "192.168.255.255")
	if err != nil {
		// Some systems may not resolve localhost via DNS — skip if resolution fails
		t.Skipf("localhost DNS resolution not available: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected warnings for mismatched IP")
	}
	if !strings.Contains(warnings[0], "192.168.255.255") {
		t.Fatalf("warning should mention the node IP, got: %s", warnings[0])
	}
}

func TestValidateNodeDomain_WhitespaceDomain(t *testing.T) {
	_, err := ValidateNodeDomain("   ", "1.2.3.4")
	if err == nil {
		t.Fatal("expected error for whitespace-only domain")
	}
}
