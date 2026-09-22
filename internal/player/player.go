package player

import (
	"errors"
	"time"
)

type Presence string

const (
	Online  Presence = "ONLINE"
	Away    Presence = "AWAY"
	Offline Presence = "OFFLINE"
)

type Player struct {
	ID        string
	Elo       int
	Rank      string
	Presence  Presence
	Invisible bool
	CreatedAt time.Time
}

func (p Player) Validate() error {
	if p.ID == "" {
		return errors.New("player id is required")
	}
	if p.Elo < 0 {
		return errors.New("player elo cannot be negative")
	}
	return nil
}
