package matchmaking

import (
	"time"

	"player-matchmaker/internal/queue"
)

// PoolMatcher defines the interface for pool-based matchmaking (V2).
// This separates V2's pool-based approach from V1's entry-based approach.
type PoolMatcher interface {
	// FindMatchFromPool finds a match using the internal matchmaking pool.
	// This is the primary V2 entry point that uses indexed candidate retrieval.
	FindMatchFromPool(gameMode string) Result

	// AddParty adds a party to the matchmaking pool.
	AddParty(entry queue.PoolEntry) error

	// RemoveParty removes a party from the matchmaking pool.
	RemoveParty(id string) (queue.PoolEntry, bool)

	// GetPool returns the underlying matchmaking pool for inspection.
	GetPool() *queue.MatchmakingPool
}

// IndexedMatcherConfig holds configuration for the indexed matcher.
type IndexedMatcherConfig struct {
	WithinTeamMaxSpread float64
	BaseTeamEloGap      float64
	GrowthPerMinute     float64
	MaxAdditionalGap    float64
	Now                 func() time.Time
	SearchPolicy        queue.SearchPolicy
}

func DefaultIndexedMatcherConfig() IndexedMatcherConfig {
	cfg := DefaultConfig()
	return IndexedMatcherConfig{
		WithinTeamMaxSpread: cfg.WithinTeamMaxSpread,
		BaseTeamEloGap:      cfg.BaseTeamEloGap,
		GrowthPerMinute:     cfg.GrowthPerMinute,
		MaxAdditionalGap:    cfg.MaxAdditionalGap,
		Now:                 cfg.Now,
		SearchPolicy:        queue.DefaultSearchPolicy(),
	}
}