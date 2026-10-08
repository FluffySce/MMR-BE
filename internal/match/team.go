package match

import "player-matchmaker/internal/player"

// Team holds a set of players and exposes ELO statistics.
type Team struct {
	players []player.Player
}

// NewTeam constructs a Team from the given players.
func NewTeam(players []player.Player) Team {
	out := make([]player.Player, len(players))
	copy(out, players)
	return Team{players: out}
}

// Len returns the number of players in the team.
func (t Team) Len() int { return len(t.players) }

// Mean returns the average ELO of the team.
func (t Team) Mean() float64 {
	if len(t.players) == 0 {
		return 0
	}
	total := 0
	for _, p := range t.players {
		total += p.Elo
	}
	return float64(total) / float64(len(t.players))
}

// Variance returns the population variance of player ELOs.
func (t Team) Variance() float64 {
	if len(t.players) == 0 {
		return 0
	}
	mean := t.Mean()
	sum := 0.0
	for _, p := range t.players {
		d := float64(p.Elo) - mean
		sum += d * d
	}
	return sum / float64(len(t.players))
}

// Min returns the lowest ELO in the team.
func (t Team) Min() int {
	if len(t.players) == 0 {
		return 0
	}
	min := t.players[0].Elo
	for _, p := range t.players[1:] {
		if p.Elo < min {
			min = p.Elo
		}
	}
	return min
}

// Max returns the highest ELO in the team.
func (t Team) Max() int {
	if len(t.players) == 0 {
		return 0
	}
	max := t.players[0].Elo
	for _, p := range t.players[1:] {
		if p.Elo > max {
			max = p.Elo
		}
	}
	return max
}

// Spread returns the difference between the highest and lowest ELO.
func (t Team) Spread() float64 {
	return float64(t.Max() - t.Min())
}

// HighestPlayerDifference returns how far the highest-ELO player
// deviates from the team mean.
func (t Team) HighestPlayerDifference() float64 {
	return float64(t.Max()) - t.Mean()
}

// LowestPlayerDifference returns how far the lowest-ELO player
// deviates from the team mean (negative when below the mean).
func (t Team) LowestPlayerDifference() float64 {
	return float64(t.Min()) - t.Mean()
}