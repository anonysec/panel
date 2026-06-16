package health

import (
	"context"
	"database/sql"
	"time"
)

// HealthCheck is a single probe function that evaluates one aspect of system health.
type HealthCheck interface {
	Name() string
	Category() string
	Run(ctx context.Context, db *sql.DB) CheckResult
}

// --- 1. DatabaseCheck ---

// DatabaseCheck verifies basic database connectivity with SELECT 1.
type DatabaseCheck struct{}

func (c *DatabaseCheck) Name() string     { return "database_connectivity" }
func (c *DatabaseCheck) Category() string  { return "database" }

func (c *DatabaseCheck) Run(ctx context.Context, db *sql.DB) CheckResult {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var one int
	err := db.QueryRowContext(ctx, "SELECT 1").Scan(&one)
	if err != nil {
		return CheckResult{
			Name:             c.Name(),
			Category:         c.Category(),
			Severity:         SeverityCritical,
			Message:          "Database unreachable",
			SuggestedActions: []string{"Check database server status", "Verify database connection settings"},
		}
	}
	return CheckResult{
		Name:     c.Name(),
		Category: c.Category(),
		Severity: SeverityHealthy,
		Message:  "Database is responsive",
	}
}

// --- 2. NodeOnlineCheck ---

// NodeOnlineCheck counts nodes that are not disabled but have stale status updates.
type NodeOnlineCheck struct{}

func (c *NodeOnlineCheck) Name() string     { return "node_online_status" }
func (c *NodeOnlineCheck) Category() string  { return "nodes" }

func (c *NodeOnlineCheck) Run(ctx context.Context, db *sql.DB) CheckResult {
	query := `SELECT COUNT(*) FROM nodes WHERE status NOT IN ('disabled') AND (id NOT IN (SELECT node_id FROM node_status WHERE updated_at > NOW() - INTERVAL 5 MINUTE))`

	var count int
	err := db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return CheckResult{
			Name:             c.Name(),
			Category:         c.Category(),
			Severity:         SeverityCritical,
			Message:          "Failed to query node status",
			SuggestedActions: []string{"Check node connectivity"},
		}
	}

	severity := ClassifySeverity(float64(count), 1, 3)
	result := CheckResult{
		Name:     c.Name(),
		Category: c.Category(),
		Severity: severity,
		Message:  "Nodes with stale status",
		Value:    float64(count),
	}
	if severity != SeverityHealthy {
		result.SuggestedActions = []string{"Check node connectivity"}
	}
	return result
}

// --- 3. VPNServiceCheck ---

// VPNServiceCheck queries node_status for VPN service health across all active nodes.
type VPNServiceCheck struct{}

func (c *VPNServiceCheck) Name() string     { return "vpn_service_health" }
func (c *VPNServiceCheck) Category() string  { return "vpn" }

func (c *VPNServiceCheck) Run(ctx context.Context, db *sql.DB) CheckResult {
	query := `SELECT openvpn_status, l2tp_status, ikev2_status FROM node_status`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return CheckResult{
			Name:             c.Name(),
			Category:         c.Category(),
			Severity:         SeverityCritical,
			Message:          "Failed to query VPN service status",
			SuggestedActions: []string{"Restart affected VPN services via node tasks"},
		}
	}
	defer rows.Close()

	stoppedCount := 0
	for rows.Next() {
		var openvpn, l2tp, ikev2 string
		if err := rows.Scan(&openvpn, &l2tp, &ikev2); err != nil {
			continue
		}
		if openvpn == "stopped" || openvpn == "failed" {
			stoppedCount++
		}
		if l2tp == "stopped" || l2tp == "failed" {
			stoppedCount++
		}
		if ikev2 == "stopped" || ikev2 == "failed" {
			stoppedCount++
		}
	}

	severity := ClassifySeverity(float64(stoppedCount), 1, 2)
	result := CheckResult{
		Name:     c.Name(),
		Category: c.Category(),
		Severity: severity,
		Message:  "VPN services not running",
		Value:    float64(stoppedCount),
	}
	if severity != SeverityHealthy {
		result.SuggestedActions = []string{"Restart affected VPN services via node tasks"}
	}
	return result
}

