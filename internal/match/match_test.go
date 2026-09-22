package match

import "testing"

func TestMatchRequiresFivePlayersPerTeam(t *testing.T) {
	players := make([]Player, 10)
	for i := range players {
		players[i] = Player{PlayerID: string(rune('a' + i)), PartyID: "party", Team: TeamA}
	}
	m := Match{ID: "match-1", Players: players}
	if err := m.Validate(); err == nil {
		t.Fatal("expected unbalanced teams to fail")
	}
}
