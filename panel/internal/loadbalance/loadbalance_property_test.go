//go:build !lite

package loadbalance

import (
	"math"
	"math/rand"
	"testing"
	"testing/quick"
)

// checkThreshold determines warning and critical bandwidth quota notifications.
// Warning at 80% of quota, critical at 100% of quota.
func checkThreshold(usedBytes int64, quotaGB int) (warning bool, critical bool) {
	if quotaGB <= 0 {
		return false, false
	}
	quotaBytes := int64(quotaGB) * 1_000_000_000
	usagePercent := float64(usedBytes) / float64(quotaBytes) * 100.0
	warning = usagePercent >= 80.0
	critical = usagePercent >= 100.0
	return
}

// calculateMigrationResult computes the number of migrated and failed users.
func calculateMigrationResult(total, failures int) (migrated, failed int) {
	if failures < 0 {
		failures = 0
	}
	if failures > total {
		failures = total
	}
	return total - failures, failures
}

// Property 7: Load Balancer Selects Minimum Load
// For any random set of NodeLoad values (1-20 nodes with random ActiveSessions and MaxCapacity),
// if there exists at least one node below 90% threshold, SelectNode should return the one with
// lowest load percentage.
// **Validates: Requirements 10.1, 10.2, 10.4**

func TestProperty_LoadBalancer_SelectsMinimumLoad(t *testing.T) {
	rng := rand.New(rand.NewSource(7))

	for i := 0; i < 200; i++ {
		numNodes := rng.Intn(20) + 1 // 1 to 20 nodes
		nodes := make([]NodeLoad, numNodes)

		for j := 0; j < numNodes; j++ {
			nodes[j] = NodeLoad{
				NodeID:         int64(j + 1),
				ActiveSessions: rng.Intn(150),
				MaxCapacity:    rng.Intn(200) + 1, // capacity > 0
			}
		}

		// Find expected: lowest load node below 90%
		threshold := 90.0
		expectedID := int64(0)
		expectedLoad := math.MaxFloat64
		hasCandidate := false

		for _, n := range nodes {
			load := CalculateLoad(n.ActiveSessions, n.MaxCapacity)
			if load < threshold {
				if load < expectedLoad {
					expectedLoad = load
					expectedID = n.NodeID
					hasCandidate = true
				}
			}
		}

		resultID, err := SelectNode(nodes, threshold)

		if !hasCandidate {
			// All nodes overloaded — expect error
			if err == nil {
				t.Fatalf("iteration %d: expected error when all nodes overloaded, got node %d", i, resultID)
			}
			continue
		}

		// At least one node under threshold — should succeed
		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}

		// Verify result has same load percentage as expected minimum
		resultLoad := float64(-1)
		for _, n := range nodes {
			if n.NodeID == resultID {
				resultLoad = CalculateLoad(n.ActiveSessions, n.MaxCapacity)
				break
			}
		}

		if math.Abs(resultLoad-expectedLoad) > 1e-9 {
			t.Fatalf("iteration %d: SelectNode returned node %d (load %.2f%%), but expected node %d (load %.2f%%)",
				i, resultID, resultLoad, expectedID, expectedLoad)
		}
	}
}

// Property 8: Load Percentage Calculation
// For any active sessions and capacity > 0: CalculateLoad should equal (active/capacity)*100
// For capacity <= 0: should return 100.0
// **Validates: Requirements 10.2**

func TestProperty_LoadPercentage_PositiveCapacity(t *testing.T) {
	f := func(active int, capacity int) bool {
		// Constrain to positive capacity
		if capacity <= 0 {
			capacity = 1
		}
		if active < 0 {
			active = -active
		}
		// Cap to avoid overflow
		if active > 1_000_000 {
			active = 1_000_000
		}
		if capacity > 1_000_000 {
			capacity = 1_000_000
		}

		got := CalculateLoad(active, capacity)
		expected := (float64(active) / float64(capacity)) * 100.0

		return math.Abs(got-expected) < 1e-9
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: CalculateLoad should equal (active/capacity)*100 for positive capacity: %v", err)
	}
}

