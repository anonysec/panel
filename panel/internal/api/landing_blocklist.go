//go:build !lite

package api

import (
	"fmt"
	"strings"
)

// vpnBlocklist contains terms that must not appear in any landing page content.
// These terms would reveal the server's VPN management purpose to external
// scanners or censors. This is the content validation blocklist per R14.1/R14.8.
var vpnBlocklist = []string{
	"vpn",
	"proxy",
	"tunnel",
	"openvpn",
	"wireguard",
	"ikev2",
	"l2tp",
	"xray",
	"vless",
	"vmess",
	"trojan",
	"mtproto",
	"ssh tunnel",
	"shadowsocks",
	"v2ray",
	"koris",
	"korispanel",
}

// CheckBlocklist checks a text string against the VPN blocklist.
// Returns the first matched term (lowercased) or empty string if clean.
// The check is case-insensitive.
func CheckBlocklist(text string) string {
	lower := strings.ToLower(text)
	for _, term := range vpnBlocklist {
		if strings.Contains(lower, term) {
			return term
		}
	}
	return ""
}

// CheckBlocklistAll checks multiple text fields against the VPN blocklist.
// Returns a map of field name → matched blocklist term for any violations found.
// Returns nil if all fields are clean.
func CheckBlocklistAll(fields map[string]string) map[string]string {
	violations := map[string]string{}
	for field, text := range fields {
		if term := CheckBlocklist(text); term != "" {
			violations[field] = term
		}
	}
	if len(violations) == 0 {
		return nil
	}
	return violations
}

// CheckLandingContentBlocklist validates an entire LandingContent struct against
// the blocklist. Returns violations map (field → term) or nil if clean.
func CheckLandingContentBlocklist(c *LandingContent) map[string]string {
	fields := make(map[string]string)
	fields["hero_title"] = c.HeroTitle
	fields["hero_subtitle"] = c.HeroSubtitle
	fields["footer_text"] = c.FooterText

	for i, f := range c.Features {
		fields[fmt.Sprintf("features[%d].title", i)] = f.Title
		fields[fmt.Sprintf("features[%d].description", i)] = f.Description
	}

	for i, p := range c.Pricing {
		fields[fmt.Sprintf("pricing[%d].name", i)] = p.Name
		fields[fmt.Sprintf("pricing[%d].price", i)] = p.Price
		for j, feat := range p.Features {
			fields[fmt.Sprintf("pricing[%d].features[%d]", i, j)] = feat
		}
	}

	for i, faq := range c.FAQ {
		fields[fmt.Sprintf("faq[%d].question", i)] = faq.Question
		fields[fmt.Sprintf("faq[%d].answer", i)] = faq.Answer
	}

	return CheckBlocklistAll(fields)
}
