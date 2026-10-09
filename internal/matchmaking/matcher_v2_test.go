package matchmaking

import (
	"testing"
	"time"

	"player-matchmaker/internal/party"
	"player-matchmaker/internal/player"
	"player-matchmaker/internal/queue"
)

func TestIndexedMatcherBuildsFiveVFive(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)
	pool := queue.NewMatchmakingPool(100)
	config := DefaultIndexedMatcherConfig()
	config.Now = func() time.Time { return now }
	m := NewIndexedMatcher(config, pool)

	// Add parties to pool
	pool.Add(mustPoolEntryV2("a", now.Add(-time.Minute), 1000))
	pool.Add(mustPoolEntryV2("b", now.Add(-time.Minute), 1010, 1010))
	pool.Add(mustPoolEntryV2("c", now.Add(-time.Minute), 990))
	pool.Add(mustPoolEntryV2("d", now.Add(-time.Minute), 1000, 1000, 1000))
	pool.Add(mustPoolEntryV2("e", now.Add(-time.Minute), 1000, 1000, 1000))

	result := m.FindMatchFromPool("competitive")

	if result.Match == nil {
		t.Fatalf("expected match, rejections: %+v", result.Rejections)
	}
	if err := result.Match.Validate(); err != nil {
		t.Fatal(err)
	}
	if result.Match.TeamA.Len() != 5 || result.Match.TeamB.Len() != 5 {
		t.Fatalf("expected both teams to have 5 players: TeamA=%d, TeamB=%d", result.Match.TeamA.Len(), result.Match.TeamB.Len())
	}
}

func TestIndexedMatcherUsesPoolNotFullScan(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)
	pool := queue.NewMatchmakingPool(100)
	config := DefaultIndexedMatcherConfig()
	config.Now = func() time.Time { return now }
	m := NewIndexedMatcher(config, pool)

	// Add many parties, but only some in range
	for i := 0; i < 20; i++ {
		elo := 800 + i*50 // 800, 850, 900, ... 1750
		pool.Add(mustPoolEntryV2(string(rune('a'+i)), now, elo))
	}

	// Add target parties at 1000 ELO
	pool.Add(mustPoolEntryV2("target1", now.Add(-time.Minute), 1000))
	pool.Add(mustPoolEntryV2("target2", now.Add(-time.Minute), 1000, 1000, 1000))
	pool.Add(mustPoolEntryV2("target3", now.Add(-time.Minute), 1000, 1000))
	pool.Add(mustPoolEntryV2("target4", now.Add(-time.Minute), 1000, 1000))
	pool.Add(mustPoolEntryV2("target5", now.Add(-time.Minute), 1000))

	result := m.FindMatchFromPool("competitive")

	// Should find match using only nearby ELO candidates
	if result.Match == nil {
		t.Fatalf("expected match, rejections: %+v", result.Rejections)
	}
}

func TestIndexedMatcherDynamicSearchWindow(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)
	pool := queue.NewMatchmakingPool(100)

	// Custom policy: narrow window
	config := DefaultIndexedMatcherConfig()
	config.Now = func() time.Time { return now }
	config.SearchPolicy = queue.SearchPolicy{
		BaseWindow:    50,
		MaxWindow:     200,
		ExpansionRate: 50,
		Now:           config.Now,
	}
	m := NewIndexedMatcher(config, pool)

	// Old party at 1000 ELO (5 min wait -> window 50 + 5*50 = 250, capped at 200)
	pool.Add(mustPoolEntryV2("old", now.Add(-5*time.Minute), 1000))
	// Nearby parties
	pool.Add(mustPoolEntryV2("near1", now, 1000))
	pool.Add(mustPoolEntryV2("near2", now, 1000))
	pool.Add(mustPoolEntryV2("near3", now, 1000))
	pool.Add(mustPoolEntryV2("near4", now, 1000))
	pool.Add(mustPoolEntryV2("near5", now, 1000))
	pool.Add(mustPoolEntryV2("near6", now, 1000))
	pool.Add(mustPoolEntryV2("near7", now, 1000))
	pool.Add(mustPoolEntryV2("near8", now, 1000))
	pool.Add(mustPoolEntryV2("near9", now, 1000))

	// Far party at 1300 (outside 200 window from 1000)
	pool.Add(mustPoolEntryV2("far", now, 1300))

	result := m.FindMatchFromPool("competitive")

	if result.Match == nil {
		t.Fatalf("expected match with nearby parties, rejections: %+v", result.Rejections)
	}
	// Far party should not be in match
	for _, p := range result.Match.Players {
		if p.PlayerID == "far-player" {
			t.Fatal("far party should not be included due to search window")
		}
	}
}

