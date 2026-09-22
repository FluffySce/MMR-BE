package queue

import (
	"testing"
	"time"

	"player-matchmaker/internal/party"
	"player-matchmaker/internal/player"
)

func TestNewEntryStoresPartyAverageAndQueueTime(t *testing.T) {
	enqueuedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p := party.Party{ID: "party-1", Members: []player.Player{{ID: "p1", Elo: 1000}, {ID: "p2", Elo: 1200}}}
	entry, err := NewEntry("entry-1", p, enqueuedAt)
	if err != nil {
		t.Fatal(err)
	}
	if entry.PartyElo != 1100 || !entry.EnqueuedAt.Equal(enqueuedAt) {
		t.Fatalf("unexpected queue entry: %+v", entry)
	}
}
