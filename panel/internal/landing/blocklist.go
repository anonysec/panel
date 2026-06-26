//go:build !lite

package landing

import "strings"

// VPNBlocklist contains terms that should not appear in landing page content
// to avoid revealing the server's VPN management purpose to external observers.
var VPNBlocklist = []string{
	"vpn", "proxy", "tunnel", "openvpn", "wireguard", "ikev2",
	"l2tp", "xray", "vless", "vmess", "trojan", "mtproto",
	"ssh tunnel", "shadowsocks", "v2ray", "koris", "korispanel",
}

// BlocklistMatch represents a single match of a blocklist term in a content field.
type BlocklistMatch struct {
	Field string `json:"field"`
	Term  string `json:"term"`
}

// CheckContent checks a single string content against the VPN blocklist.
// Returns all matching terms found (case-insensitive substring match).
func CheckContent(content string) []string {
	if content == "" {
		return nil
	}
	lower := strings.ToLower(content)
	var matches []string
	for _, term := range VPNBlocklist {
		if strings.Contains(lower, term) {
			matches = append(matches, term)
		}
	}
	return matches
}

// ValidateFields checks multiple named content fields against the blocklist.
// Returns a list of BlocklistMatch entries indicating which field contained which term.
func ValidateFields(fields map[string]string) []BlocklistMatch {
	var results []BlocklistMatch
	for fieldName, content := range fields {
		terms := CheckContent(content)
		for _, term := range terms {
			results = append(results, BlocklistMatch{
				Field: fieldName,
				Term:  term,
			})
		}
	}
	return results
}