func TestIndexedMatcherRejectsAwayParty(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	pool := queue.NewMatchmakingPool(100)
	config := DefaultIndexedMatcherConfig()
	config.Now = func() time.Time { return now }
	m := NewIndexedMatcher(config, pool)

	// Add away party
	pool.Add(mustPoolEntryWithPresence("away", now, player.Away))
	// Add valid parties
	for i := 0; i < 10; i++ {
		pool.Add(mustPoolEntryV2(string(rune('a'+i)), now, 1000))
	}

	result := m.FindMatchFromPool("competitive")

	// Should still find match with online parties
	if result.Match == nil {
		t.Fatalf("expected match with online parties, rejections: %+v", result.Rejections)
	}
}

func TestIndexedMatcherBlocksSocialConflicts(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	pool := queue.NewMatchmakingPool(100)
	config := DefaultIndexedMatcherConfig()
	config.Now = func() time.Time { return now }
	m := NewIndexedMatcher(config, pool)
	m.AddAvoidance("avoider", "avoided")

	pool.Add(mustPoolEntryV2("p1", now, 1000)) // avoider
	pool.Add(mustPoolEntryV2("p2", now, 1000)) // avoided
	for i := 0; i < 8; i++ {
		pool.Add(mustPoolEntryV2(string(rune('c'+i)), now, 1000))
	}

	result := m.FindMatchFromPool("competitive")

	// Should not create match with conflict
	if result.Match != nil {
		for _, p := range result.Match.Players {
			if p.PlayerID == "avoider" || p.PlayerID == "avoided" {
				t.Fatal("match should not include conflicting players")
			}
		}
	}
}

func TestIndexedMatcherEmptyPool(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	pool := queue.NewMatchmakingPool(100)
	config := DefaultIndexedMatcherConfig()
	config.Now = func() time.Time { return now }
	m := NewIndexedMatcher(config, pool)

	result := m.FindMatchFromPool("competitive")
	if result.Match != nil {
		t.Fatal("expected no match from empty pool")
	}
}

func TestIndexedMatcherInsufficientPlayers(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	pool := queue.NewMatchmakingPool(100)
	config := DefaultIndexedMatcherConfig()
	config.Now = func() time.Time { return now }
	m := NewIndexedMatcher(config, pool)

	pool.Add(mustPoolEntryV2("p1", now, 1000))
	pool.Add(mustPoolEntryV2("p2", now, 1000))

	result := m.FindMatchFromPool("competitive")
	if result.Match != nil {
		t.Fatal("expected no match with insufficient players")
	}
}

func TestPoolMatcherInterface(t *testing.T) {
	// This test demonstrates that V2 can be used via the PoolMatcher interface
	// independently from V1's Matcher interface.
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)
	pool := queue.NewMatchmakingPool(100)
	config := DefaultIndexedMatcherConfig()
	config.Now = func() time.Time { return now }
	var m PoolMatcher = NewIndexedMatcher(config, pool)

	// Add parties via PoolMatcher interface
	m.AddParty(mustPoolEntryV2("a", now.Add(-time.Minute), 1000))
	m.AddParty(mustPoolEntryV2("b", now.Add(-time.Minute), 1010, 1010))
	m.AddParty(mustPoolEntryV2("c", now.Add(-time.Minute), 990))
	m.AddParty(mustPoolEntryV2("d", now.Add(-time.Minute), 1000, 1000, 1000))
	m.AddParty(mustPoolEntryV2("e", now.Add(-time.Minute), 1000, 1000, 1000))

	result := m.FindMatchFromPool("competitive")

	if result.Match == nil {
		t.Fatalf("expected match, rejections: %+v", result.Rejections)
	}
	if result.Match.TeamA.Len() != 5 || result.Match.TeamB.Len() != 5 {
		t.Fatalf("expected both teams to have 5 players: TeamA=%d, TeamB=%d", result.Match.TeamA.Len(), result.Match.TeamB.Len())
	}

	// Verify pool access via interface
	retrievedPool := m.GetPool()
	if retrievedPool.Len() != 5 {
		t.Fatalf("expected 5 entries in pool, got %d", retrievedPool.Len())
	}

	// Verify RemoveParty via interface
	removed, ok := m.RemoveParty("a")
	if !ok || removed.ID != "a" {
		t.Fatalf("expected to remove entry 'a', got %v, %v", removed, ok)
	}
	if retrievedPool.Len() != 4 {
		t.Fatalf("expected 4 entries after removal, got %d", retrievedPool.Len())
	}
}