// --- 4. DiskUsageCheck ---

// DiskUsageCheck queries the maximum disk usage percentage across all nodes.
type DiskUsageCheck struct{}

func (c *DiskUsageCheck) Name() string     { return "disk_usage" }
func (c *DiskUsageCheck) Category() string  { return "resources" }

func (c *DiskUsageCheck) Run(ctx context.Context, db *sql.DB) CheckResult {
	query := `SELECT MAX(disk_percent) FROM node_status`

	var maxDisk sql.NullFloat64
	err := db.QueryRowContext(ctx, query).Scan(&maxDisk)
	if err != nil || !maxDisk.Valid {
		return CheckResult{
			Name:     c.Name(),
			Category: c.Category(),
			Severity: SeverityHealthy,
			Message:  "No disk usage data available",
		}
	}

	severity := ClassifySeverity(maxDisk.Float64, 80, 90)
	result := CheckResult{
		Name:      c.Name(),
		Category:  c.Category(),
		Severity:  severity,
		Message:   "Maximum disk usage across nodes",
		Value:     maxDisk.Float64,
		Threshold: 80,
	}
	if severity != SeverityHealthy {
		result.SuggestedActions = []string{"Clean up logs and old backups"}
	}
	return result
}

// --- 5. MemoryUsageCheck ---

// MemoryUsageCheck queries the maximum memory usage percentage across all nodes.
type MemoryUsageCheck struct{}

func (c *MemoryUsageCheck) Name() string     { return "memory_usage" }
func (c *MemoryUsageCheck) Category() string  { return "resources" }

func (c *MemoryUsageCheck) Run(ctx context.Context, db *sql.DB) CheckResult {
	query := `SELECT MAX(ram_percent) FROM node_status`

	var maxRAM sql.NullFloat64
	err := db.QueryRowContext(ctx, query).Scan(&maxRAM)
	if err != nil || !maxRAM.Valid {
		return CheckResult{
			Name:     c.Name(),
			Category: c.Category(),
			Severity: SeverityHealthy,
			Message:  "No memory usage data available",
		}
	}

	severity := ClassifySeverity(maxRAM.Float64, 85, 95)
	result := CheckResult{
		Name:      c.Name(),
		Category:  c.Category(),
		Severity:  severity,
		Message:   "Maximum memory usage across nodes",
		Value:     maxRAM.Float64,
		Threshold: 85,
	}
	if severity != SeverityHealthy {
		result.SuggestedActions = []string{"Investigate high memory processes", "Consider adding more RAM or scaling horizontally"}
	}
	return result
}

// --- 6. CPUUsageCheck ---

// CPUUsageCheck queries the maximum CPU usage percentage across all nodes.
type CPUUsageCheck struct{}

func (c *CPUUsageCheck) Name() string     { return "cpu_usage" }
func (c *CPUUsageCheck) Category() string  { return "resources" }

func (c *CPUUsageCheck) Run(ctx context.Context, db *sql.DB) CheckResult {
	query := `SELECT MAX(cpu_percent) FROM node_status`

	var maxCPU sql.NullFloat64
	err := db.QueryRowContext(ctx, query).Scan(&maxCPU)
	if err != nil || !maxCPU.Valid {
		return CheckResult{
			Name:     c.Name(),
			Category: c.Category(),
			Severity: SeverityHealthy,
			Message:  "No CPU usage data available",
		}
	}

	severity := ClassifySeverity(maxCPU.Float64, 80, 95)
	result := CheckResult{
		Name:      c.Name(),
		Category:  c.Category(),
		Severity:  severity,
		Message:   "Maximum CPU usage across nodes",
		Value:     maxCPU.Float64,
		Threshold: 80,
	}
	if severity != SeverityHealthy {
		result.SuggestedActions = []string{"Investigate high CPU processes", "Consider load balancing or scaling"}
	}
	return result
}

