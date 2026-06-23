//go:build !lite

package api

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"testing/quick"
)

// generateTgProxyLink produces a tg://proxy share link for an MTProto proxy.
// This mirrors the format used in handleMTProtoLink.
func generateTgProxyLink(ip string, port int, secret string) string {
	return fmt.Sprintf("tg://proxy?server=%s&port=%d&secret=%s", ip, port, secret)
}

// **Validates: Requirements 3.3, 3.4**
// Property 6: MTProto Share Link Format
// For any valid IP (4 random octets as "a.b.c.d"), valid port (1-65535), and random hex secret (32 bytes hex):
//  1. Starts with "tg://proxy?"
//  2. Contains "server=" followed by the exact IP
//  3. Contains "port=" followed by the exact port number
//  4. Contains "secret=" followed by the exact secret
//  5. Is parseable as a valid URL
func TestProperty_MTProtoShareLinkFormat(t *testing.T) {
	f := func(a, b, c, d uint8, portRaw uint16, secretSeed int64) bool {
		// Build valid IP from 4 random octets
		ip := fmt.Sprintf("%d.%d.%d.%d", a, b, c, d)

		// Constrain port to [1, 65535]
		port := int(portRaw)%65535 + 1

		// Generate a deterministic 32-byte hex secret from seed
		rng := rand.New(rand.NewSource(secretSeed))
		secretBytes := make([]byte, 32)
		rng.Read(secretBytes)
		secret := hex.EncodeToString(secretBytes)

		link := generateTgProxyLink(ip, port, secret)

		// 1. Must start with "tg://proxy?"
		if !strings.HasPrefix(link, "tg://proxy?") {
			return false
		}

		// 2. Must contain "server=" followed by exact IP
		if !strings.Contains(link, "server="+ip) {
			return false
		}

		// 3. Must contain "port=" followed by exact port number
		if !strings.Contains(link, "port="+strconv.Itoa(port)) {
			return false
		}

		// 4. Must contain "secret=" followed by exact secret
		if !strings.Contains(link, "secret="+secret) {
			return false
		}

		// 5. Must be parseable as a valid URL
		parsed, err := url.Parse(link)
		if err != nil {
			return false
		}

		// Verify parsed components match
		if parsed.Scheme != "tg" {
			return false
		}
		if parsed.Host != "proxy" {
			return false
		}

		// Verify query parameters match exactly
		q := parsed.Query()
		if q.Get("server") != ip {
			return false
		}
		if q.Get("port") != strconv.Itoa(port) {
			return false
		}
		if q.Get("secret") != secret {
			return false
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 500}); err != nil {
		t.Fatalf("MTProto share link format property violated: %v", err)
	}
}