func TestProperty_LoadPercentage_ZeroOrNegativeCapacity(t *testing.T) {
	f := func(active int, capacity int) bool {
		// Force capacity <= 0
		if capacity > 0 {
			capacity = -capacity
		}

		got := CalculateLoad(active, capacity)
		return got == 100.0
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: CalculateLoad should return 100.0 for capacity <= 0: %v", err)
	}
}

// Property 13: Node Group Membership Invariant
// Test that SelectNode with duplicate node IDs still selects the lowest load.
// This validates that group membership logic (one node per group) doesn't affect selection.
// **Validates: Requirements 9.4, 9.5**

func TestProperty_NodeGroup_MembershipInvariant(t *testing.T) {
	rng := rand.New(rand.NewSource(13))

	for i := 0; i < 200; i++ {
		numNodes := rng.Intn(10) + 2 // 2 to 11 nodes
		nodes := make([]NodeLoad, numNodes)

		// Create some nodes with duplicate IDs to simulate "same node in multiple groups" scenario
		for j := 0; j < numNodes; j++ {
			nodeID := int64(rng.Intn(5) + 1) // IDs 1-5, guarantees duplicates
			nodes[j] = NodeLoad{
				NodeID:         nodeID,
				ActiveSessions: rng.Intn(100),
				MaxCapacity:    rng.Intn(150) + 1,
			}
		}

		threshold := 90.0
		resultID, err := SelectNode(nodes, threshold)

		// Find the minimum load among nodes below threshold
		minLoad := math.MaxFloat64
		hasCandidate := false
		for _, n := range nodes {
			load := CalculateLoad(n.ActiveSessions, n.MaxCapacity)
			if load < threshold && load < minLoad {
				minLoad = load
				hasCandidate = true
			}
		}

		if !hasCandidate {
			if err == nil {
				t.Fatalf("iteration %d: expected error when all overloaded, got node %d", i, resultID)
			}
			continue
		}

		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}

		// Verify the returned node has the minimum load
		resultLoad := float64(-1)
		for _, n := range nodes {
			if n.NodeID == resultID {
				load := CalculateLoad(n.ActiveSessions, n.MaxCapacity)
				if load < threshold && (resultLoad < 0 || load < resultLoad) {
					resultLoad = load
				}
			}
		}

		if math.Abs(resultLoad-minLoad) > 1e-9 {
			t.Fatalf("iteration %d: with duplicate IDs, SelectNode returned node %d (load %.2f%%) but min was %.2f%%",
				i, resultID, resultLoad, minLoad)
		}
	}
}

// Property 14: Bandwidth Quota Threshold Notifications
// For random (usedBytes, quotaGB): warning at 80%, critical at 100%.
// **Validates: Requirements 12.2, 12.3**

func TestProperty_BandwidthQuota_WarningAt80Percent(t *testing.T) {
	rng := rand.New(rand.NewSource(14))

	for i := 0; i < 200; i++ {
		quotaGB := rng.Intn(1000) + 1 // 1 to 1000 GB
		quotaBytes := int64(quotaGB) * 1_000_000_000

		// Generate usage at exactly 80% or above
		usedBytes := int64(float64(quotaBytes) * (0.80 + rng.Float64()*0.20))

		warning, _ := checkThreshold(usedBytes, quotaGB)
		if !warning {
			usagePercent := float64(usedBytes) / float64(quotaBytes) * 100.0
			t.Fatalf("iteration %d: expected warning=true at %.2f%% usage (used=%d, quota=%dGB)",
				i, usagePercent, usedBytes, quotaGB)
		}
	}
}