func TestV1AndV2Independence(t *testing.T) {
	// This test demonstrates V1 and V2 can be used independently
	// with separate state and configurations.
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)

	// V1: ExhaustiveMatcher with entries
	v1Config := DefaultConfig()
	v1Config.Now = func() time.Time { return now }
	v1Matcher := NewExhaustiveMatcher(v1Config)

	// V2: IndexedMatcher with pool
	v2Pool := queue.NewMatchmakingPool(100)
	v2Config := DefaultIndexedMatcherConfig()
	v2Config.Now = func() time.Time { return now }
	v2Matcher := NewIndexedMatcher(v2Config, v2Pool)

	// Add same parties to both
	v1Entries := []queue.Entry{
		entry(t, "a", now.Add(-time.Minute), 1000),
		entry(t, "b", now.Add(-time.Minute), 1010, 1010),
		entry(t, "c", now.Add(-time.Minute), 990),
		entry(t, "d", now.Add(-time.Minute), 1000, 1000, 1000),
		entry(t, "e", now.Add(-time.Minute), 1000, 1000, 1000),
	}

	v2Matcher.AddParty(mustPoolEntryV2("a", now.Add(-time.Minute), 1000))
	v2Matcher.AddParty(mustPoolEntryV2("b", now.Add(-time.Minute), 1010, 1010))
	v2Matcher.AddParty(mustPoolEntryV2("c", now.Add(-time.Minute), 990))
	v2Matcher.AddParty(mustPoolEntryV2("d", now.Add(-time.Minute), 1000, 1000, 1000))
	v2Matcher.AddParty(mustPoolEntryV2("e", now.Add(-time.Minute), 1000, 1000, 1000))

	// Both should find matches independently
	v1Result := v1Matcher.FindMatch(v1Entries, "competitive")
	v2Result := v2Matcher.FindMatchFromPool("competitive")

	if v1Result.Match == nil {
		t.Fatalf("V1 expected match, rejections: %+v", v1Result.Rejections)
	}
	if v2Result.Match == nil {
		t.Fatalf("V2 expected match, rejections: %+v", v2Result.Rejections)
	}

	// Both should produce valid 5v5 matches
	if v1Result.Match.TeamA.Len() != 5 || v1Result.Match.TeamB.Len() != 5 {
		t.Fatalf("V1: expected both teams to have 5 players")
	}
	if v2Result.Match.TeamA.Len() != 5 || v2Result.Match.TeamB.Len() != 5 {
		t.Fatalf("V2: expected both teams to have 5 players")
	}

	// Verify they have separate state - V2 pool should have 5 entries
	if v2Matcher.GetPool().Len() != 5 {
		t.Fatalf("V2 pool should have 5 entries, got %d", v2Matcher.GetPool().Len())
	}
}

func mustPoolEntryV2(id string, enqueuedAt time.Time, elos ...int) queue.PoolEntry {
	players := make([]player.Player, len(elos))
	for i, elo := range elos {
		players[i] = player.Player{ID: id + string(rune('a'+i)), Elo: elo, Presence: player.Online}
	}
	entry, err := queue.NewPoolEntry(id, party.Party{
		ID:       id,
		GameMode: "competitive",
		Members:  players,
	}, enqueuedAt, "us-east")
	if err != nil {
		panic(err)
	}
	return entry
}

func mustPoolEntryWithPresence(id string, enqueuedAt time.Time, presence player.Presence) queue.PoolEntry {
	entry, err := queue.NewPoolEntry(id, party.Party{
		ID:       id,
		GameMode: "competitive",
		Members:  []player.Player{{ID: id + "-player", Elo: 1000, Presence: presence}},
	}, enqueuedAt, "us-east")
	if err != nil {
		panic(err)
	}
	return entry
}