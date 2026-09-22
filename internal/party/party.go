package party

import (
	"errors"
	"fmt"
	"time"

	"player-matchmaker/internal/player"
)

const MaxMembers = 3

type Party struct {
	ID        string
	GameMode  string
	Members   []player.Player
	CreatedAt time.Time
}

func (p Party) Validate() error {
	if p.ID == "" {
		return errors.New("party id is required")
	}
	if len(p.Members) == 0 || len(p.Members) > MaxMembers {
		return fmt.Errorf("party must contain between 1 and %d players", MaxMembers)
	}

	seen := make(map[string]struct{}, len(p.Members))
	for _, member := range p.Members {
		if err := member.Validate(); err != nil {
			return fmt.Errorf("invalid party member: %w", err)
		}
		if _, exists := seen[member.ID]; exists {
			return fmt.Errorf("player %q appears more than once", member.ID)
		}
		seen[member.ID] = struct{}{}
	}
	return nil
}

// Elo returns the arithmetic mean of all players in the party.
func (p Party) Elo() (float64, error) {
	if err := p.Validate(); err != nil {
		return 0, err
	}

	total := 0
	for _, member := range p.Members {
		total += member.Elo
	}
	return float64(total) / float64(len(p.Members)), nil
}
