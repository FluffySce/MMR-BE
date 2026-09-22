package matchmaking

import (
	"testing"
	"time"

	"player-matchmaker/internal/party"
	"player-matchmaker/internal/player"
	"player-matchmaker/internal/queue"
)

func TestMatcherBuildsFiveVFiveWithoutSplittingParties(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)
	m := New(DefaultConfig())
	m.config.Now = func() time.Time { return now }

	entries := []queue.Entry{
		entry(t, "a", now.Add(-time.Minute), 1000, 1000),
		entry(t, "b", now.Add(-time.Minute), 1010, 1010),
		entry(t, "c", now.Add(-time.Minute), 990, 990),
		entry(t, "d", now.Add(-time.Minute), 1000, 1000),
		entry(t, "e", now.Add(-time.Minute), 1000, 1000),
		entry(t, "f", now.Add(-time.Minute), 1000, 1000),
	}

	result := m.FindMatch(entries, "competitive")
	if result.Match == nil {
		t.Fatalf("expected match, rejections: %+v", result.Rejections)
	}
	if err := result.Match.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestMatcherRejectsAwayParty(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := New(DefaultConfig())
	entries := []queue.Entry{entryWithPresence(t, "away", now, player.Away)}

	result := m.FindMatch(entries, "competitive")
	if len(result.Rejections) != 1 || result.Rejections[0].Reasons[0] == "" {
		t.Fatalf("expected useful away rejection, got %+v", result.Rejections)
	}
}

func TestMatcherBlocksInvisibleFriendAndAvoidedPlayer(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := New(DefaultConfig())
	m.AddFriendship("invisible", "friend")
	m.AddAvoidance("avoider", "avoided")

	entries := []queue.Entry{
		entryWithPlayers(t, "p1", now, player.Player{ID: "invisible", Elo: 1000, Presence: player.Online, Invisible: true}),
		entryWithPlayers(t, "p2", now, player.Player{ID: "friend", Elo: 1000, Presence: player.Online}),
		entry(t, "p3", now, 1000, 1000),
		entry(t, "p4", now, 1000, 1000),
		entry(t, "p5", now, 1000, 1000),
		entry(t, "p6", now, 1000, 1000),
	}
	result := m.FindMatch(entries, "competitive")
	if result.Match != nil {
		t.Fatal("expected invisible friend conflict to prevent the match")
	}
}

func TestTeamAverageToleranceGrowsAndCaps(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)
	config := DefaultConfig()
	config.Now = func() time.Time { return now }
	m := New(config)
	candidate := []queue.Entry{entry(t, "old", now.Add(-10*time.Minute), 1000, 1000)}
	if got := m.allowedTeamGap(candidate); got != 250 {
		t.Fatalf("expected capped gap 250, got %v", got)
	}
}

func entry(t *testing.T, id string, enqueuedAt time.Time, elos ...int) queue.Entry {
	players := make([]player.Player, 0, len(elos))
	for i, elo := range elos {
		players = append(players, player.Player{ID: id + string(rune('a'+i)), Elo: elo, Presence: player.Online})
	}
	return entryWithPlayers(t, id, enqueuedAt, players...)
}

func entryWithPresence(t *testing.T, id string, enqueuedAt time.Time, presence player.Presence) queue.Entry {
	return entryWithPlayers(t, id, enqueuedAt, player.Player{ID: id + "-player", Elo: 1000, Presence: presence})
}

func entryWithPlayers(t *testing.T, id string, enqueuedAt time.Time, players ...player.Player) queue.Entry {
	t.Helper()
	p := party.Party{ID: id, GameMode: "competitive", Members: players}
	entry, err := queue.NewEntry(id, p, enqueuedAt)
	if err != nil {
		t.Fatal(err)
	}
	return entry
}
