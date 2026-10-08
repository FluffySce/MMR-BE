package queue

import (
	"testing"
	"time"

	"player-matchmaker/internal/party"
	"player-matchmaker/internal/player"
)

func TestMatchmakingPoolAddAndGet(t *testing.T) {
	pool := NewMatchmakingPool(100)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	entry, err := NewPoolEntry("entry-1", party.Party{
		ID:       "party-1",
		GameMode: "competitive",
		Members: []player.Player{
			{ID: "p1", Elo: 1000, Presence: player.Online},
			{ID: "p2", Elo: 1200, Presence: player.Online},
		},
	}, now, "us-east")
	if err != nil {
		t.Fatal(err)
	}

	if err := pool.Add(entry); err != nil {
		t.Fatal(err)
	}

	got, ok := pool.Get("entry-1")
	if !ok {
		t.Fatal("expected entry to exist")
	}
	if got.ID != "entry-1" || got.PartyElo != 1100 || got.GameMode != "competitive" || got.Region != "us-east" {
		t.Fatalf("unexpected entry: %+v", got)
	}
	if !got.EnqueuedAt.Equal(now) {
		t.Fatalf("queue timestamp not preserved: %v", got.EnqueuedAt)
	}
}

func TestMatchmakingPoolRemove(t *testing.T) {
	pool := NewMatchmakingPool(100)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	entry, _ := NewPoolEntry("entry-1", party.Party{
		ID:       "party-1",
		GameMode: "competitive",
		Members: []player.Player{{ID: "p1", Elo: 1000, Presence: player.Online}},
	}, now, "us-east")
	pool.Add(entry)

	removed, ok := pool.Remove("entry-1")
	if !ok {
		t.Fatal("expected entry to be removed")
	}
	if removed.ID != "entry-1" {
		t.Fatalf("wrong entry removed: %+v", removed)
	}

	_, ok = pool.Get("entry-1")
	if ok {
		t.Fatal("entry should not exist after removal")
	}
}

func TestMatchmakingPoolGameModeIndex(t *testing.T) {
	pool := NewMatchmakingPool(100)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	pool.Add(mustPoolEntry("comp-1", "competitive", 1000, now, "us-east"))
	pool.Add(mustPoolEntry("comp-2", "competitive", 1200, now, "us-east"))
	pool.Add(mustPoolEntry("casual-1", "casual", 1000, now, "us-east"))

	comp := pool.GetByGameMode("competitive")
	if len(comp) != 2 {
		t.Fatalf("expected 2 competitive entries, got %d", len(comp))
	}

	casual := pool.GetByGameMode("casual")
	if len(casual) != 1 {
		t.Fatalf("expected 1 casual entry, got %d", len(casual))
	}
}

func TestMatchmakingPoolEloBucketIndex(t *testing.T) {
	pool := NewMatchmakingPool(100)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	pool.Add(mustPoolEntry("low", "competitive", 950, now, "us-east"))  // bucket 9
	pool.Add(mustPoolEntry("mid", "competitive", 1050, now, "us-east")) // bucket 10
	pool.Add(mustPoolEntry("high", "competitive", 1150, now, "us-east")) // bucket 11

	bucket9 := pool.GetByEloBucket(9)
	if len(bucket9) != 1 || bucket9[0].ID != "low" {
		t.Fatalf("bucket 9: expected [low], got %v", bucket9)
	}

	bucket10 := pool.GetByEloBucket(10)
	if len(bucket10) != 1 || bucket10[0].ID != "mid" {
		t.Fatalf("bucket 10: expected [mid], got %v", bucket10)
	}

	bucket11 := pool.GetByEloBucket(11)
	if len(bucket11) != 1 || bucket11[0].ID != "high" {
		t.Fatalf("bucket 11: expected [high], got %v", bucket11)
	}
}

