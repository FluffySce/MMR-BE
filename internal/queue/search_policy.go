package queue

import (
	"time"
)

// SearchPolicy defines how the ELO search window expands with queue time.
type SearchPolicy struct {
	// BaseWindow is the initial ELO window (e.g., ±100)
	BaseWindow float64
	// MaxWindow is the maximum ELO window (e.g., ±400)
	MaxWindow float64
	// ExpansionRate is how much the window grows per minute of wait time
	ExpansionRate float64
	// Now is a function returning current time (for testing)
	Now func() time.Time
}

func DefaultSearchPolicy() SearchPolicy {
	return SearchPolicy{
		BaseWindow:    100,
		MaxWindow:     400,
		ExpansionRate: 50, // per minute
		Now:           time.Now,
	}
}

// WindowFor returns the ELO window for a given queue age (duration).
func (p SearchPolicy) WindowFor(queueAge time.Duration) float64 {
	minutes := queueAge.Minutes()
	if minutes < 0 {
		minutes = 0
	}
	window := p.BaseWindow + minutes*p.ExpansionRate
	if window > p.MaxWindow {
		window = p.MaxWindow
	}
	return window
}

// EloRange returns the min/max ELO for a party with the given average ELO and queue age.
func (p SearchPolicy) EloRange(partyElo float64, enqueuedAt time.Time) (float64, float64) {
	queueAge := p.Now().Sub(enqueuedAt)
	window := p.WindowFor(queueAge)
	return partyElo - window, partyElo + window
}