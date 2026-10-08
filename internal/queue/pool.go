package queue

import (
	"errors"
	"sync"
	"time"

	"player-matchmaker/internal/party"
)

type PoolEntry struct {
	ID         string
	Party      party.Party
	GameMode   string
	PartyElo   float64
	PartySize  int
	Region     string
	EnqueuedAt time.Time
}

type MatchmakingPool struct {
	mu         sync.RWMutex
	entries    map[string]PoolEntry
	gameModeIdx map[string]map[string]struct{}
	eloIdx     map[int]map[string]struct{}
	sizeIdx    map[int]map[string]struct{}
	regionIdx  map[string]map[string]struct{}
	eloBucketSize int
}

func NewMatchmakingPool(eloBucketSize int) *MatchmakingPool {
	if eloBucketSize <= 0 {
		eloBucketSize = 100
	}
	return &MatchmakingPool{
		entries:       make(map[string]PoolEntry),
		gameModeIdx:   make(map[string]map[string]struct{}),
		eloIdx:        make(map[int]map[string]struct{}),
		sizeIdx:       make(map[int]map[string]struct{}),
		regionIdx:     make(map[string]map[string]struct{}),
		eloBucketSize: eloBucketSize,
	}
}

func (p *MatchmakingPool) Add(entry PoolEntry) error {
	if entry.ID == "" {
		return errors.New("pool entry id is required")
	}
	if err := entry.Party.Validate(); err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.entries[entry.ID]; exists {
		return errors.New("entry with this ID already exists")
	}

	p.entries[entry.ID] = entry
	p.addToIndexes(entry)
	return nil
}

func (p *MatchmakingPool) Remove(id string) (PoolEntry, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry, exists := p.entries[id]
	if !exists {
		return PoolEntry{}, false
	}

	p.removeFromIndexes(entry)
	delete(p.entries, id)
	return entry, true
}

func (p *MatchmakingPool) Get(id string) (PoolEntry, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	entry, exists := p.entries[id]
	return entry, exists
}

func (p *MatchmakingPool) GetByGameMode(gameMode string) []PoolEntry {
	p.mu.RLock()
	defer p.mu.RUnlock()

	ids, ok := p.gameModeIdx[gameMode]
	if !ok {
		return nil
	}

	result := make([]PoolEntry, 0, len(ids))
	for id := range ids {
		if entry, ok := p.entries[id]; ok {
			result = append(result, entry)
		}
	}
	return result
}

func (p *MatchmakingPool) GetByEloBucket(eloBucket int) []PoolEntry {
	p.mu.RLock()
	defer p.mu.RUnlock()

	ids, ok := p.eloIdx[eloBucket]
	if !ok {
		return nil
	}

	result := make([]PoolEntry, 0, len(ids))
	for id := range ids {
		if entry, ok := p.entries[id]; ok {
			result = append(result, entry)
		}
	}
	return result
}

func (p *MatchmakingPool) GetByPartySize(size int) []PoolEntry {
	p.mu.RLock()
	defer p.mu.RUnlock()

	ids, ok := p.sizeIdx[size]
	if !ok {
		return nil
	}

	result := make([]PoolEntry, 0, len(ids))
	for id := range ids {
		if entry, ok := p.entries[id]; ok {
			result = append(result, entry)
		}
	}
	return result
}

func (p *MatchmakingPool) GetByRegion(region string) []PoolEntry {
	p.mu.RLock()
	defer p.mu.RUnlock()

	ids, ok := p.regionIdx[region]
	if !ok {
		return nil
	}

	result := make([]PoolEntry, 0, len(ids))
	for id := range ids {
		if entry, ok := p.entries[id]; ok {
			result = append(result, entry)
		}
	}
	return result
}

func (p *MatchmakingPool) GetByEloRange(minElo, maxElo float64) []PoolEntry {
	p.mu.RLock()
	defer p.mu.RUnlock()

	minBucket := int(minElo) / p.eloBucketSize
	maxBucket := int(maxElo) / p.eloBucketSize

	var candidateIDs map[string]struct{}
	for bucket := minBucket; bucket <= maxBucket; bucket++ {
		if ids, ok := p.eloIdx[bucket]; ok {
			if candidateIDs == nil {
				candidateIDs = make(map[string]struct{})
			}
			for id := range ids {
				candidateIDs[id] = struct{}{}
			}
		}
	}

	if candidateIDs == nil {
		return nil
	}

	result := make([]PoolEntry, 0, len(candidateIDs))
	for id := range candidateIDs {
		if entry, ok := p.entries[id]; ok {
			if entry.PartyElo >= minElo && entry.PartyElo <= maxElo {
				result = append(result, entry)
			}
		}
	}
	return result
}

