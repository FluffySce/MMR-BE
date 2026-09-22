package matchmaking

import (
	"fmt"
	"math"
	"sort"
	"time"

	"player-matchmaker/internal/match"
	"player-matchmaker/internal/player"
	"player-matchmaker/internal/queue"
)

const (
	DefaultWithinTeamMaxSpread = 200.0
	DefaultBaseTeamEloGap      = 100.0
	DefaultGrowthPerMinute     = 50.0
	DefaultMaxAdditionalGap    = 150.0
)

type Config struct {
	WithinTeamMaxSpread float64
	BaseTeamEloGap      float64
	GrowthPerMinute     float64
	MaxAdditionalGap    float64
	Now                 func() time.Time
}

func DefaultConfig() Config {
	return Config{
		WithinTeamMaxSpread: DefaultWithinTeamMaxSpread,
		BaseTeamEloGap:      DefaultBaseTeamEloGap,
		GrowthPerMinute:     DefaultGrowthPerMinute,
		MaxAdditionalGap:    DefaultMaxAdditionalGap,
		Now:                 time.Now,
	}
}

type Rejection struct {
	PartyIDs []string
	Reasons  []string
}

type Result struct {
	Match      *match.Match
	Rejections []Rejection
}

type Matcher struct {
	config      Config
	friendships map[string]map[string]struct{}
	avoids      map[string]map[string]struct{}
}

func New(config Config) *Matcher {
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Matcher{
		config:      config,
		friendships: make(map[string]map[string]struct{}),
		avoids:      make(map[string]map[string]struct{}),
	}
}

func (m *Matcher) AddFriendship(playerID, friendID string) {
	addRelation(m.friendships, playerID, friendID)
	addRelation(m.friendships, friendID, playerID)
}

func (m *Matcher) AddAvoidance(playerID, avoidedPlayerID string) {
	addRelation(m.avoids, playerID, avoidedPlayerID)
}

func (m *Matcher) FindMatch(entries []queue.Entry, gameMode string) Result {
	result := Result{}
	eligible := make([]queue.Entry, 0, len(entries))
	for _, entry := range entries {
		if entry.GameMode != gameMode {
			continue
		}
		if reason := m.ineligibleParty(entry); reason != "" {
			result.Rejections = append(result.Rejections, Rejection{PartyIDs: []string{entry.Party.ID}, Reasons: []string{reason}})
			continue
		}
		eligible = append(eligible, entry)
	}

	for size := 1; size <= len(eligible); size++ {
		var found *match.Match
		var foundRejections []Rejection
		m.visitCombinations(eligible, size, 0, nil, func(candidate []queue.Entry) bool {
			if totalPlayers(candidate) != 10 {
				return true
			}
			candidateMatch, reasons := m.tryCandidate(candidate)
			if candidateMatch != nil {
				found = candidateMatch
				return false
			}
			foundRejections = append(foundRejections, Rejection{PartyIDs: partyIDs(candidate), Reasons: reasons})
			return true
		})
		if found != nil {
			result.Match = found
			return result
		}
		result.Rejections = append(result.Rejections, foundRejections...)
	}
	return result
}

func (m *Matcher) tryCandidate(candidate []queue.Entry) (*match.Match, []string) {
	if reason := m.relationshipConflict(candidate); reason != "" {
		return nil, []string{reason}
	}

	for mask := 1; mask < (1<<len(candidate))-1; mask++ {
		teamAEntries, teamBEntries := splitCandidate(candidate, mask)
		if totalPlayers(teamAEntries) != 5 || totalPlayers(teamBEntries) != 5 {
			continue
		}

		teamAPlayers := flatten(teamAEntries)
		teamBPlayers := flatten(teamBEntries)
		if spread(teamAPlayers) > m.config.WithinTeamMaxSpread || spread(teamBPlayers) > m.config.WithinTeamMaxSpread {
			continue
		}

		teamAElo := averageElo(teamAPlayers)
		teamBElo := averageElo(teamBPlayers)
		if math.Abs(teamAElo-teamBElo) > m.allowedTeamGap(candidate) {
			continue
		}

		return &match.Match{
			ID:            fmt.Sprintf("match-%d", m.config.Now().UnixNano()),
			GameMode:      candidate[0].GameMode,
			TeamAElo:      teamAElo,
			TeamBElo:      teamBElo,
			EloDifference: math.Abs(teamAElo - teamBElo),
			Players:       matchPlayers(teamAEntries, teamBEntries),
			CreatedAt:     m.config.Now(),
		}, nil
	}

	return nil, []string{"no valid 5v5 partition satisfies the ELO constraints"}
}

