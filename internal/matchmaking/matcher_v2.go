package matchmaking

import (
	"fmt"
	"math"
	"time"

	"player-matchmaker/internal/match"
	"player-matchmaker/internal/player"
	"player-matchmaker/internal/queue"
)

type IndexedMatcher struct {
	config      IndexedMatcherConfig
	friendships map[string]map[string]struct{}
	avoids      map[string]map[string]struct{}
	pool        *queue.MatchmakingPool
}

func NewIndexedMatcher(config IndexedMatcherConfig, pool *queue.MatchmakingPool) *IndexedMatcher {
	if config.Now == nil {
		config.Now = time.Now
	}
	return &IndexedMatcher{
		config:      config,
		friendships: make(map[string]map[string]struct{}),
		avoids:      make(map[string]map[string]struct{}),
		pool:        pool,
	}
}

var _ Matcher = (*IndexedMatcher)(nil)
var _ PoolMatcher = (*IndexedMatcher)(nil)

func (m *IndexedMatcher) AddFriendship(playerID, friendID string) {
	addRelation(m.friendships, playerID, friendID)
	addRelation(m.friendships, friendID, playerID)
}

func (m *IndexedMatcher) AddAvoidance(playerID, avoidedPlayerID string) {
	addRelation(m.avoids, playerID, avoidedPlayerID)
}

func (m *IndexedMatcher) isFriend(playerID, friendID string) bool {
	_, exists := m.friendships[playerID][friendID]
	return exists
}

func (m *IndexedMatcher) isAvoidedByEither(playerID, otherID string) bool {
	_, first := m.avoids[playerID][otherID]
	_, second := m.avoids[otherID][playerID]
	return first || second
}

func (m *IndexedMatcher) FindMatch(entries []queue.Entry, gameMode string) Result {
	m.syncPool(entries, gameMode)
	return m.FindMatchFromPool(gameMode)
}

