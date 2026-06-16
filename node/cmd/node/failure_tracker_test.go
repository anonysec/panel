package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"koris-next/node/internal/logger"
)

func TestFailureTracker_InitialState(t *testing.T) {
	buf := &bytes.Buffer{}
	log := logger.NewWithWriter(logger.LevelDebug, buf)
	ft := NewFailureTracker(3, log)

	if ft.ConsecutiveFailures() != 0 {
		t.Errorf("expected 0 failures initially, got %d", ft.ConsecutiveFailures())
	}
	if !ft.FirstFailureTime().IsZero() {
		t.Errorf("expected zero first failure time initially")
	}
	if ft.LastError() != "" {
		t.Errorf("expected empty last error initially, got %q", ft.LastError())
	}
}

func TestFailureTracker_SingleFailureNoErrorLog(t *testing.T) {
	buf := &bytes.Buffer{}
	log := logger.NewWithWriter(logger.LevelDebug, buf)
	ft := NewFailureTracker(3, log)

	ft.RecordFailure("connection refused")

	if ft.ConsecutiveFailures() != 1 {
		t.Errorf("expected 1 failure, got %d", ft.ConsecutiveFailures())
	}
	if ft.FirstFailureTime().IsZero() {
		t.Errorf("expected first failure time to be set")
	}
	if ft.LastError() != "connection refused" {
		t.Errorf("expected last error 'connection refused', got %q", ft.LastError())
	}

	// Should NOT log at error level for fewer than 3 failures
	if strings.Contains(buf.String(), `"level":"error"`) {
		t.Errorf("should not log error for only 1 failure")
	}
}

func TestFailureTracker_TwoFailuresNoErrorLog(t *testing.T) {
	buf := &bytes.Buffer{}
	log := logger.NewWithWriter(logger.LevelDebug, buf)
	ft := NewFailureTracker(3, log)

	ft.RecordFailure("error 1")
	ft.RecordFailure("error 2")

	if ft.ConsecutiveFailures() != 2 {
		t.Errorf("expected 2 failures, got %d", ft.ConsecutiveFailures())
	}
	if ft.LastError() != "error 2" {
		t.Errorf("expected last error 'error 2', got %q", ft.LastError())
	}

	// Should NOT log at error level for fewer than 3 failures
	if strings.Contains(buf.String(), `"level":"error"`) {
		t.Errorf("should not log error for only 2 failures")
	}
}

func TestFailureTracker_ThreeFailuresTriggersErrorLog(t *testing.T) {
	buf := &bytes.Buffer{}
	log := logger.NewWithWriter(logger.LevelDebug, buf)
	ft := NewFailureTracker(3, log)

	ft.RecordFailure("error 1")
	ft.RecordFailure("error 2")
	ft.RecordFailure("error 3")

	if ft.ConsecutiveFailures() != 3 {
		t.Errorf("expected 3 failures, got %d", ft.ConsecutiveFailures())
	}

	// Should log at error level after 3 failures
	output := buf.String()
	if !strings.Contains(output, `"level":"error"`) {
		t.Errorf("expected error-level log after 3 failures, got: %s", output)
	}
	if !strings.Contains(output, "panel unreachable for multiple push intervals") {
		t.Errorf("expected 'panel unreachable' message, got: %s", output)
	}

	// Parse the error log entry to verify fields
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var errorEntry logger.LogEntry
	for _, line := range lines {
		var entry logger.LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err == nil {
			if entry.Level == "error" {
				errorEntry = entry
				break
			}
		}
	}

	if errorEntry.Fields == nil {
		t.Fatal("expected error log entry with fields")
	}
	if errorEntry.Fields["consecutive_failures"] != float64(3) {
		t.Errorf("expected consecutive_failures=3, got %v", errorEntry.Fields["consecutive_failures"])
	}
	if errorEntry.Fields["last_error"] != "error 3" {
		t.Errorf("expected last_error='error 3', got %v", errorEntry.Fields["last_error"])
	}
	if _, ok := errorEntry.Fields["disconnection_duration"]; !ok {
		t.Errorf("expected disconnection_duration field in error log")
	}
	if _, ok := errorEntry.Fields["disconnection_seconds"]; !ok {
		t.Errorf("expected disconnection_seconds field in error log")
	}
}

