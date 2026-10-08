package queue

import (
	"testing"
	"time"
)

func TestSearchPolicyDefaultWindow(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)
	policy := DefaultSearchPolicy()
	policy.Now = func() time.Time { return now }

	// Just enqueued (0 minutes wait)
	window := policy.WindowFor(0)
	if window != 100 {
		t.Fatalf("expected base window 100, got %v", window)
	}
}

func TestSearchPolicyExpandsWithWaitTime(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)
	policy := DefaultSearchPolicy()
	policy.Now = func() time.Time { return now }

	// 1 minute wait
	window := policy.WindowFor(time.Minute)
	if window != 150 {
		t.Fatalf("expected 150 at 1 min, got %v", window)
	}

	// 2 minutes wait
	window = policy.WindowFor(2 * time.Minute)
	if window != 200 {
		t.Fatalf("expected 200 at 2 min, got %v", window)
	}

	// 6 minutes wait
	window = policy.WindowFor(6 * time.Minute)
	if window != 400 {
		t.Fatalf("expected 400 at 6 min, got %v", window)
	}
}

func TestSearchPolicyCapsAtMaxWindow(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)
	policy := DefaultSearchPolicy()
	policy.Now = func() time.Time { return now }

	// 10 minutes wait - should cap at 400
	window := policy.WindowFor(10 * time.Minute)
	if window != 400 {
		t.Fatalf("expected max window 400, got %v", window)
	}

	// 60 minutes wait - still capped
	window = policy.WindowFor(60 * time.Minute)
	if window != 400 {
		t.Fatalf("expected max window 400 at 60 min, got %v", window)
	}
}

func TestSearchPolicyEloRange(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)
	policy := DefaultSearchPolicy()
	policy.Now = func() time.Time { return now }

	// Party at 1000 ELO, just enqueued
	minElo, maxElo := policy.EloRange(1000, now)
	if minElo != 900 || maxElo != 1100 {
		t.Fatalf("expected [900, 1100], got [%v, %v]", minElo, maxElo)
	}

	// Party at 1500 ELO, 2 minutes wait
	enqueuedAt := now.Add(-2 * time.Minute)
	minElo, maxElo = policy.EloRange(1500, enqueuedAt)
	if minElo != 1300 || maxElo != 1700 {
		t.Fatalf("expected [1300, 1700], got [%v, %v]", minElo, maxElo)
	}
}

func TestSearchPolicyCustomConfig(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)
	policy := SearchPolicy{
		BaseWindow:    50,
		MaxWindow:     300,
		ExpansionRate: 25,
		Now:           func() time.Time { return now },
	}

	// 0 min
	window := policy.WindowFor(0)
	if window != 50 {
		t.Fatalf("expected 50, got %v", window)
	}

	// 4 min
	window = policy.WindowFor(4 * time.Minute)
	if window != 150 {
		t.Fatalf("expected 150 at 4 min, got %v", window)
	}

	// 10 min - capped at 300
	window = policy.WindowFor(10 * time.Minute)
	if window != 300 {
		t.Fatalf("expected 300 max, got %v", window)
	}
}

func TestSearchPolicyNegativeQueueAge(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)
	policy := DefaultSearchPolicy()
	policy.Now = func() time.Time { return now }

	// Negative queue age (should be treated as 0)
	window := policy.WindowFor(-time.Minute)
	if window != 100 {
		t.Fatalf("expected base window for negative age, got %v", window)
	}
}