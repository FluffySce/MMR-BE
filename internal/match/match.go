package match

import (
	"errors"
	"fmt"
	"time"
)

type Team string

const (
	TeamA Team = "A"
	TeamB Team = "B"
)

type Player struct {
	PlayerID string
	PartyID  string
	Team     Team
}

type Match struct {
	ID            string
	GameMode      string
	TeamAElo      float64
	TeamBElo      float64
	EloDifference float64
	Players       []Player
	CreatedAt     time.Time
}

func (m Match) Validate() error {
	if m.ID == "" {
		return errors.New("match id is required")
	}
	if len(m.Players) != 10 {
		return fmt.Errorf("match must contain exactly 10 players, got %d", len(m.Players))
	}

	teamCounts := map[Team]int{}
	seen := make(map[string]struct{}, len(m.Players))
	for _, player := range m.Players {
		if player.PlayerID == "" || player.PartyID == "" {
			return errors.New("match player and party ids are required")
		}
		if player.Team != TeamA && player.Team != TeamB {
			return fmt.Errorf("invalid team %q", player.Team)
		}
		if _, exists := seen[player.PlayerID]; exists {
			return fmt.Errorf("player %q appears more than once", player.PlayerID)
		}
		seen[player.PlayerID] = struct{}{}
		teamCounts[player.Team]++
	}
	if teamCounts[TeamA] != 5 || teamCounts[TeamB] != 5 {
		return fmt.Errorf("match teams must contain 5 players each: team A=%d, team B=%d", teamCounts[TeamA], teamCounts[TeamB])
	}
	return nil
}
