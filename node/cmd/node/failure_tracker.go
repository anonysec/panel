package main

import (
	"time"

	"KorisPanel/node/internal/logger"
)

// FailureTracker tracks consecutive push failures and logs an error
// after a configurable threshold of consecutive failures.
type FailureTracker struct {
	consecutiveFailures int
	firstFailureTime    time.Time
	lastErrorMsg        string
	threshold           int
	log                 *logger.Logger
}

// NewFailureTracker creates a FailureTracker with the given threshold
// (number of consecutive failures before logging at error level).
func NewFailureTracker(threshold int, log *logger.Logger) *FailureTracker {
	return &FailureTracker{
		threshold: threshold,
		log:       log,
	}
}

// RecordSuccess resets the failure counter. It logs a recovery message
// if there were previous failures.
func (ft *FailureTracker) RecordSuccess() {
	if ft.consecutiveFailures > 0 {
		ft.log.Info("panel connectivity restored", map[string]any{
			"previous_failures": ft.consecutiveFailures,
		})
	}
	ft.consecutiveFailures = 0
	ft.firstFailureTime = time.Time{}
	ft.lastErrorMsg = ""
}

// RecordFailure increments the failure counter and logs at error level
// when the threshold is reached or exceeded.
func (ft *FailureTracker) RecordFailure(errMsg string) {
	ft.consecutiveFailures++
	ft.lastErrorMsg = errMsg
	if ft.consecutiveFailures == 1 {
		ft.firstFailureTime = time.Now()
	}
	if ft.consecutiveFailures >= ft.threshold {
		disconnectDuration := time.Since(ft.firstFailureTime)
		ft.log.Error("panel unreachable for multiple push intervals", map[string]any{
			"consecutive_failures":   ft.consecutiveFailures,
			"disconnection_duration": disconnectDuration.String(),
			"disconnection_seconds":  int64(disconnectDuration.Seconds()),
			"last_error":             ft.lastErrorMsg,
		})
	}
}

// ConsecutiveFailures returns the current failure count.
func (ft *FailureTracker) ConsecutiveFailures() int {
	return ft.consecutiveFailures
}

// FirstFailureTime returns the time of the first failure in the current streak.
func (ft *FailureTracker) FirstFailureTime() time.Time {
	return ft.firstFailureTime
}

// LastError returns the last recorded error message.
func (ft *FailureTracker) LastError() string {
	return ft.lastErrorMsg
}
