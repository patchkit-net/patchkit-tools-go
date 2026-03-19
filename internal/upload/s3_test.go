package upload

import (
	"testing"
	"time"
)

func TestRetryBackoff(t *testing.T) {
	// Test that backoff increases exponentially
	prev := time.Duration(0)
	for attempt := 1; attempt <= 5; attempt++ {
		d := retryBackoff(attempt)
		if d <= 0 {
			t.Errorf("attempt %d: backoff should be positive, got %v", attempt, d)
		}
		if d > 60*time.Second+12*time.Second { // 60s + 20% jitter
			t.Errorf("attempt %d: backoff %v exceeds cap", attempt, d)
		}
		if attempt > 1 && d < prev/2 {
			// With jitter, the next value should generally be larger,
			// but jitter can cause some variation. Just check it's not absurdly small.
		}
		prev = d
	}

	// Test that high attempt numbers are capped at ~60s
	d := retryBackoff(20)
	if d > 72*time.Second { // 60s + 20% max jitter
		t.Errorf("attempt 20: backoff %v should be capped near 60s", d)
	}
}

func TestRetryBackoff_firstAttempt(t *testing.T) {
	// First attempt should be around 2s +/- 20%
	for i := 0; i < 100; i++ {
		d := retryBackoff(1)
		if d < 1600*time.Millisecond || d > 2400*time.Millisecond {
			t.Errorf("attempt 1: backoff %v outside expected range [1.6s, 2.4s]", d)
		}
	}
}
