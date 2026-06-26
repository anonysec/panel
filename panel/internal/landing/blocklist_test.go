//go:build !lite

package landing

import "testing"

func TestCheckContent_EmptyString(t *testing.T) {
	matches := CheckContent("")
	if len(matches) != 0 {
		t.Errorf("expected no matches for empty string, got %v", matches)
	}
}

func TestCheckContent_NoMatch(t *testing.T) {
	matches := CheckContent("Welcome to our business solutions platform")
	if len(matches) != 0 {
		t.Errorf("expected no matches, got %v", matches)
	}
}

func TestCheckContent_SingleMatch(t *testing.T) {
	matches := CheckContent("Fast VPN management system")
	if len(matches) != 1 || matches[0] != "vpn" {
		t.Errorf("expected [vpn], got %v", matches)
	}
}

func TestCheckContent_CaseInsensitive(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"VPN service", "vpn"},
		{"Vpn service", "vpn"},
		{"vPn service", "vpn"},
		{"WIREGUARD config", "wireguard"},
		{"WireGuard config", "wireguard"},
		{"KORISPANEL dashboard", "korispanel"},
	}

	for _, tc := range tests {
		matches := CheckContent(tc.input)
		found := false
		for _, m := range matches {
			if m == tc.want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("CheckContent(%q) should contain %q, got %v", tc.input, tc.want, matches)
		}
	}
}

func TestCheckContent_MultipleMatches(t *testing.T) {
	matches := CheckContent("Setup VPN with WireGuard and OpenVPN protocols")
	// Should match: vpn, wireguard, openvpn
	if len(matches) < 3 {
		t.Errorf("expected at least 3 matches, got %v", matches)
	}
	expected := map[string]bool{"vpn": false, "wireguard": false, "openvpn": false}
	for _, m := range matches {
		if _, ok := expected[m]; ok {
			expected[m] = true
		}
	}
	for term, found := range expected {
		if !found {
			t.Errorf("expected to find %q in matches", term)
		}
	}
}

func TestCheckContent_SubstringMatch(t *testing.T) {
	// "tunnel" should match as a substring in longer text
	matches := CheckContent("ssh tunnel connection")
	foundTunnel := false
	foundSSHTunnel := false
	for _, m := range matches {
		if m == "tunnel" {
			foundTunnel = true
		}
		if m == "ssh tunnel" {
			foundSSHTunnel = true
		}
	}
	if !foundTunnel {
		t.Error("expected 'tunnel' to match in 'ssh tunnel connection'")
	}
	if !foundSSHTunnel {
		t.Error("expected 'ssh tunnel' to match in 'ssh tunnel connection'")
	}
}

func TestCheckContent_AllBlocklistTerms(t *testing.T) {
	for _, term := range VPNBlocklist {
		matches := CheckContent(term)
		if len(matches) == 0 {
			t.Errorf("expected %q to match itself", term)
		}
		found := false
		for _, m := range matches {
			if m == term {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q to be in matches %v", term, matches)
		}
	}
}

func TestValidateFields_Empty(t *testing.T) {
	results := ValidateFields(map[string]string{})
	if len(results) != 0 {
		t.Errorf("expected no results for empty fields, got %v", results)
	}
}

func TestValidateFields_CleanContent(t *testing.T) {
	fields := map[string]string{
		"hero_title":    "Welcome to Our Platform",
		"hero_subtitle": "Professional business solutions",
		"footer_text":   "All rights reserved",
	}
	results := ValidateFields(fields)
	if len(results) != 0 {
		t.Errorf("expected no results for clean content, got %v", results)
	}
}

func TestValidateFields_WithMatches(t *testing.T) {
	fields := map[string]string{
		"hero_title":    "Best VPN Service",
		"hero_subtitle": "Fast and secure connectivity",
		"footer_text":   "Powered by KorisPanel",
	}
	results := ValidateFields(fields)
	if len(results) == 0 {
		t.Error("expected matches but got none")
	}

	// Should find "vpn" in hero_title and "koris"/"korispanel" in footer_text
	heroMatch := false
	footerMatch := false
	for _, r := range results {
		if r.Field == "hero_title" && r.Term == "vpn" {
			heroMatch = true
		}
		if r.Field == "footer_text" && (r.Term == "korispanel" || r.Term == "koris") {
			footerMatch = true
		}
	}
	if !heroMatch {
		t.Error("expected 'vpn' match in hero_title")
	}
	if !footerMatch {
		t.Error("expected 'koris' or 'korispanel' match in footer_text")
	}
}
