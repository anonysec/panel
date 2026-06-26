package grpcclient

import (
	"testing"
	"time"
)

func TestDefaultBackoff(t *testing.T) {
	b := DefaultBackoff()
	if b.BaseDelay != 2*time.Second {
		t.Errorf("expected base delay 2s, got %s", b.BaseDelay)
	}
	if b.MaxDelay != 60*time.Second {
		t.Errorf("expected max delay 60s, got %s", b.MaxDelay)
	}
}

func TestBackoffDelay_ExponentialSequence(t *testing.T) {
	b := DefaultBackoff()

	// Expected sequence: 2s, 4s, 8s, 16s, 32s, 60s, 60s, ...
	expected := []time.Duration{
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		32 * time.Second,
		60 * time.Second,
		60 * time.Second,
		60 * time.Second,
	}

	for i, want := range expected {
		got := b.Delay(i)
		if got != want {
			t.Errorf("Delay(%d) = %s, want %s", i, got, want)
		}
	}
}

func TestBackoffDelay_NeverExceedsMax(t *testing.T) {
	b := DefaultBackoff()

	for attempt := 0; attempt < 100; attempt++ {
		delay := b.Delay(attempt)
		if delay > b.MaxDelay {
			t.Errorf("Delay(%d) = %s exceeds max %s", attempt, delay, b.MaxDelay)
		}
		if delay < b.BaseDelay {
			t.Errorf("Delay(%d) = %s below base %s", attempt, delay, b.BaseDelay)
		}
	}
}

func TestBackoffDelay_NegativeAttemptTreatedAsZero(t *testing.T) {
	b := DefaultBackoff()

	got := b.Delay(-1)
	if got != 2*time.Second {
		t.Errorf("Delay(-1) = %s, want 2s", got)
	}
}

func TestBackoffDelay_Formula(t *testing.T) {
	// Verify: min(2s × 2^attempt, 60s)
	b := DefaultBackoff()

	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 2 * time.Second},   // 2 * 2^0 = 2
		{1, 4 * time.Second},   // 2 * 2^1 = 4
		{2, 8 * time.Second},   // 2 * 2^2 = 8
		{3, 16 * time.Second},  // 2 * 2^3 = 16
		{4, 32 * time.Second},  // 2 * 2^4 = 32
		{5, 60 * time.Second},  // 2 * 2^5 = 64, capped to 60
		{6, 60 * time.Second},  // 2 * 2^6 = 128, capped to 60
		{10, 60 * time.Second}, // way beyond cap
		{50, 60 * time.Second}, // extreme attempt
	}

	for _, tc := range cases {
		got := b.Delay(tc.attempt)
		if got != tc.want {
			t.Errorf("Delay(%d) = %s, want %s", tc.attempt, got, tc.want)
		}
	}
}

func TestBackoffDelay_CustomPolicy(t *testing.T) {
	b := BackoffPolicy{
		BaseDelay: 1 * time.Second,
		MaxDelay:  10 * time.Second,
	}

	// Sequence: 1s, 2s, 4s, 8s, 10s, 10s
	expected := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		10 * time.Second,
		10 * time.Second,
	}

	for i, want := range expected {
		got := b.Delay(i)
		if got != want {
			t.Errorf("custom Delay(%d) = %s, want %s", i, got, want)
		}
	}
}

func TestBackoffDelay_ZeroBaseUsesDefault(t *testing.T) {
	b := BackoffPolicy{BaseDelay: 0, MaxDelay: 60 * time.Second}
	got := b.Delay(0)
	if got != 2*time.Second {
		t.Errorf("zero base Delay(0) = %s, want 2s", got)
	}
}

func TestBackoffDelay_ZeroMaxUsesDefault(t *testing.T) {
	b := BackoffPolicy{BaseDelay: 2 * time.Second, MaxDelay: 0}
	got := b.Delay(5)
	if got != 60*time.Second {
		t.Errorf("zero max Delay(5) = %s, want 60s (default max)", got)
	}
}