func TestMatchmakingPoolPartySizeIndex(t *testing.T) {
	pool := NewMatchmakingPool(100)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	pool.Add(mustPoolEntryWithSize("solo", "competitive", 1000, now, "us-east", 1))     // size 1
	pool.Add(mustPoolEntryWithSize("duo", "competitive", 1000, now, "us-east", 2))      // size 2
	pool.Add(mustPoolEntryWithSize("trio", "competitive", 1000, now, "us-east", 3))     // size 3

	size1 := pool.GetByPartySize(1)
	if len(size1) != 1 || size1[0].ID != "solo" {
		t.Fatalf("size 1: expected [solo], got %v", size1)
	}

	size2 := pool.GetByPartySize(2)
	if len(size2) != 1 || size2[0].ID != "duo" {
		t.Fatalf("size 2: expected [duo], got %v", size2)
	}

	size3 := pool.GetByPartySize(3)
	if len(size3) != 1 || size3[0].ID != "trio" {
		t.Fatalf("size 3: expected [trio], got %v", size3)
	}
}

func TestMatchmakingPoolRegionIndex(t *testing.T) {
	pool := NewMatchmakingPool(100)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	pool.Add(mustPoolEntry("us-1", "competitive", 1000, now, "us-east"))
	pool.Add(mustPoolEntry("us-2", "competitive", 1000, now, "us-west"))
	pool.Add(mustPoolEntry("eu-1", "competitive", 1000, now, "eu-west"))

	usEast := pool.GetByRegion("us-east")
	if len(usEast) != 1 || usEast[0].ID != "us-1" {
		t.Fatalf("us-east: expected [us-1], got %v", usEast)
	}

	usWest := pool.GetByRegion("us-west")
	if len(usWest) != 1 || usWest[0].ID != "us-2" {
		t.Fatalf("us-west: expected [us-2], got %v", usWest)
	}
}

func TestMatchmakingPoolEloRange(t *testing.T) {
	pool := NewMatchmakingPool(100)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	pool.Add(mustPoolEntry("low", "competitive", 900, now, "us-east"))
	pool.Add(mustPoolEntry("mid", "competitive", 1000, now, "us-east"))
	pool.Add(mustPoolEntry("high", "competitive", 1100, now, "us-east"))
	pool.Add(mustPoolEntry("very-high", "competitive", 1200, now, "us-east"))

	inRange := pool.GetByEloRange(950, 1150)
	if len(inRange) != 2 {
		t.Fatalf("expected 2 entries in range 950-1150, got %d: %v", len(inRange), inRange)
	}
	for _, e := range inRange {
		if e.PartyElo < 950 || e.PartyElo > 1150 {
			t.Fatalf("entry %s outside range: elo=%f", e.ID, e.PartyElo)
		}
	}
}

func TestMatchmakingPoolGameModeAndEloRange(t *testing.T) {
	pool := NewMatchmakingPool(100)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	pool.Add(mustPoolEntry("comp-low", "competitive", 900, now, "us-east"))
	pool.Add(mustPoolEntry("comp-mid", "competitive", 1000, now, "us-east"))
	pool.Add(mustPoolEntry("casual-mid", "casual", 1000, now, "us-east"))

	inRange := pool.GetByGameModeAndEloRange("competitive", 950, 1050)
	if len(inRange) != 1 || inRange[0].ID != "comp-mid" {
		t.Fatalf("expected [comp-mid], got %v", inRange)
	}
}

func TestMatchmakingPoolIndexesConsistentAfterRemove(t *testing.T) {
	pool := NewMatchmakingPool(100)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	pool.Add(mustPoolEntry("e1", "competitive", 1000, now, "us-east"))
	pool.Add(mustPoolEntry("e2", "competitive", 1000, now, "us-east"))

	pool.Remove("e1")

	comp := pool.GetByGameMode("competitive")
	if len(comp) != 1 || comp[0].ID != "e2" {
		t.Fatalf("game mode index inconsistent after remove: %v", comp)
	}

	bucket10 := pool.GetByEloBucket(10)
	if len(bucket10) != 1 || bucket10[0].ID != "e2" {
		t.Fatalf("elo bucket index inconsistent after remove: %v", bucket10)
	}

	size1 := pool.GetByPartySize(1)
	if len(size1) != 1 || size1[0].ID != "e2" {
		t.Fatalf("party size index inconsistent after remove: %v", size1)
	}

	usEast := pool.GetByRegion("us-east")
	if len(usEast) != 1 || usEast[0].ID != "e2" {
		t.Fatalf("region index inconsistent after remove: %v", usEast)
	}
}

