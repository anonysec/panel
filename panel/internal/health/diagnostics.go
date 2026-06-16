package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"time"

	"koris-next/panel/internal/notify"
)

// DiagnosticsEngine orchestrates health checks, computes scores,
// tracks trends, and runs root-cause analysis.
type DiagnosticsEngine struct {
	db       *sql.DB
	analyzer Analyzer
	notifier *notify.Notifier
	checks   []HealthCheck
}

// HealthScoreRecord represents a historical health score stored in the database.
type HealthScoreRecord struct {
	ID          int64           `json:"id"`
	Score       int             `json:"score"`
	Trend       string          `json:"trend"`
	ChecksJSON  json.RawMessage `json:"checks_json"`
	GeneratedAt time.Time       `json:"generated_at"`
}

// NewDiagnosticsEngine creates the engine with all registered checks.
func NewDiagnosticsEngine(db *sql.DB, analyzer Analyzer, notifier *notify.Notifier) *DiagnosticsEngine {
	return &DiagnosticsEngine{
		db:       db,
		analyzer: analyzer,
		notifier: notifier,
		checks: []HealthCheck{
			&DatabaseCheck{},
			&NodeOnlineCheck{},
			&VPNServiceCheck{},
			&DiskUsageCheck{},
			&MemoryUsageCheck{},
			&CPUUsageCheck{},
			&StaleSessionCheck{},
			&ExpiredSubscriptionCheck{},
			&DNSFailoverCheck{},
		},
	}
}

// RunAll executes all health checks and produces a full HealthReport.
// Each check is given a 5-second timeout. After running all checks,
// it computes the score, determines the trend from historical data,
// runs root-cause analysis on non-healthy checks, and persists the result.
func (de *DiagnosticsEngine) RunAll(ctx context.Context) (*HealthReport, error) {
	var results []CheckResult

	for _, check := range de.checks {
		// Each check gets a 5-second timeout
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		result := check.Run(checkCtx, de.db)
		cancel()

		// If context was cancelled (timed out), mark as critical
		if checkCtx.Err() != nil {
			result = CheckResult{
				Name:             check.Name(),
				Category:         check.Category(),
				Severity:         SeverityCritical,
				Message:          "Check timed out",
				SuggestedActions: []string{"Investigate why " + check.Name() + " is slow"},
			}
		}

		results = append(results, result)
	}

	// Compute the health score using default weights
	score := ComputeScore(results, DefaultWeights)

	// Get historical scores for trend computation
	historicalScores, err := de.getRecentScores(ctx, 24*time.Hour)
	if err != nil {
		log.Printf("[health] failed to get historical scores for trend: %v", err)
		historicalScores = nil
	}

	trend := ComputeTrend(score, historicalScores)

	// Build the report
	report := &HealthReport{
		Score:       score,
		Trend:       trend,
		Checks:      results,
		GeneratedAt: time.Now().UTC(),
	}

	// Run root-cause analysis on non-healthy checks
	var nonHealthy []CheckResult
	for _, r := range results {
		if r.Severity != SeverityHealthy {
			nonHealthy = append(nonHealthy, r)
		}
	}

	if len(nonHealthy) > 0 && de.analyzer != nil {
		analysis, err := de.analyzer.Analyze(AnalysisInput{
			CheckResults: nonHealthy,
		})
		if err != nil {
			log.Printf("[health] analyzer error: %v", err)
			// Report is still valid without RCA
		} else {
			report.RootCauseAnalysis = &analysis
		}
	}

	// Persist the score to the database
	if err := de.PersistScore(ctx, report); err != nil {
		log.Printf("[health] failed to persist score: %v", err)
	}

	return report, nil
}

// PersistScore saves the health report score, trend, and checks to the health_scores table.
func (de *DiagnosticsEngine) PersistScore(ctx context.Context, report *HealthReport) error {
	checksJSON, err := json.Marshal(report.Checks)
	if err != nil {
		return err
	}

	_, err = de.db.ExecContext(ctx,
		`INSERT INTO health_scores (score, trend, checks_json, generated_at) VALUES (?, ?, ?, ?)`,
		report.Score, report.Trend, string(checksJSON), report.GeneratedAt,
	)
	return err
}

// GetHistory retrieves historical health scores within the given time range.
func (de *DiagnosticsEngine) GetHistory(ctx context.Context, from, to time.Time) ([]HealthScoreRecord, error) {
	rows, err := de.db.QueryContext(ctx,
		`SELECT id, score, trend, checks_json, generated_at FROM health_scores WHERE generated_at >= ? AND generated_at <= ? ORDER BY generated_at DESC LIMIT 1000`,
		from, to,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []HealthScoreRecord
	for rows.Next() {
		var r HealthScoreRecord
		var genAt sql.NullTime
		if err := rows.Scan(&r.ID, &r.Score, &r.Trend, &r.ChecksJSON, &genAt); err != nil {
			return nil, err
		}
		if genAt.Valid {
			r.GeneratedAt = genAt.Time
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// getRecentScores retrieves score values from the last `duration` for trend computation.
func (de *DiagnosticsEngine) getRecentScores(ctx context.Context, duration time.Duration) ([]int, error) {
	since := time.Now().Add(-duration)
	rows, err := de.db.QueryContext(ctx,
		`SELECT score FROM health_scores WHERE generated_at >= ? ORDER BY generated_at ASC`,
		since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var scores []int
	for rows.Next() {
		var s int
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		scores = append(scores, s)
	}
	return scores, rows.Err()
}