func (p *MatchmakingPool) GetByGameModeAndEloRange(gameMode string, minElo, maxElo float64) []PoolEntry {
	p.mu.RLock()
	defer p.mu.RUnlock()

	gameModeIDs, ok := p.gameModeIdx[gameMode]
	if !ok {
		return nil
	}

	minBucket := int(minElo) / p.eloBucketSize
	maxBucket := int(maxElo) / p.eloBucketSize

	var candidateIDs map[string]struct{}
	for bucket := minBucket; bucket <= maxBucket; bucket++ {
		if ids, ok := p.eloIdx[bucket]; ok {
			for id := range ids {
				if _, inGameMode := gameModeIDs[id]; inGameMode {
					if candidateIDs == nil {
						candidateIDs = make(map[string]struct{})
					}
					candidateIDs[id] = struct{}{}
				}
			}
		}
	}

	if candidateIDs == nil {
		return nil
	}

	result := make([]PoolEntry, 0, len(candidateIDs))
	for id := range candidateIDs {
		if entry, ok := p.entries[id]; ok {
			if entry.PartyElo >= minElo && entry.PartyElo <= maxElo {
				result = append(result, entry)
			}
		}
	}
	return result
}

func (p *MatchmakingPool) All() []PoolEntry {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]PoolEntry, 0, len(p.entries))
	for _, entry := range p.entries {
		result = append(result, entry)
	}
	return result
}

func (p *MatchmakingPool) Len() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.entries)
}

func (p *MatchmakingPool) addToIndexes(entry PoolEntry) {
	p.addToIndex(p.gameModeIdx, entry.GameMode, entry.ID)
	p.addToIndexInt(p.eloIdx, p.eloBucket(entry.PartyElo), entry.ID)
	p.addToIndexInt(p.sizeIdx, entry.PartySize, entry.ID)
	p.addToIndex(p.regionIdx, entry.Region, entry.ID)
}

func (p *MatchmakingPool) removeFromIndexes(entry PoolEntry) {
	p.removeFromIndex(p.gameModeIdx, entry.GameMode, entry.ID)
	p.removeFromIndexInt(p.eloIdx, p.eloBucket(entry.PartyElo), entry.ID)
	p.removeFromIndexInt(p.sizeIdx, entry.PartySize, entry.ID)
	p.removeFromIndex(p.regionIdx, entry.Region, entry.ID)
}

func (p *MatchmakingPool) eloBucket(elo float64) int {
	return int(elo) / p.eloBucketSize
}

func (p *MatchmakingPool) addToIndex(idx map[string]map[string]struct{}, key, id string) {
	if idx[key] == nil {
		idx[key] = make(map[string]struct{})
	}
	idx[key][id] = struct{}{}
}

func (p *MatchmakingPool) addToIndexInt(idx map[int]map[string]struct{}, key int, id string) {
	if idx[key] == nil {
		idx[key] = make(map[string]struct{})
	}
	idx[key][id] = struct{}{}
}

func (p *MatchmakingPool) removeFromIndex(idx map[string]map[string]struct{}, key, id string) {
	if m, ok := idx[key]; ok {
		delete(m, id)
		if len(m) == 0 {
			delete(idx, key)
		}
	}
}

func (p *MatchmakingPool) removeFromIndexInt(idx map[int]map[string]struct{}, key int, id string) {
	if m, ok := idx[key]; ok {
		delete(m, id)
		if len(m) == 0 {
			delete(idx, key)
		}
	}
}

func NewPoolEntry(id string, p party.Party, enqueuedAt time.Time, region string) (PoolEntry, error) {
	if id == "" {
		return PoolEntry{}, errors.New("pool entry id is required")
	}
	if err := p.Validate(); err != nil {
		return PoolEntry{}, err
	}
	elo, err := p.Elo()
	if err != nil {
		return PoolEntry{}, err
	}
	return PoolEntry{
		ID:         id,
		Party:      p,
		GameMode:   p.GameMode,
		PartyElo:   elo,
		PartySize:  len(p.Members),
		Region:     region,
		EnqueuedAt: enqueuedAt,
	}, nil
}