func TestMatchmakingPoolEmptyBucket(t *testing.T) {
	pool := NewMatchmakingPool(100)
	bucket := pool.GetByEloBucket(999)
	if bucket != nil {
		t.Fatalf("expected nil for empty bucket, got %v", bucket)
	}
}

func TestMatchmakingPoolEloBucketBoundaries(t *testing.T) {
	pool := NewMatchmakingPool(100)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	pool.Add(mustPoolEntry("exact-1000", "competitive", 1000, now, "us-east"))
	pool.Add(mustPoolEntry("exact-1100", "competitive", 1100, now, "us-east"))

	bucket10 := pool.GetByEloBucket(10)
	if len(bucket10) != 1 || bucket10[0].ID != "exact-1000" {
		t.Fatalf("boundary 1000: expected [exact-1000], got %v", bucket10)
	}

	bucket11 := pool.GetByEloBucket(11)
	if len(bucket11) != 1 || bucket11[0].ID != "exact-1100" {
		t.Fatalf("boundary 1100: expected [exact-1100], got %v", bucket11)
	}
}

func TestMatchmakingPoolEloOutsideNormalRange(t *testing.T) {
	pool := NewMatchmakingPool(100)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Very low ELO
	pool.Add(mustPoolEntry("very-low", "competitive", 100, now, "us-east"))
	// Very high ELO
	pool.Add(mustPoolEntry("very-high", "competitive", 3000, now, "us-east"))

	lowBucket := pool.GetByEloBucket(1)
	if len(lowBucket) != 1 || lowBucket[0].ID != "very-low" {
		t.Fatalf("very low ELO: expected [very-low], got %v", lowBucket)
	}

	highBucket := pool.GetByEloBucket(30)
	if len(highBucket) != 1 || highBucket[0].ID != "very-high" {
		t.Fatalf("very high ELO: expected [very-high], got %v", highBucket)
	}

	// Range query spanning extreme values
	inRange := pool.GetByEloRange(50, 3050)
	if len(inRange) != 2 {
		t.Fatalf("expected 2 entries in extreme range, got %d", len(inRange))
	}
}

func TestMatchmakingPoolEntryMetadataChange(t *testing.T) {
	pool := NewMatchmakingPool(100)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Add entry at ELO 1000
	entry, _ := NewPoolEntry("e1", party.Party{
		ID:       "p1",
		GameMode: "competitive",
		Members:  []player.Player{{ID: "p1", Elo: 1000, Presence: player.Online}},
	}, now, "us-east")
	pool.Add(entry)

	// Verify in bucket 10
	bucket10 := pool.GetByEloBucket(10)
	if len(bucket10) != 1 {
		t.Fatalf("expected entry in bucket 10, got %v", bucket10)
	}

	// Remove and re-add with different ELO (simulating party ELO change)
	pool.Remove("e1")
	entry, _ = NewPoolEntry("e1", party.Party{
		ID:       "p1",
		GameMode: "competitive",
		Members:  []player.Player{{ID: "p1", Elo: 1200, Presence: player.Online}},
	}, now, "us-east")
	pool.Add(entry)

	// Verify no longer in bucket 10
	bucket10 = pool.GetByEloBucket(10)
	if len(bucket10) != 0 {
		t.Fatalf("expected empty bucket 10 after ELO change, got %v", bucket10)
	}

	// Verify now in bucket 12
	bucket12 := pool.GetByEloBucket(12)
	if len(bucket12) != 1 || bucket12[0].ID != "e1" {
		t.Fatalf("expected entry in bucket 12, got %v", bucket12)
	}
}

