package party

import (
	"testing"

	"player-matchmaker/internal/player"
)

func TestPartyStats(t *testing.T) {
	p := Party{
		ID: "party-1",
		Members: []player.Player{
			{ID: "p1", Elo: 1000},
			{ID: "p2", Elo: 1200},
			{ID: "p3", Elo: 1400},
		},
	}
	// Validate party (max 3 members)
	if err := p.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	stats, err := p.Stats()
	if err != nil {
		t.Fatalf("unexpected stats error: %v", err)
	}
	if stats.Mean != 1200 {
		t.Fatalf("expected mean 1200, got %v", stats.Mean)
	}
	if stats.Variance != 26666.666666666668 {
		t.Fatalf("expected variance 80000/3, got %v", stats.Variance)
	}
	if stats.Min != 1000 {
		t.Fatalf("expected min 1000, got %d", stats.Min)
	}
	if stats.Max != 1400 {
		t.Fatalf("expected max 1400, got %d", stats.Max)
	}
}

func TestPartyStatsTooManyMembers(t *testing.T) {
	p := Party{
		ID: "party-1",
		Members: make([]player.Player, 4),
	}
	if err := p.Validate(); err == nil {
		t.Fatal("expected validation to fail for >3 members")
	}
}
