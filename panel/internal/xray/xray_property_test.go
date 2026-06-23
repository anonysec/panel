//go:build !lite

package xray

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// **Validates: Requirements 5.1, 5.2, 5.4**

// validProtocols are the supported protocols for share link generation.
var validProtocols = []string{ProtocolVLESS, ProtocolVMess, ProtocolTrojan}

// validTransports are the supported transport types.
var validTransports = []string{TransportTCP, TransportWS, TransportGRPC, TransportH2}

// randomUUID generates a random UUID in valid format (xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx).
func randomUUID(rng *rand.Rand) string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		rng.Uint32(),
		rng.Uint32()&0xFFFF,
		rng.Uint32()&0xFFFF,
		rng.Uint32()&0xFFFF,
		rng.Int63()&0xFFFFFFFFFFFF,
	)
}

// randomHost generates a random hostname string.
func randomHost(rng *rand.Rand) string {
	labels := []string{"server", "node", "proxy", "edge", "cdn", "vpn"}
	tlds := []string{"com", "net", "org", "io", "dev"}
	return fmt.Sprintf("%s%d.example.%s",
		labels[rng.Intn(len(labels))],
		rng.Intn(100),
		tlds[rng.Intn(len(tlds))],
	)
}

// randomPort generates a random port in the range 1-65535.
func randomPort(rng *rand.Rand) int {
	return rng.Intn(65535) + 1
}

// randomRemark generates a random remark string.
func randomRemark(rng *rand.Rand) string {
	words := []string{"MyServer", "FastNode", "USWest", "Germany", "Tokyo", "Reality", "WS-TLS"}
	return words[rng.Intn(len(words))]
}

// randomInboundConfig generates a random valid InboundConfig for testing.
func randomInboundConfig(rng *rand.Rand) InboundConfig {
	protocol := validProtocols[rng.Intn(len(validProtocols))]
	transport := validTransports[rng.Intn(len(validTransports))]
	port := randomPort(rng)
	uuid := randomUUID(rng)

	cfg := InboundConfig{
		UUID:      uuid,
		Protocol:  protocol,
		Transport: transport,
		Port:      port,
		Security:  "none",
	}

	// Randomly add security settings.
	securityChoices := []string{"none", "tls", "reality"}
	cfg.Security = securityChoices[rng.Intn(len(securityChoices))]

	if cfg.Security == "reality" {
		cfg.ServerName = "www.google.com"
		cfg.PublicKey = fmt.Sprintf("pk%08x", rng.Uint32())
		cfg.ShortID = fmt.Sprintf("%04x", rng.Uint32()&0xFFFF)
	} else if cfg.Security == "tls" {
		cfg.ServerName = randomHost(rng)
	}

	// Add transport-specific settings.
	if transport == TransportWS || transport == TransportH2 {
		cfg.Path = fmt.Sprintf("/%s", []string{"ws", "chat", "h2", "proxy"}[rng.Intn(4)])
	}
	if transport == TransportGRPC {
		cfg.ServiceName = fmt.Sprintf("grpc%d", rng.Intn(100))
	}

	return cfg
}

