package alerts

import (
	"testing"
)

func TestAlerter_EvaluateMetrics_DispatchesAlerts(t *testing.T) {
	alerter := NewAlerter(DefaultThresholds())

	var received []Alert
	alerter.OnAlert(func(a Alert) {
		received = append(received, a)
	})

	// Should trigger CPU alert
	alerter.EvaluateMetrics(1, 95, 50, 50)

	// Default LogHandler + our custom handler = 1 alert dispatched to custom handler
	if len(received) != 1 {
		t.Fatalf("expected 1 alert dispatched, got %d", len(received))
	}
	if received[0].Type != AlertHighCPU {
		t.Errorf("expected AlertHighCPU, got %s", received[0].Type)
	}
}

func TestAlerter_EvaluateMetrics_NoDispatchBelowThreshold(t *testing.T) {
	alerter := NewAlerter(DefaultThresholds())

	var received []Alert
	alerter.OnAlert(func(a Alert) {
		received = append(received, a)
	})

	alerter.EvaluateMetrics(1, 50, 50, 50)

	if len(received) != 0 {
		t.Errorf("expected no alerts, got %d", len(received))
	}
}

func TestAlerter_EvaluateStatusTransition_DispatchesNodeDown(t *testing.T) {
	alerter := NewAlerter(DefaultThresholds())

	var received []Alert
	alerter.OnAlert(func(a Alert) {
		received = append(received, a)
	})

	alerter.EvaluateStatusTransition(10, "online", "offline")

	if len(received) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(received))
	}
	if received[0].Type != AlertNodeDown {
		t.Errorf("expected AlertNodeDown, got %s", received[0].Type)
	}
	if received[0].NodeID != 10 {
		t.Errorf("expected NodeID 10, got %d", received[0].NodeID)
	}
}

func TestAlerter_EvaluateStatusTransition_NoAlertFromStale(t *testing.T) {
	alerter := NewAlerter(DefaultThresholds())

	var received []Alert
	alerter.OnAlert(func(a Alert) {
		received = append(received, a)
	})

	alerter.EvaluateStatusTransition(10, "stale", "offline")

	if len(received) != 0 {
		t.Errorf("expected no alerts for stale→offline, got %d", len(received))
	}
}