// --- 7. StaleSessionCheck ---

// StaleSessionCheck counts RADIUS sessions with no stop time that haven't been updated recently.
type StaleSessionCheck struct{}

func (c *StaleSessionCheck) Name() string     { return "stale_sessions" }
func (c *StaleSessionCheck) Category() string  { return "sessions" }

func (c *StaleSessionCheck) Run(ctx context.Context, db *sql.DB) CheckResult {
	query := `SELECT COUNT(*) FROM radacct WHERE acctstoptime IS NULL AND acctupdatetime < NOW() - INTERVAL 5 MINUTE`

	var count int
	err := db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return CheckResult{
			Name:             c.Name(),
			Category:         c.Category(),
			Severity:         SeverityWarning,
			Message:          "Failed to query stale sessions",
			SuggestedActions: []string{"Clear stale sessions"},
		}
	}

	severity := ClassifySeverity(float64(count), 1, 10)
	result := CheckResult{
		Name:     c.Name(),
		Category: c.Category(),
		Severity: severity,
		Message:  "Stale RADIUS sessions detected",
		Value:    float64(count),
	}
	if severity != SeverityHealthy {
		result.SuggestedActions = []string{"Clear stale sessions"}
	}
	return result
}

// --- 8. ExpiredSubscriptionCheck ---

// ExpiredSubscriptionCheck counts customers with expired status.
type ExpiredSubscriptionCheck struct{}

func (c *ExpiredSubscriptionCheck) Name() string     { return "expired_subscriptions" }
func (c *ExpiredSubscriptionCheck) Category() string  { return "subscriptions" }

func (c *ExpiredSubscriptionCheck) Run(ctx context.Context, db *sql.DB) CheckResult {
	query := `SELECT COUNT(*) FROM customers WHERE status='expired' AND deleted_at IS NULL`

	var count int
	err := db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return CheckResult{
			Name:     c.Name(),
			Category: c.Category(),
			Severity: SeverityWarning,
			Message:  "Failed to query expired subscriptions",
		}
	}

	severity := ClassifySeverity(float64(count), 1, 5)
	result := CheckResult{
		Name:     c.Name(),
		Category: c.Category(),
		Severity: severity,
		Message:  "Expired customer subscriptions",
		Value:    float64(count),
	}
	if severity != SeverityHealthy {
		result.SuggestedActions = []string{"Review expired accounts", "Send renewal reminders"}
	}
	return result
}

// --- 9. DNSFailoverCheck ---

// DNSFailoverCheck counts recent failover events that are still pending, propagating, or failed.
type DNSFailoverCheck struct{}

func (c *DNSFailoverCheck) Name() string     { return "dns_failover_status" }
func (c *DNSFailoverCheck) Category() string  { return "failover" }

func (c *DNSFailoverCheck) Run(ctx context.Context, db *sql.DB) CheckResult {
	query := `SELECT COUNT(*) FROM failover_events WHERE status IN ('pending','propagating','failed') AND created_at > NOW() - INTERVAL 1 HOUR`

	var count int
	err := db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return CheckResult{
			Name:     c.Name(),
			Category: c.Category(),
			Severity: SeverityWarning,
			Message:  "Failed to query failover events",
		}
	}

	severity := ClassifySeverity(float64(count), 1, 2)
	result := CheckResult{
		Name:     c.Name(),
		Category: c.Category(),
		Severity: severity,
		Message:  "Active failover events in last hour",
		Value:    float64(count),
	}
	if severity != SeverityHealthy {
		result.SuggestedActions = []string{"Check DNS propagation status", "Verify failover target nodes are healthy"}
	}
	return result
}