// TestProperty4_ShareLinkRoundTrip verifies that GenerateShareLink produces
// correct protocol-prefixed links containing the UUID and host:port for
// any valid InboundConfig.
func TestProperty4_ShareLinkRoundTrip(t *testing.T) {
	const iterations = 200

	rng := rand.New(rand.NewSource(42))

	for i := 0; i < iterations; i++ {
		cfg := randomInboundConfig(rng)
		host := randomHost(rng)
		remark := randomRemark(rng)

		link := GenerateShareLink(cfg, host, remark)

		// Property 4.1: Return non-empty string for supported protocols.
		if link == "" {
			t.Fatalf("iteration %d: GenerateShareLink returned empty for protocol=%s transport=%s",
				i, cfg.Protocol, cfg.Transport)
		}

		// Property 4.2: Contains the correct protocol prefix.
		expectedPrefix := cfg.Protocol + "://"
		if !strings.HasPrefix(link, expectedPrefix) {
			t.Fatalf("iteration %d: expected prefix %q, got link %q",
				i, expectedPrefix, link)
		}

		// Protocol-specific checks.
		switch cfg.Protocol {
		case ProtocolVLESS:
			// Property 4.3: For VLESS, contain the UUID and host:port.
			expectedHostPort := fmt.Sprintf("%s@%s:%d", cfg.UUID, host, cfg.Port)
			if !strings.Contains(link, expectedHostPort) {
				t.Fatalf("iteration %d: VLESS link missing UUID@host:port %q in %q",
					i, expectedHostPort, link)
			}

		case ProtocolTrojan:
			// Property 4.4: For Trojan, contain the UUID and host:port.
			expectedHostPort := fmt.Sprintf("%s@%s:%d", cfg.UUID, host, cfg.Port)
			if !strings.Contains(link, expectedHostPort) {
				t.Fatalf("iteration %d: Trojan link missing UUID@host:port %q in %q",
					i, expectedHostPort, link)
			}

		case ProtocolVMess:
			// Property 4.5: For VMess, base64 decode successfully and contain the UUID.
			encoded := strings.TrimPrefix(link, "vmess://")
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				t.Fatalf("iteration %d: VMess link base64 decode failed: %v (link: %q)",
					i, err, link)
			}

			var vmessObj map[string]string
			if err := json.Unmarshal(decoded, &vmessObj); err != nil {
				t.Fatalf("iteration %d: VMess JSON unmarshal failed: %v (decoded: %q)",
					i, err, string(decoded))
			}

			if vmessObj["id"] != cfg.UUID {
				t.Fatalf("iteration %d: VMess decoded id=%q, expected %q",
					i, vmessObj["id"], cfg.UUID)
			}

			if vmessObj["add"] != host {
				t.Fatalf("iteration %d: VMess decoded add=%q, expected %q",
					i, vmessObj["add"], host)
			}

			if vmessObj["port"] != fmt.Sprintf("%d", cfg.Port) {
				t.Fatalf("iteration %d: VMess decoded port=%q, expected %d",
					i, vmessObj["port"], cfg.Port)
			}
		}
	}
}

// TestProperty5_SubscriptionEncodingRoundTrip verifies that GenerateSubscription
// base64-decoded equals the original links joined by newline, and that encoding
// is deterministic.
func TestProperty5_SubscriptionEncodingRoundTrip(t *testing.T) {
	const iterations = 200

	rng := rand.New(rand.NewSource(99))

	for i := 0; i < iterations; i++ {
		// Generate a random list of links (0 to 10 links).
		numLinks := rng.Intn(11)
		links := make([]string, numLinks)
		for j := 0; j < numLinks; j++ {
			cfg := randomInboundConfig(rng)
			host := randomHost(rng)
			remark := randomRemark(rng)
			link := GenerateShareLink(cfg, host, remark)
			if link != "" {
				links[j] = link
			} else {
				links[j] = fmt.Sprintf("vless://fallback-%d@host:%d#remark", j, randomPort(rng))
			}
		}

		// Encode.
		subscription := GenerateSubscription(links)

		// Property 5.1: base64-decoded should equal the original links joined by newline.
		decoded, err := base64.StdEncoding.DecodeString(subscription)
		if err != nil {
			t.Fatalf("iteration %d: base64 decode failed: %v", i, err)
		}

		expected := strings.Join(links, "\n")
		if string(decoded) != expected {
			t.Fatalf("iteration %d: subscription round-trip mismatch.\nExpected: %q\nGot: %q",
				i, expected, string(decoded))
		}

		// Property 5.2: Deterministic — same input produces same output.
		subscription2 := GenerateSubscription(links)
		if subscription != subscription2 {
			t.Fatalf("iteration %d: GenerateSubscription is not deterministic.\nFirst: %q\nSecond: %q",
				i, subscription, subscription2)
		}
	}
}
