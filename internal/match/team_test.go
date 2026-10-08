package match

import (
	"testing"

	"player-matchmaker/internal/player"
)

func TestTeamStats(t *testing.T) {
	// Team with multiple players
	team := NewTeam([]player.Player{
		{ID: "p1", Elo: 1000},
		{ID: "p2", Elo: 1100},
		{ID: "p3", Elo: 1200},
		{ID: "p4", Elo: 1300},
		{ID: "p5", Elo: 1400},
	})
	if team.Len() != 5 {
		t.Fatalf("expected 5 players, got %d", team.Len())
	}
	if mean := team.Mean(); mean != 1200 {
		t.Fatalf("expected mean 1200, got %v", mean)
	}
	if variance := team.Variance(); variance != 20000 {
		t.Fatalf("expected variance %v, got %v", 20000, variance)
	}
	if min := team.Min(); min != 1000 {
		t.Fatalf("expected min 1000, got %d", min)
	}
	if max := team.Max(); max != 1400 {
		t.Fatalf("expected max 1400, got %d", max)
	}
	if spread := team.Spread(); spread != 400 {
		t.Fatalf("expected spread 400, got %v", spread)
	}
	if diff := team.HighestPlayerDifference(); diff != 200 {
		t.Fatalf("expected highest diff 200, got %v", diff)
	}
	if diff := team.LowestPlayerDifference(); diff != -200 {
		t.Fatalf("expected lowest diff -200, got %v", diff)
	}
}

func TestTeamStatsEmpty(t *testing.T) {
	team := NewTeam([]player.Player{})
	if team.Len() != 0 {
		t.Fatal("expected empty team")
	}
	if mean := team.Mean(); mean != 0 {
		t.Fatalf("expected mean 0, got %v", mean)
	}
	if variance := team.Variance(); variance != 0 {
		t.Fatalf("expected variance 0, got %v", variance)
	}
	if min := team.Min(); min != 0 {
		t.Fatalf("expected min 0, got %d", min)
	}
	if max := team.Max(); max != 0 {
		t.Fatalf("expected max 0, got %d", max)
	}
}