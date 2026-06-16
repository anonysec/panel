// Package health provides core types, interfaces, and computation functions
// for the AI Health Monitor system.
package health

import (
	"os"
	"time"
)

// Severity represents the health status of a check.
type Severity string

const (
	SeverityHealthy  Severity = "healthy"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Analyzer defines the interface for root-cause analysis and suggestion generation.
// The default implementation uses rule-based heuristics. An LLM-backed implementation
// can be swapped in via PANEL_LLM_ENDPOINT environment variable.
type Analyzer interface {
	// Analyze takes structured health data and returns analysis results.
	Analyze(input AnalysisInput) (AnalysisOutput, error)
}

// AnalysisInput is the structured input to the Analyzer.
type AnalysisInput struct {
	CheckResults []CheckResult  `json:"check_results"`
	EventHistory []AnomalyEvent `json:"event_history,omitempty"`
}

// AnalysisOutput is the structured output from the Analyzer.
type AnalysisOutput struct {
	RootCause          string   `json:"root_cause"`
	Confidence         float64  `json:"confidence"`           // 0.0 - 1.0
	SuggestedActions   []string `json:"suggested_actions"`
	AffectedComponents []string `json:"affected_components"`
}

// CheckResult represents the output of a single health check.
type CheckResult struct {
	Name             string         `json:"name"`
	Category         string         `json:"category"`
	Severity         Severity       `json:"severity"`
	Message          string         `json:"message"`
	Value            float64        `json:"value,omitempty"`
	Threshold        float64        `json:"threshold,omitempty"`
	SuggestedActions []string       `json:"suggested_actions,omitempty"`
	NodeID           *int64         `json:"node_id,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

// HealthReport is the full diagnostics response.
type HealthReport struct {
	Score             int             `json:"score"`    // 0-100
	Trend             string          `json:"trend"`    // "improving", "stable", "degrading"
	Checks            []CheckResult   `json:"checks"`
	RootCauseAnalysis *AnalysisOutput `json:"root_cause_analysis,omitempty"`
	GeneratedAt       time.Time       `json:"generated_at"`
}

// HealingRule represents a configurable auto-healing policy.
type HealingRule struct {
	ID              int64          `json:"id"`
	RuleKey         string         `json:"rule_key"`
	DisplayName     string         `json:"display_name"`
	ConditionType   string         `json:"condition_type"`
	ActionMode      string         `json:"action_mode"` // "auto_fix" or "alert_only"
	CooldownSeconds int            `json:"cooldown_seconds"`
	Enabled         bool           `json:"enabled"`
	ThresholdsJSON  map[string]any `json:"thresholds_json,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// HealingAction represents a recorded auto-healing action.
type HealingAction struct {
	ID              int64     `json:"id"`
	RuleKey         string    `json:"rule_key"`
	ResourceType    string    `json:"resource_type"`
	ResourceID      string    `json:"resource_id"`
	ActionPerformed string    `json:"action_performed"`
	ResultStatus    string    `json:"result_status"` // "success", "partial", "failure"
	ErrorMessage    string    `json:"error_message,omitempty"`
	ExecutionMs     int       `json:"execution_ms"`
	CreatedAt       time.Time `json:"created_at"`
}

// AnomalyEvent represents a detected statistical anomaly.
type AnomalyEvent struct {
	ID                    int64          `json:"id"`
	AnomalyType           string         `json:"anomaly_type"`
	DetectedValue         float64        `json:"detected_value"`
	BaselineValue         float64        `json:"baseline_value"`
	Severity              Severity       `json:"severity"`
	MetadataJSON          map[string]any `json:"metadata_json,omitempty"`
	CorrelatedIncidentID  *int64         `json:"correlated_incident_id,omitempty"`
	CreatedAt             time.Time      `json:"created_at"`
}

// TriggeredCondition represents a detected issue ready for remediation.
type TriggeredCondition struct {
	RuleID        string         `json:"rule_id"`
	ConditionType string         `json:"condition_type"`
	ResourceType  string         `json:"resource_type"`
	ResourceID    string         `json:"resource_id"`
	Details       map[string]any `json:"details,omitempty"`
}

// DefaultWeights defines the default weight configuration for health score computation.
var DefaultWeights = map[string]float64{
	"Database":             20,
	"NodeOnline":           20,
	"VPNService":           15,
	"ResourceUsage":        15,
	"StaleSessions":        10,
	"ExpiredSubscriptions": 10,
	"DNSFailover":          10,
}

// ComputeScore calculates a weighted health score from check results.
// Formula: score = 100 - sum(weight * severity_factor)
// where severity_factor: healthy=0, warning=0.5, critical=1.0
// The result is clamped to [0, 100].
func ComputeScore(results []CheckResult, weights map[string]float64) int {
	totalPenalty := 0.0
	for _, r := range results {
		weight, ok := weights[r.Category]
		if !ok {
			continue
		}
		var factor float64
		switch r.Severity {
		case SeverityHealthy:
			factor = 0.0
		case SeverityWarning:
			factor = 0.5
		case SeverityCritical:
			factor = 1.0
		default:
			factor = 0.0
		}
		totalPenalty += weight * factor
	}

	score := 100.0 - totalPenalty

	// Clamp to [0, 100]
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return int(score)
}

// ClassifySeverity assigns severity based on value and thresholds.
// value < warningThreshold -> healthy
// warningThreshold <= value < criticalThreshold -> warning
// value >= criticalThreshold -> critical
func ClassifySeverity(value, warningThreshold, criticalThreshold float64) Severity {
	if value < warningThreshold {
		return SeverityHealthy
	}
	if value < criticalThreshold {
		return SeverityWarning
	}
	return SeverityCritical
}

// ComputeTrend compares the current score to the average of historical scores.
// Returns "improving" if current > avg + 5, "degrading" if current < avg - 5, else "stable".
// If historicalScores is empty, returns "stable".
func ComputeTrend(currentScore int, historicalScores []int) string {
	if len(historicalScores) == 0 {
		return "stable"
	}

	sum := 0
	for _, s := range historicalScores {
		sum += s
	}
	avg := float64(sum) / float64(len(historicalScores))

	current := float64(currentScore)
	if current > avg+5 {
		return "improving"
	}
	if current < avg-5 {
		return "degrading"
	}
	return "stable"
}


// NewAnalyzer returns the appropriate Analyzer based on configuration.
// If PANEL_LLM_ENDPOINT is set, it would return an LLM-based analyzer (future);
// otherwise it returns the default RuleBasedAnalyzer.
func NewAnalyzer() Analyzer {
	if endpoint := os.Getenv("PANEL_LLM_ENDPOINT"); endpoint != "" {
		// Future: return NewLLMAnalyzer(endpoint)
		_ = endpoint
	}
	return &RuleBasedAnalyzer{}
}
