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

type Stats struct {
	Mean     float64
	Variance float64
	Min      int
	Max      int
}

func (p Party) Stats() (Stats, error) {
	if err := p.Validate(); err != nil {
		return Stats{}, err
	}
	elos := make([]int, len(p.Members))
	for i, m := range p.Members {
		elos[i] = m.Elo
	}
	mean := meanInt(elos)
	var sum float64
	for _, e := range elos {
		d := float64(e) - mean
		sum += d * d
	}
	return Stats{
		Mean:     mean,
		Variance: sum / float64(len(elos)),
		Min:      minInt(elos),
		Max:      maxInt(elos),
	}, nil
}

func meanInt(values []int) float64 {
	total := 0
	for _, v := range values {
		total += v
	}
	return float64(total) / float64(len(values))
}

func minInt(values []int) int {
	min := values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
	}
	return min
}

func maxInt(values []int) int {
	max := values[0]
	for _, v := range values[1:] {
		if v > max {
			max = v
		}
	}
	return max
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
