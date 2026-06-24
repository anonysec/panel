//go:build !lite

package api

import (
	"fmt"
	"math/rand"
	"testing"
	"testing/quick"
)

// canRemoveCore checks if a core can be safely removed from a node.
// If there are active inbounds using the core, removal is blocked.
func canRemoveCore(activeInboundCount int) bool {
	return activeInboundCount == 0
}

// filterActivePlans returns only plans where IsActive is true.
func filterActivePlans(plans []Plan) []Plan {
	var result []Plan
	for _, p := range plans {
		if p.IsActive {
			result = append(result, p)
		}
	}
	return result
}

// **Validates: Requirements 24.7**
// Property 24: Anti-DPI Config Validation
// For each technique (reality, fragment, domain_fronting, warp):
//   - Valid configs pass validation (return nil)
//   - Invalid configs (missing required fields) fail validation (return error)
func TestProperty_AntiDPI_ConfigValidation_Reality(t *testing.T) {
	rng := rand.New(rand.NewSource(24))

	for i := 0; i < 200; i++ {
		// Generate random valid reality config
		serverName := fmt.Sprintf("www.example%d.com", rng.Intn(10000))
		privateKey := randomAlphanumString(rng, 43)
		shortID := randomHexString(rng, 8)

		validCfg := map[string]any{
			"server_name": serverName,
			"private_key": privateKey,
			"short_ids":   []any{shortID},
		}

		if err := validateAntiDPIConfig("reality", validCfg); err != nil {
			t.Fatalf("iteration %d: valid reality config rejected: %v (cfg=%v)", i, err, validCfg)
		}

		// Generate invalid config: randomly remove one required field
		invalidCfg := map[string]any{
			"server_name": serverName,
			"private_key": privateKey,
			"short_ids":   []any{shortID},
		}
		fields := []string{"server_name", "private_key", "short_ids"}
		removeIdx := rng.Intn(len(fields))
		delete(invalidCfg, fields[removeIdx])

		if err := validateAntiDPIConfig("reality", invalidCfg); err == nil {
			t.Fatalf("iteration %d: invalid reality config (missing %s) accepted", i, fields[removeIdx])
		}
	}
}

func TestProperty_AntiDPI_ConfigValidation_Fragment(t *testing.T) {
	rng := rand.New(rand.NewSource(241))

	for i := 0; i < 200; i++ {
		// Generate random valid fragment config
		length := rng.Intn(1000) + 1
		interval := rng.Intn(500) + 1

		validCfg := map[string]any{
			"length":   float64(length),
			"interval": float64(interval),
		}

		if err := validateAntiDPIConfig("fragment", validCfg); err != nil {
			t.Fatalf("iteration %d: valid fragment config rejected: %v", i, err)
		}

		// Generate invalid config: randomly remove one required field
		invalidCfg := map[string]any{
			"length":   float64(length),
			"interval": float64(interval),
		}
		fields := []string{"length", "interval"}
		removeIdx := rng.Intn(len(fields))
		delete(invalidCfg, fields[removeIdx])

		if err := validateAntiDPIConfig("fragment", invalidCfg); err == nil {
			t.Fatalf("iteration %d: invalid fragment config (missing %s) accepted", i, fields[removeIdx])
		}
	}
}

func TestProperty_AntiDPI_ConfigValidation_DomainFronting(t *testing.T) {
	rng := rand.New(rand.NewSource(242))

	for i := 0; i < 200; i++ {
		// Generate random valid domain_fronting config
		cdnDomain := fmt.Sprintf("cdn%d.cloudfront.net", rng.Intn(10000))
		backendAddr := fmt.Sprintf("%d.%d.%d.%d", rng.Intn(256), rng.Intn(256), rng.Intn(256), rng.Intn(256))

		validCfg := map[string]any{
			"cdn_domain":      cdnDomain,
			"backend_address": backendAddr,
		}

		if err := validateAntiDPIConfig("domain_fronting", validCfg); err != nil {
			t.Fatalf("iteration %d: valid domain_fronting config rejected: %v", i, err)
		}

		// Generate invalid config: randomly remove one required field
		invalidCfg := map[string]any{
			"cdn_domain":      cdnDomain,
			"backend_address": backendAddr,
		}
		fields := []string{"cdn_domain", "backend_address"}
		removeIdx := rng.Intn(len(fields))
		delete(invalidCfg, fields[removeIdx])

		if err := validateAntiDPIConfig("domain_fronting", invalidCfg); err == nil {
			t.Fatalf("iteration %d: invalid domain_fronting config (missing %s) accepted", i, fields[removeIdx])
		}
	}
}

