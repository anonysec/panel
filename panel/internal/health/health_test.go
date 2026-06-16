package health

import "testing"

// --- ComputeScore tests ---

func TestComputeScore_AllHealthy(t *testing.T) {
	results := []CheckResult{
		{Category: "Database", Severity: SeverityHealthy},
		{Category: "NodeOnline", Severity: SeverityHealthy},
		{Category: "VPNService", Severity: SeverityHealthy},
		{Category: "ResourceUsage", Severity: SeverityHealthy},
		{Category: "StaleSessions", Severity: SeverityHealthy},
		{Category: "ExpiredSubscriptions", Severity: SeverityHealthy},
		{Category: "DNSFailover", Severity: SeverityHealthy},
	}
	score := ComputeScore(results, DefaultWeights)
	if score != 100 {
		t.Errorf("all healthy: expected 100, got %d", score)
	}
}

func TestComputeScore_AllCritical(t *testing.T) {
	results := []CheckResult{
		{Category: "Database", Severity: SeverityCritical},
		{Category: "NodeOnline", Severity: SeverityCritical},
		{Category: "VPNService", Severity: SeverityCritical},
		{Category: "ResourceUsage", Severity: SeverityCritical},
		{Category: "StaleSessions", Severity: SeverityCritical},
		{Category: "ExpiredSubscriptions", Severity: SeverityCritical},
		{Category: "DNSFailover", Severity: SeverityCritical},
	}
	score := ComputeScore(results, DefaultWeights)
	// 100 - (20+20+15+15+10+10+10)*1.0 = 100 - 100 = 0
	if score != 0 {
		t.Errorf("all critical: expected 0, got %d", score)
	}
}

func TestComputeScore_MixedSeverity(t *testing.T) {
	results := []CheckResult{
		{Category: "Database", Severity: SeverityHealthy},
		{Category: "NodeOnline", Severity: SeverityWarning},
		{Category: "VPNService", Severity: SeverityCritical},
	}
	score := ComputeScore(results, DefaultWeights)
	// 100 - (0 + 20*0.5 + 15*1.0) = 100 - 10 - 15 = 75
	if score != 75 {
		t.Errorf("mixed: expected 75, got %d", score)
	}
}

func TestComputeScore_EmptyResults(t *testing.T) {
	score := ComputeScore(nil, DefaultWeights)
	if score != 100 {
		t.Errorf("empty results: expected 100, got %d", score)
	}
}

func TestComputeScore_UnknownCategory(t *testing.T) {
	results := []CheckResult{
		{Category: "Unknown", Severity: SeverityCritical},
	}
	score := ComputeScore(results, DefaultWeights)
	// Unknown category has no weight, so no penalty
	if score != 100 {
		t.Errorf("unknown category: expected 100, got %d", score)
	}
}

func TestComputeScore_ClampsToZero(t *testing.T) {
	// Use weights that sum to more than 100 to test clamping
	bigWeights := map[string]float64{
		"Database":   60,
		"NodeOnline": 60,
	}
	results := []CheckResult{
		{Category: "Database", Severity: SeverityCritical},
		{Category: "NodeOnline", Severity: SeverityCritical},
	}
	score := ComputeScore(results, bigWeights)
	// 100 - 120 = -20 -> clamped to 0
	if score != 0 {
		t.Errorf("clamp to zero: expected 0, got %d", score)
	}
}

// --- ClassifySeverity tests ---

func TestClassifySeverity_Healthy(t *testing.T) {
	sev := ClassifySeverity(30.0, 70.0, 90.0)
	if sev != SeverityHealthy {
		t.Errorf("expected healthy, got %s", sev)
	}
}

func TestClassifySeverity_Warning(t *testing.T) {
	sev := ClassifySeverity(75.0, 70.0, 90.0)
	if sev != SeverityWarning {
		t.Errorf("expected warning, got %s", sev)
	}
}

func TestClassifySeverity_Critical(t *testing.T) {
	sev := ClassifySeverity(95.0, 70.0, 90.0)
	if sev != SeverityCritical {
		t.Errorf("expected critical, got %s", sev)
	}
}

func TestClassifySeverity_AtWarningThreshold(t *testing.T) {
	sev := ClassifySeverity(70.0, 70.0, 90.0)
	if sev != SeverityWarning {
		t.Errorf("at warning threshold: expected warning, got %s", sev)
	}
}

func TestClassifySeverity_AtCriticalThreshold(t *testing.T) {
	sev := ClassifySeverity(90.0, 70.0, 90.0)
	if sev != SeverityCritical {
		t.Errorf("at critical threshold: expected critical, got %s", sev)
	}
}

func TestClassifySeverity_JustBelowWarning(t *testing.T) {
	sev := ClassifySeverity(69.9, 70.0, 90.0)
	if sev != SeverityHealthy {
		t.Errorf("just below warning: expected healthy, got %s", sev)
	}
}

// --- ComputeTrend tests ---

func TestComputeTrend_Improving(t *testing.T) {
	// avg = 50, current = 60 -> 60 > 55 -> improving
	trend := ComputeTrend(60, []int{50, 50, 50})
	if trend != "improving" {
		t.Errorf("expected improving, got %s", trend)
	}
}

func TestComputeTrend_Degrading(t *testing.T) {
	// avg = 80, current = 70 -> 70 < 75 -> degrading
	trend := ComputeTrend(70, []int{80, 80, 80})
	if trend != "degrading" {
		t.Errorf("expected degrading, got %s", trend)
	}
}

func TestComputeTrend_Stable(t *testing.T) {
	// avg = 75, current = 77 -> 77 <= 80 and 77 >= 70 -> stable
	trend := ComputeTrend(77, []int{75, 75, 75})
	if trend != "stable" {
		t.Errorf("expected stable, got %s", trend)
	}
}

func TestComputeTrend_EmptyHistory(t *testing.T) {
	trend := ComputeTrend(50, nil)
	if trend != "stable" {
		t.Errorf("empty history: expected stable, got %s", trend)
	}
}

func TestComputeTrend_ExactlyAtBoundary(t *testing.T) {
	// avg = 50, current = 55 -> 55 <= 55 -> stable (not improving, needs > avg+5)
	trend := ComputeTrend(55, []int{50, 50, 50})
	if trend != "stable" {
		t.Errorf("at +5 boundary: expected stable, got %s", trend)
	}

	// avg = 50, current = 45 -> 45 >= 45 -> stable (not degrading, needs < avg-5)
	trend = ComputeTrend(45, []int{50, 50, 50})
	if trend != "stable" {
		t.Errorf("at -5 boundary: expected stable, got %s", trend)
	}
}

func TestComputeTrend_SingleHistoryScore(t *testing.T) {
	// avg = 80, current = 90 -> 90 > 85 -> improving
	trend := ComputeTrend(90, []int{80})
	if trend != "improving" {
		t.Errorf("single history improving: expected improving, got %s", trend)
	}
}
