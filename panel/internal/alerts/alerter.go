package alerts

import "time"

// AlertType identifies the kind of alert.
type AlertType string

const (
	AlertHighCPU      AlertType = "high_cpu"
	AlertHighRAM      AlertType = "high_ram"
	AlertHighDisk     AlertType = "high_disk"
	AlertNodeDown     AlertType = "node_down"
	AlertNodeDegraded AlertType = "node_degraded"
)

// Alert represents a single alert event emitted by the alerting system.
type Alert struct {
	Type      AlertType `json:"type"`
	NodeID    int64     `json:"node_id"`
	Message   string    `json:"message"`
	Value     float64   `json:"value"`
	Threshold float64   `json:"threshold"`
	Timestamp time.Time `json:"timestamp"`
}

// Thresholds holds the configured alert thresholds (percent values).
// An alert is emitted when a metric strictly exceeds its threshold.
type Thresholds struct {
	CPUPercent  float64 // default: 90
	RAMPercent  float64 // default: 85
	DiskPercent float64 // default: 90
}

// DefaultThresholds returns the default alert thresholds.
func DefaultThresholds() Thresholds {
	return Thresholds{
		CPUPercent:  90,
		RAMPercent:  85,
		DiskPercent: 90,
	}
}

// CheckMetrics evaluates metric values against the configured thresholds.
// An alert is emitted if and only if the metric value strictly exceeds the
// configured threshold (Property 11).
//
// Parameters are raw metric values to avoid circular import with grpcclient.
func CheckMetrics(nodeID int64, cpu, ram, disk float64, thresholds Thresholds) []Alert {
	now := time.Now()
	var alerts []Alert

	if cpu > thresholds.CPUPercent {
		alerts = append(alerts, Alert{
			Type:      AlertHighCPU,
			NodeID:    nodeID,
			Message:   "CPU usage exceeds threshold",
			Value:     cpu,
			Threshold: thresholds.CPUPercent,
			Timestamp: now,
		})
	}

	if ram > thresholds.RAMPercent {
		alerts = append(alerts, Alert{
			Type:      AlertHighRAM,
			NodeID:    nodeID,
			Message:   "RAM usage exceeds threshold",
			Value:     ram,
			Threshold: thresholds.RAMPercent,
			Timestamp: now,
		})
	}

	if disk > thresholds.DiskPercent {
		alerts = append(alerts, Alert{
			Type:      AlertHighDisk,
			NodeID:    nodeID,
			Message:   "Disk usage exceeds threshold",
			Value:     disk,
			Threshold: thresholds.DiskPercent,
			Timestamp: now,
		})
	}

	return alerts
}

// CheckStatusTransition determines if a node status change warrants an alert.
// A node-down alert is emitted if and only if the previous status is "online"
// and the new status is "stale" or "offline" (Property 12).
//
// Parameters use string to avoid importing grpcclient.NodeStatus and prevent
// circular dependencies.
func CheckStatusTransition(nodeID int64, old, new string) *Alert {
	if old != "online" {
		return nil
	}

	if new == "stale" || new == "offline" {
		return &Alert{
			Type:      AlertNodeDown,
			NodeID:    nodeID,
			Message:   "Node went down (was online, now " + new + ")",
			Value:     0,
			Threshold: 0,
			Timestamp: time.Now(),
		}
	}

	return nil
}