func (m *Matcher) allowedTeamGap(candidate []queue.Entry) float64 {
	oldest := candidate[0].EnqueuedAt
	for _, entry := range candidate[1:] {
		if entry.EnqueuedAt.Before(oldest) {
			oldest = entry.EnqueuedAt
		}
	}
	minutes := math.Floor(m.config.Now().Sub(oldest).Minutes())
	if minutes < 0 {
		minutes = 0
	}
	additional := math.Min(minutes*m.config.GrowthPerMinute, m.config.MaxAdditionalGap)
	return m.config.BaseTeamEloGap + additional
}

func (m *Matcher) ineligibleParty(entry queue.Entry) string {
	for _, member := range entry.Party.Members {
		if member.Presence != player.Online {
			return fmt.Sprintf("party contains player %q who is not online", member.ID)
		}
	}
	return ""
}

func (m *Matcher) relationshipConflict(entries []queue.Entry) string {
	players := flatten(entries)
	seen := make(map[string]player.Player, len(players))
	for _, current := range players {
		if _, exists := seen[current.ID]; exists {
			return fmt.Sprintf("player %q appears in more than one queued party", current.ID)
		}
		for otherID := range seen {
			if current.Invisible && m.isFriend(current.ID, otherID) || seen[otherID].Invisible && m.isFriend(otherID, current.ID) {
				return fmt.Sprintf("invisible friends %q and %q cannot share a match", current.ID, otherID)
			}
			if m.isAvoidedByEither(current.ID, otherID) {
				return fmt.Sprintf("players %q and %q cannot share a match because of avoidance", current.ID, otherID)
			}
		}
		seen[current.ID] = current
	}
	return ""
}

func (m *Matcher) isFriend(playerID, friendID string) bool {
	_, exists := m.friendships[playerID][friendID]
	return exists
}

func (m *Matcher) isAvoidedByEither(playerID, otherID string) bool {
	_, first := m.avoids[playerID][otherID]
	_, second := m.avoids[otherID][playerID]
	return first || second
}

func (m *Matcher) visitCombinations(entries []queue.Entry, size, start int, current []queue.Entry, visit func([]queue.Entry) bool) bool {
	if len(current) == size {
		return visit(append([]queue.Entry(nil), current...))
	}
	for i := start; i <= len(entries)-(size-len(current)); i++ {
		if !m.visitCombinations(entries, size, i+1, append(current, entries[i]), visit) {
			return false
		}
	}
	return true
}

func addRelation(relations map[string]map[string]struct{}, from, to string) {
	if relations[from] == nil {
		relations[from] = make(map[string]struct{})
	}
	relations[from][to] = struct{}{}
}

func splitCandidate(candidate []queue.Entry, mask int) ([]queue.Entry, []queue.Entry) {
	teamA, teamB := make([]queue.Entry, 0), make([]queue.Entry, 0)
	for i, entry := range candidate {
		if mask&(1<<i) != 0 {
			teamA = append(teamA, entry)
		} else {
			teamB = append(teamB, entry)
		}
	}
	return teamA, teamB
}

func flatten(entries []queue.Entry) []player.Player {
	players := make([]player.Player, 0)
	for _, entry := range entries {
		players = append(players, entry.Party.Members...)
	}
	return players
}

func totalPlayers(entries []queue.Entry) int {
	total := 0
	for _, entry := range entries {
		total += len(entry.Party.Members)
	}
	return total
}

func averageElo(players []player.Player) float64 {
	total := 0
	for _, current := range players {
		total += current.Elo
	}
	return float64(total) / float64(len(players))
}

func spread(players []player.Player) float64 {
	elos := make([]int, 0, len(players))
	for _, current := range players {
		elos = append(elos, current.Elo)
	}
	sort.Ints(elos)
	return float64(elos[len(elos)-1] - elos[0])
}

func matchPlayers(teamA, teamB []queue.Entry) []match.Player {
	players := make([]match.Player, 0, 10)
	for _, entry := range teamA {
		for _, member := range entry.Party.Members {
			players = append(players, match.Player{PlayerID: member.ID, PartyID: entry.Party.ID, Team: match.TeamA})
		}
	}
	for _, entry := range teamB {
		for _, member := range entry.Party.Members {
			players = append(players, match.Player{PlayerID: member.ID, PartyID: entry.Party.ID, Team: match.TeamB})
		}
	}
	return players
}

func partyIDs(entries []queue.Entry) []string {
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.Party.ID)
	}
	return ids
}