func TestFailureTracker_MoreThanThreeFailuresContinuesLogging(t *testing.T) {
	buf := &bytes.Buffer{}
	log := logger.NewWithWriter(logger.LevelDebug, buf)
	ft := NewFailureTracker(3, log)

	for i := 0; i < 5; i++ {
		ft.RecordFailure("persistent error")
	}

	if ft.ConsecutiveFailures() != 5 {
		t.Errorf("expected 5 failures, got %d", ft.ConsecutiveFailures())
	}

	// Count error log entries — should be 3 (failures 3, 4, 5 trigger error)
	output := buf.String()
	errorCount := strings.Count(output, `"level":"error"`)
	if errorCount != 3 {
		t.Errorf("expected 3 error logs (for failures 3,4,5), got %d", errorCount)
	}
}

func TestFailureTracker_SuccessResetsCounter(t *testing.T) {
	buf := &bytes.Buffer{}
	log := logger.NewWithWriter(logger.LevelDebug, buf)
	ft := NewFailureTracker(3, log)

	ft.RecordFailure("error 1")
	ft.RecordFailure("error 2")
	ft.RecordSuccess()

	if ft.ConsecutiveFailures() != 0 {
		t.Errorf("expected 0 failures after success, got %d", ft.ConsecutiveFailures())
	}
	if !ft.FirstFailureTime().IsZero() {
		t.Errorf("expected first failure time to be reset after success")
	}
	if ft.LastError() != "" {
		t.Errorf("expected last error to be empty after success, got %q", ft.LastError())
	}

	// Should log info about connectivity restored
	output := buf.String()
	if !strings.Contains(output, "panel connectivity restored") {
		t.Errorf("expected 'panel connectivity restored' message, got: %s", output)
	}
}

func TestFailureTracker_SuccessWithNoFailuresDoesNotLogRecovery(t *testing.T) {
	buf := &bytes.Buffer{}
	log := logger.NewWithWriter(logger.LevelDebug, buf)
	ft := NewFailureTracker(3, log)

	ft.RecordSuccess()

	output := buf.String()
	if strings.Contains(output, "panel connectivity restored") {
		t.Errorf("should not log recovery when there were no failures, got: %s", output)
	}
}

func TestFailureTracker_ResetAfterThreeFailuresThenNewStreak(t *testing.T) {
	buf := &bytes.Buffer{}
	log := logger.NewWithWriter(logger.LevelDebug, buf)
	ft := NewFailureTracker(3, log)

	// First streak of 3 failures
	ft.RecordFailure("error A")
	ft.RecordFailure("error B")
	ft.RecordFailure("error C")

	// Reset
	ft.RecordSuccess()

	// Clear buffer for second streak
	buf.Reset()

	// Second streak — should not trigger error until 3 new failures
	ft.RecordFailure("error X")
	ft.RecordFailure("error Y")

	output := buf.String()
	if strings.Contains(output, `"level":"error"`) {
		t.Errorf("should not log error for only 2 failures in new streak")
	}

	ft.RecordFailure("error Z")
	output = buf.String()
	if !strings.Contains(output, `"level":"error"`) {
		t.Errorf("expected error log after 3 failures in new streak")
	}
}

func TestFailureTracker_FirstFailureTimePreservedAcrossStreak(t *testing.T) {
	buf := &bytes.Buffer{}
	log := logger.NewWithWriter(logger.LevelDebug, buf)
	ft := NewFailureTracker(3, log)

	ft.RecordFailure("first")
	firstTime := ft.FirstFailureTime()

	ft.RecordFailure("second")
	ft.RecordFailure("third")

	// First failure time should not change during the streak
	if !ft.FirstFailureTime().Equal(firstTime) {
		t.Errorf("first failure time should not change during streak")
	}
}
