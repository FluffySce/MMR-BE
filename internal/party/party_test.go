package party

import (
	"testing"

	"player-matchmaker/internal/player"
)

func TestPartyUsesAveragePlayerElo(t *testing.T) {
	p := Party{ID: "party-1", Members: []player.Player{{ID: "p1", Elo: 1000}, {ID: "p2", Elo: 1200}, {ID: "p3", Elo: 1400}}}
	elo, err := p.Elo()
	if err != nil {
		t.Fatal(err)
	}
	if elo != 1200 {
		t.Fatalf("expected party elo 1200, got %v", elo)
	}
}

func TestPartyRejectsMoreThanThreeMembers(t *testing.T) {
	p := Party{ID: "party-1", Members: make([]player.Player, 4)}
	if err := p.Validate(); err == nil {
		t.Fatal("expected oversized party to fail")
	}
}