func (m *IndexedMatcher) FindMatchFromPool(gameMode string) Result {
	result := Result{}

	anchor := m.findOldestEligible(gameMode)
	if anchor == nil {
		return result
	}

	candidates := m.getCandidates(anchor, gameMode)
	if len(candidates) == 0 {
		return result
	}

	eligible := make([]queue.PoolEntry, 0, len(candidates))
	for _, c := range candidates {
		if reason := m.ineligiblePoolEntry(c); reason != "" {
			result.Rejections = append(result.Rejections, Rejection{PartyIDs: []string{c.ID}, Reasons: []string{reason}})
			continue
		}
		eligible = append(eligible, c)
	}

	for size := 1; size <= len(eligible); size++ {
		var found *match.Match
		var foundRejections []Rejection
		m.visitPoolCombinations(eligible, size, 0, nil, func(candidate []queue.PoolEntry) bool {
			if totalPoolPlayers(candidate) != 10 {
				return true
			}
			candidateMatch, reasons := m.tryPoolCandidate(candidate)
			if candidateMatch != nil {
				found = candidateMatch
				return false
			}
			foundRejections = append(foundRejections, Rejection{PartyIDs: poolPartyIDs(candidate), Reasons: reasons})
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

func (m *IndexedMatcher) AddParty(entry queue.PoolEntry) error {
	return m.pool.Add(entry)
}

func (m *IndexedMatcher) RemoveParty(id string) (queue.PoolEntry, bool) {
	return m.pool.Remove(id)
}

func (m *IndexedMatcher) GetPool() *queue.MatchmakingPool {
	return m.pool
}

func (m *IndexedMatcher) syncPool(entries []queue.Entry, gameMode string) {
	if m.pool == nil {
		return
	}
	for _, entry := range entries {
		if entry.GameMode != gameMode {
			continue
		}
		poolEntry := queue.PoolEntry{
			ID:         entry.ID,
			Party:      entry.Party,
			GameMode:   entry.GameMode,
			PartyElo:   entry.PartyElo,
			PartySize:  len(entry.Party.Members),
			Region:     "",
			EnqueuedAt: entry.EnqueuedAt,
		}
		existing, _ := m.pool.Get(entry.ID)
		if existing.ID == "" {
			m.pool.Add(poolEntry)
		}
	}
}

func (m *IndexedMatcher) findOldestEligible(gameMode string) *queue.PoolEntry {
	if m.pool == nil {
		return nil
	}
	entries := m.pool.GetByGameMode(gameMode)
	if len(entries) == 0 {
		return nil
	}

	var oldest *queue.PoolEntry
	for i := range entries {
		e := entries[i]
		if reason := m.ineligiblePoolEntry(e); reason != "" {
			continue
		}
		if oldest == nil || e.EnqueuedAt.Before(oldest.EnqueuedAt) {
			oldest = &entries[i]
		}
	}
	return oldest
}

func (m *IndexedMatcher) getCandidates(anchor *queue.PoolEntry, gameMode string) []queue.PoolEntry {
	if m.pool == nil {
		return nil
	}

	minElo, maxElo := m.config.SearchPolicy.EloRange(anchor.PartyElo, anchor.EnqueuedAt)
	return m.pool.GetByGameModeAndEloRange(gameMode, minElo, maxElo)
}

func (m *IndexedMatcher) ineligiblePoolEntry(entry queue.PoolEntry) string {
	for _, member := range entry.Party.Members {
		if member.Presence != player.Online {
			return fmt.Sprintf("party contains player %q who is not online", member.ID)
		}
	}
	return ""
}

func (m *IndexedMatcher) tryPoolCandidate(candidate []queue.PoolEntry) (*match.Match, []string) {
	if reason := m.relationshipConflictPool(candidate); reason != "" {
		return nil, []string{reason}
	}

	for mask := 1; mask < (1<<len(candidate))-1; mask++ {
		teamAEntries, teamBEntries := splitPoolCandidate(candidate, mask)
		if totalPoolPlayers(teamAEntries) != 5 || totalPoolPlayers(teamBEntries) != 5 {
			continue
		}

		teamAPlayers := flattenPool(teamAEntries)
		teamBPlayers := flattenPool(teamBEntries)
		if spread(teamAPlayers) > m.config.WithinTeamMaxSpread || spread(teamBPlayers) > m.config.WithinTeamMaxSpread {
			continue
		}

		teamAElo := averageElo(teamAPlayers)
		teamBElo := averageElo(teamBPlayers)
		if math.Abs(teamAElo-teamBElo) > m.allowedTeamGapPool(candidate) {
			continue
		}

		return &match.Match{
			ID:            fmt.Sprintf("match-%d", m.config.Now().UnixNano()),
			GameMode:      candidate[0].GameMode,
			TeamAElo:      teamAElo,
			TeamBElo:      teamBElo,
			EloDifference: math.Abs(teamAElo - teamBElo),
			TeamA:         match.NewTeam(teamAPlayers),
			TeamB:         match.NewTeam(teamBPlayers),
			Players:       matchPoolPlayers(teamAEntries, teamBEntries),
			CreatedAt:     m.config.Now(),
		}, nil
	}

	return nil, []string{"no valid 5v5 partition satisfies the ELO constraints"}
}

func (m *IndexedMatcher) allowedTeamGapPool(candidate []queue.PoolEntry) float64 {
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

func (m *IndexedMatcher) relationshipConflictPool(entries []queue.PoolEntry) string {
	players := flattenPool(entries)
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

func (m *IndexedMatcher) visitPoolCombinations(entries []queue.PoolEntry, size, start int, current []queue.PoolEntry, visit func([]queue.PoolEntry) bool) bool {
	if len(current) == size {
		return visit(append([]queue.PoolEntry(nil), current...))
	}
	for i := start; i <= len(entries)-(size-len(current)); i++ {
		if !m.visitPoolCombinations(entries, size, i+1, append(current, entries[i]), visit) {
			return false
		}
	}
	return true
}

func splitPoolCandidate(candidate []queue.PoolEntry, mask int) ([]queue.PoolEntry, []queue.PoolEntry) {
	teamA, teamB := make([]queue.PoolEntry, 0), make([]queue.PoolEntry, 0)
	for i, entry := range candidate {
		if mask&(1<<i) != 0 {
			teamA = append(teamA, entry)
		} else {
			teamB = append(teamB, entry)
		}
	}
	return teamA, teamB
}

func flattenPool(entries []queue.PoolEntry) []player.Player {
	players := make([]player.Player, 0)
	for _, entry := range entries {
		players = append(players, entry.Party.Members...)
	}
	return players
}

func totalPoolPlayers(entries []queue.PoolEntry) int {
	total := 0
	for _, entry := range entries {
		total += entry.PartySize
	}
	return total
}

func matchPoolPlayers(teamA, teamB []queue.PoolEntry) []match.Player {
	players := make([]match.Player, 0, 10)
	for _, entry := range teamA {
		for _, member := range entry.Party.Members {
			players = append(players, match.Player{PlayerID: member.ID, PartyID: entry.ID, Team: match.TeamA, Elo: member.Elo})
		}
	}
	for _, entry := range teamB {
		for _, member := range entry.Party.Members {
			players = append(players, match.Player{PlayerID: member.ID, PartyID: entry.ID, Team: match.TeamB, Elo: member.Elo})
		}
	}
	return players
}

func poolPartyIDs(entries []queue.PoolEntry) []string {
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.ID)
	}
	return ids
}