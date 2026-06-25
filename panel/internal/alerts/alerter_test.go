package alerts

import (
	"testing"
)

func TestCheckMetrics_NoAlerts(t *testing.T) {
	tests := []struct {
		name string
		cpu  float64
		ram  float64
		disk float64
	}{
		{"all below threshold", 50, 50, 50},
		{"exactly at threshold", 90, 85, 90},
		{"all zero", 0, 0, 0},
		{"cpu at threshold", 90, 0, 0},
		{"ram at threshold", 0, 85, 0},
		{"disk at threshold", 0, 0, 90},
	}

	thresholds := DefaultThresholds()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alerts := CheckMetrics(1, tt.cpu, tt.ram, tt.disk, thresholds)
			if len(alerts) != 0 {
				t.Errorf("expected no alerts, got %d: %+v", len(alerts), alerts)
			}
		})
	}
}

func TestCheckMetrics_CPUAlert(t *testing.T) {
	thresholds := DefaultThresholds()
	alerts := CheckMetrics(42, 91, 50, 50, thresholds)

	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Type != AlertHighCPU {
		t.Errorf("expected AlertHighCPU, got %s", alerts[0].Type)
	}
	if alerts[0].NodeID != 42 {
		t.Errorf("expected NodeID 42, got %d", alerts[0].NodeID)
	}
	if alerts[0].Value != 91 {
		t.Errorf("expected Value 91, got %f", alerts[0].Value)
	}
	if alerts[0].Threshold != 90 {
		t.Errorf("expected Threshold 90, got %f", alerts[0].Threshold)
	}
}

func TestCheckMetrics_RAMAlert(t *testing.T) {
	thresholds := DefaultThresholds()
	alerts := CheckMetrics(7, 50, 86, 50, thresholds)

	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Type != AlertHighRAM {
		t.Errorf("expected AlertHighRAM, got %s", alerts[0].Type)
	}
	if alerts[0].Value != 86 {
		t.Errorf("expected Value 86, got %f", alerts[0].Value)
	}
}

func TestCheckMetrics_DiskAlert(t *testing.T) {
	thresholds := DefaultThresholds()
	alerts := CheckMetrics(3, 50, 50, 95, thresholds)

	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Type != AlertHighDisk {
		t.Errorf("expected AlertHighDisk, got %s", alerts[0].Type)
	}
	if alerts[0].Value != 95 {
		t.Errorf("expected Value 95, got %f", alerts[0].Value)
	}
}

func TestCheckMetrics_MultipleAlerts(t *testing.T) {
	thresholds := DefaultThresholds()
	alerts := CheckMetrics(1, 95, 90, 95, thresholds)

	if len(alerts) != 3 {
		t.Fatalf("expected 3 alerts, got %d: %+v", len(alerts), alerts)
	}

	types := map[AlertType]bool{}
	for _, a := range alerts {
		types[a.Type] = true
	}

	if !types[AlertHighCPU] {
		t.Error("missing AlertHighCPU")
	}
	if !types[AlertHighRAM] {
		t.Error("missing AlertHighRAM")
	}
	if !types[AlertHighDisk] {
		t.Error("missing AlertHighDisk")
	}
}

func TestCheckMetrics_CustomThresholds(t *testing.T) {
	thresholds := Thresholds{
		CPUPercent:  50,
		RAMPercent:  60,
		DiskPercent: 70,
	}

	alerts := CheckMetrics(1, 51, 61, 71, thresholds)
	if len(alerts) != 3 {
		t.Fatalf("expected 3 alerts with custom thresholds, got %d", len(alerts))
	}

	// At threshold should NOT alert
	alerts = CheckMetrics(1, 50, 60, 70, thresholds)
	if len(alerts) != 0 {
		t.Errorf("expected no alerts at exact threshold, got %d", len(alerts))
	}
}

func TestCheckStatusTransition_NodeDown(t *testing.T) {
	tests := []struct {
		name     string
		old      string
		new      string
		wantNil  bool
		wantType AlertType
	}{
		{"online to stale", "online", "stale", false, AlertNodeDown},
		{"online to offline", "online", "offline", false, AlertNodeDown},
		{"online to online", "online", "online", true, ""},
		{"stale to offline", "stale", "offline", true, ""},
		{"stale to online", "stale", "online", true, ""},
		{"offline to online", "offline", "online", true, ""},
		{"offline to stale", "offline", "stale", true, ""},
		{"offline to offline", "offline", "offline", true, ""},
		{"stale to stale", "stale", "stale", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alert := CheckStatusTransition(5, tt.old, tt.new)
			if tt.wantNil {
				if alert != nil {
					t.Errorf("expected nil alert, got %+v", alert)
				}
			} else {
				if alert == nil {
					t.Fatal("expected an alert, got nil")
				}
				if alert.Type != tt.wantType {
					t.Errorf("expected type %s, got %s", tt.wantType, alert.Type)
				}
				if alert.NodeID != 5 {
					t.Errorf("expected NodeID 5, got %d", alert.NodeID)
				}
			}
		})
	}
}

func TestCheckStatusTransition_NodeIDPreserved(t *testing.T) {
	alert := CheckStatusTransition(999, "online", "offline")
	if alert == nil {
		t.Fatal("expected alert")
	}
	if alert.NodeID != 999 {
		t.Errorf("expected NodeID 999, got %d", alert.NodeID)
	}
}
