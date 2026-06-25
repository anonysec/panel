package grpcclient

import "time"

// BackoffPolicy computes reconnection delays using exponential backoff.
// Formula: delay(attempt) = min(baseDelay * 2^attempt, maxDelay)
//
// Default sequence: 2s, 4s, 8s, 16s, 32s, 60s, 60s, ...
type BackoffPolicy struct {
	BaseDelay time.Duration // default: 2s
	MaxDelay  time.Duration // default: 60s
}

// DefaultBackoff returns a BackoffPolicy with standard defaults (2s base, 60s max).
func DefaultBackoff() BackoffPolicy {
	return BackoffPolicy{
		BaseDelay: 2 * time.Second,
		MaxDelay:  60 * time.Second,
	}
}

// Delay computes the backoff duration for the given attempt number (0-indexed).
// The result is min(baseDelay * 2^attempt, maxDelay).
func (b BackoffPolicy) Delay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}

	base := b.BaseDelay
	if base <= 0 {
		base = 2 * time.Second
	}
	max := b.MaxDelay
	if max <= 0 {
		max = 60 * time.Second
	}

	delay := base
	for i := 0; i < attempt; i++ {
		delay *= 2
		if delay > max {
			return max
		}
	}
	return delay
}