func TestMatchmakingPoolGetAdjacentEloBuckets(t *testing.T) {
	pool := NewMatchmakingPool(100)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	pool.Add(mustPoolEntry("b8", "competitive", 850, now, "us-east"))  // bucket 8
	pool.Add(mustPoolEntry("b9", "competitive", 950, now, "us-east"))  // bucket 9
	pool.Add(mustPoolEntry("b10", "competitive", 1050, now, "us-east")) // bucket 10
	pool.Add(mustPoolEntry("b11", "competitive", 1150, now, "us-east")) // bucket 11
	pool.Add(mustPoolEntry("b12", "competitive", 1250, now, "us-east")) // bucket 12

	// Radius 0 = just center bucket
	adjacent := pool.GetAdjacentEloBuckets(10, 0)
	if len(adjacent) != 1 || adjacent[0].ID != "b10" {
		t.Fatalf("radius 0: expected [b10], got %v", adjacent)
	}

	// Radius 1 = center ± 1
	adjacent = pool.GetAdjacentEloBuckets(10, 1)
	if len(adjacent) != 3 {
		t.Fatalf("radius 1: expected 3 entries, got %d: %v", len(adjacent), adjacent)
	}
	ids := make(map[string]bool)
	for _, e := range adjacent {
		ids[e.ID] = true
	}
	if !ids["b9"] || !ids["b10"] || !ids["b11"] {
		t.Fatalf("radius 1: missing expected buckets, got %v", ids)
	}

	// Radius 2 = center ± 2
	adjacent = pool.GetAdjacentEloBuckets(10, 2)
	if len(adjacent) != 5 {
		t.Fatalf("radius 2: expected 5 entries, got %d", len(adjacent))
	}

	// Radius exceeding available buckets
	adjacent = pool.GetAdjacentEloBuckets(10, 10)
	if len(adjacent) != 5 {
		t.Fatalf("large radius: expected 5 entries (all), got %d", len(adjacent))
	}

	// Negative radius treated as 0
	adjacent = pool.GetAdjacentEloBuckets(10, -5)
	if len(adjacent) != 1 {
		t.Fatalf("negative radius: expected 1 entry, got %d", len(adjacent))
	}
}

func TestMatchmakingPoolAll(t *testing.T) {
	pool := NewMatchmakingPool(100)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	pool.Add(mustPoolEntry("e1", "competitive", 1000, now, "us-east"))
	pool.Add(mustPoolEntry("e2", "casual", 1200, now, "us-west"))

	all := pool.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(all))
	}
}

func TestMatchmakingPoolLen(t *testing.T) {
	pool := NewMatchmakingPool(100)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if pool.Len() != 0 {
		t.Fatal("expected empty pool")
	}

	pool.Add(mustPoolEntry("e1", "competitive", 1000, now, "us-east"))
	if pool.Len() != 1 {
		t.Fatal("expected 1 entry")
	}

	pool.Add(mustPoolEntry("e2", "competitive", 1000, now, "us-east"))
	if pool.Len() != 2 {
		t.Fatal("expected 2 entries")
	}

	pool.Remove("e1")
	if pool.Len() != 1 {
		t.Fatal("expected 1 entry after remove")
	}
}

func TestNewPoolEntryValidation(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	_, err := NewPoolEntry("", party.Party{ID: "p", GameMode: "competitive", Members: []player.Player{{ID: "p1", Elo: 1000, Presence: player.Online}}}, now, "us-east")
	if err == nil {
		t.Fatal("expected error for empty id")
	}

	_, err = NewPoolEntry("e1", party.Party{ID: "p", GameMode: "competitive", Members: []player.Player{{ID: "p1", Elo: -100, Presence: player.Online}}}, now, "us-east")
	if err == nil {
		t.Fatal("expected error for negative elo")
	}
}

func mustPoolEntry(id, gameMode string, elo int, enqueuedAt time.Time, region string) PoolEntry {
	entry, err := NewPoolEntry(id, party.Party{
		ID:       id,
		GameMode: gameMode,
		Members:  []player.Player{{ID: id + "-player", Elo: elo, Presence: player.Online}},
	}, enqueuedAt, region)
	if err != nil {
		panic(err)
	}
	return entry
}

func mustPoolEntryWithSize(id, gameMode string, elo int, enqueuedAt time.Time, region string, size int) PoolEntry {
	members := make([]player.Player, size)
	for i := 0; i < size; i++ {
		members[i] = player.Player{ID: id + "-player" + string(rune('a'+i)), Elo: elo, Presence: player.Online}
	}
	entry, err := NewPoolEntry(id, party.Party{
		ID:       id,
		GameMode: gameMode,
		Members:  members,
	}, enqueuedAt, region)
	if err != nil {
		panic(err)
	}
	return entry
}