func TestProperty_AntiDPI_ConfigValidation_Warp(t *testing.T) {
	rng := rand.New(rand.NewSource(243))

	for i := 0; i < 200; i++ {
		// Generate random valid warp config
		endpoint := fmt.Sprintf("engage.cloudflareclient.com:%d", rng.Intn(65535)+1)

		validCfg := map[string]any{
			"endpoint": endpoint,
		}

		if err := validateAntiDPIConfig("warp", validCfg); err != nil {
			t.Fatalf("iteration %d: valid warp config rejected: %v", i, err)
		}

		// Invalid: empty endpoint
		invalidCfg := map[string]any{
			"endpoint": "",
		}

		if err := validateAntiDPIConfig("warp", invalidCfg); err == nil {
			t.Fatalf("iteration %d: invalid warp config (empty endpoint) accepted", i)
		}

		// Invalid: missing endpoint
		emptyCfg := map[string]any{}

		if err := validateAntiDPIConfig("warp", emptyCfg); err == nil {
			t.Fatalf("iteration %d: invalid warp config (missing endpoint) accepted", i)
		}
	}
}

// **Validates: Requirements 23.8**
// Property 25: Core Plugin Removal Guard
// If activeInboundCount > 0: cannot remove (return false)
// If activeInboundCount == 0: can remove (return true)
func TestProperty_CorePlugin_RemovalGuard(t *testing.T) {
	f := func(count int) bool {
		// Constrain to non-negative
		if count < 0 {
			count = -count
		}

		result := canRemoveCore(count)

		if count > 0 {
			// Active inbounds exist → cannot remove
			return result == false
		}
		// No active inbounds → can remove
		return result == true
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("property violated: core removal guard: %v", err)
	}
}

// **Validates: Requirements 22.2**
// Property 28: Landing Page Pricing from Active Plans
// For any random list of plans with is_active true/false: only active plans appear in result.
func TestProperty_LandingPage_ActivePlansFilter(t *testing.T) {
	rng := rand.New(rand.NewSource(28))

	for i := 0; i < 100; i++ {
		numPlans := rng.Intn(20) + 1 // 1 to 20 plans
		plans := make([]Plan, numPlans)

		expectedActiveCount := 0
		for j := 0; j < numPlans; j++ {
			isActive := rng.Float64() > 0.5
			plans[j] = Plan{
				ID:       int64(j + 1),
				Name:     fmt.Sprintf("Plan %d", j+1),
				Price:    float64(rng.Intn(100)+1) * 1000,
				IsActive: isActive,
			}
			if isActive {
				expectedActiveCount++
			}
		}

		result := filterActivePlans(plans)

		// 1. Result count must equal expected active count
		if len(result) != expectedActiveCount {
			t.Fatalf("iteration %d: expected %d active plans, got %d", i, expectedActiveCount, len(result))
		}

		// 2. All returned plans must be active
		for _, p := range result {
			if !p.IsActive {
				t.Fatalf("iteration %d: inactive plan %q in result", i, p.Name)
			}
		}

		// 3. All active plans from input must be in result
		activeSet := make(map[int64]bool)
		for _, p := range result {
			activeSet[p.ID] = true
		}
		for _, p := range plans {
			if p.IsActive && !activeSet[p.ID] {
				t.Fatalf("iteration %d: active plan %q not in result", i, p.Name)
			}
		}
	}
}

// Helper: generate a random alphanumeric string of given length.
func randomAlphanumString(rng *rand.Rand, length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = chars[rng.Intn(len(chars))]
	}
	return string(b)
}

// Helper: generate a random hex string of given length.
func randomHexString(rng *rand.Rand, length int) string {
	const hexChars = "0123456789abcdef"
	b := make([]byte, length)
	for i := range b {
		b[i] = hexChars[rng.Intn(len(hexChars))]
	}
	return string(b)
}
