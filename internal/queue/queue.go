package queue

import (
	"errors"
	"fmt"
	"time"

	"player-matchmaker/internal/party"
)

type Entry struct {
	ID         string
	Party      party.Party
	GameMode   string
	PartyElo   float64
	EnqueuedAt time.Time
}

type Queue struct {
	entries []Entry
}

func NewEntry(id string, p party.Party, enqueuedAt time.Time) (Entry, error) {
	if id == "" {
		return Entry{}, errors.New("queue entry id is required")
	}
	if err := p.Validate(); err != nil {
		return Entry{}, fmt.Errorf("invalid queued party: %w", err)
	}
	elo, err := p.Elo()
	if err != nil {
		return Entry{}, err
	}
	return Entry{
		ID:         id,
		Party:      p,
		GameMode:   p.GameMode,
		PartyElo:   elo,
		EnqueuedAt: enqueuedAt,
	}, nil
}

func (q *Queue) Enqueue(entry Entry) {
	q.entries = append(q.entries, entry)
}

func (q Queue) Entries() []Entry {
	return append([]Entry(nil), q.entries...)
}