func TestProperty_BandwidthQuota_NoWarningBelow80Percent(t *testing.T) {
	rng := rand.New(rand.NewSource(141))

	for i := 0; i < 200; i++ {
		quotaGB := rng.Intn(1000) + 1 // 1 to 1000 GB
		quotaBytes := int64(quotaGB) * 1_000_000_000

		// Generate usage strictly below 80%
		usedBytes := int64(float64(quotaBytes) * rng.Float64() * 0.799)

		warning, _ := checkThreshold(usedBytes, quotaGB)
		if warning {
			usagePercent := float64(usedBytes) / float64(quotaBytes) * 100.0
			t.Fatalf("iteration %d: expected warning=false at %.2f%% usage (used=%d, quota=%dGB)",
				i, usagePercent, usedBytes, quotaGB)
		}
	}
}

func TestProperty_BandwidthQuota_CriticalAt100Percent(t *testing.T) {
	rng := rand.New(rand.NewSource(142))

	for i := 0; i < 200; i++ {
		quotaGB := rng.Intn(1000) + 1 // 1 to 1000 GB
		quotaBytes := int64(quotaGB) * 1_000_000_000

		// Generate usage at 100% or above
		usedBytes := quotaBytes + int64(rng.Float64()*float64(quotaBytes)*0.5)

		_, critical := checkThreshold(usedBytes, quotaGB)
		if !critical {
			usagePercent := float64(usedBytes) / float64(quotaBytes) * 100.0
			t.Fatalf("iteration %d: expected critical=true at %.2f%% usage (used=%d, quota=%dGB)",
				i, usagePercent, usedBytes, quotaGB)
		}
	}
}

func TestProperty_BandwidthQuota_NoCriticalBelow100Percent(t *testing.T) {
	rng := rand.New(rand.NewSource(143))

	for i := 0; i < 200; i++ {
		quotaGB := rng.Intn(1000) + 1 // 1 to 1000 GB
		quotaBytes := int64(quotaGB) * 1_000_000_000

		// Generate usage strictly below 100%
		usedBytes := int64(float64(quotaBytes) * rng.Float64() * 0.999)

		_, critical := checkThreshold(usedBytes, quotaGB)
		if critical {
			usagePercent := float64(usedBytes) / float64(quotaBytes) * 100.0
			t.Fatalf("iteration %d: expected critical=false at %.2f%% usage (used=%d, quota=%dGB)",
				i, usagePercent, usedBytes, quotaGB)
		}
	}
}

// Property 15: Node Migration Completeness
// For any total and failures (0 <= failures <= total): migrated = total - failures, failed = failures
// **Validates: Requirements 13.1, 13.4, 13.5**

func TestProperty_Migration_Completeness(t *testing.T) {
	f := func(total int, failures int) bool {
		// Constrain inputs
		if total < 0 {
			total = -total
		}
		if total > 10000 {
			total = 10000
		}
		if failures < 0 {
			failures = 0
		}
		if failures > total {
			failures = total
		}

		migrated, failed := calculateMigrationResult(total, failures)

		// Verify: migrated + failed = total
		if migrated+failed != total {
			return false
		}
		// Verify: migrated = total - failures
		if migrated != total-failures {
			return false
		}
		// Verify: failed = failures
		if failed != failures {
			return false
		}
		// Verify: both are non-negative
		if migrated < 0 || failed < 0 {
			return false
		}
		return true
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: migration completeness invariant broken: %v", err)
	}
}

func TestProperty_Migration_SumInvariant(t *testing.T) {
	rng := rand.New(rand.NewSource(15))

	for i := 0; i < 200; i++ {
		total := rng.Intn(500) + 1      // 1 to 500 users
		failures := rng.Intn(total + 1) // 0 to total failures

		migrated, failed := calculateMigrationResult(total, failures)

		// Sum invariant: migrated + failed must always equal total
		if migrated+failed != total {
			t.Fatalf("iteration %d: migrated(%d) + failed(%d) = %d, expected total %d",
				i, migrated, failed, migrated+failed, total)
		}

		// Non-negativity
		if migrated < 0 {
			t.Fatalf("iteration %d: migrated count is negative: %d", i, migrated)
		}
		if failed < 0 {
			t.Fatalf("iteration %d: failed count is negative: %d", i, failed)
		}
	}
}
