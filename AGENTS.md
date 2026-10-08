# AGENTS.md — Player Matchmaker

## 1. Purpose

You are working on **Player Matchmaker**, a Go-based matchmaking engine for online games.

The project already has a working **Matcher V1** implementation based on exhaustive candidate search.

Your job is **not to redesign or rewrite V1**.

Your job is to progressively evolve the existing system into **Matcher V2**: an indexed, quality-aware, concurrent matchmaking system capable of efficiently searching a large dynamic pool of parties.

The central engineering question is:

> How can we efficiently produce fair 5v5 matches from a large, dynamic pool of parties while simultaneously respecting skill, party composition, wait time, and social-graph constraints?

The existing V1 implementation is the correctness baseline against which V2 must be validated.

---

# 2. Critical Rules

Before making changes, understand and preserve these rules.

### V1 already exists

Do not recreate the exhaustive matcher.

Do not replace working V1 code simply because a different implementation seems cleaner.

Do not remove V1 unless explicitly instructed.

V1 should remain available as:

- A correctness reference
- A regression baseline
- A benchmark comparison
- A fallback implementation during development

The project should eventually support conceptual separation such as:

```text
Matcher V1
    ↓
Exhaustive search
    ↓
Correctness baseline

Matcher V2
    ↓
Indexed candidate search
    ↓
Optimized matchmaking
```

### Work incrementally

Do not implement the entire V2 architecture in one change.

Each phase must:

1. Have a clearly defined goal.
2. Produce a working codebase.
3. Preserve existing tests.
4. Add tests for new behavior.
5. Avoid unnecessary dependencies.
6. Be independently reviewable.

Do not jump ahead to Redis, Kafka, PostgreSQL, UI, or distributed matchmaking unless the current phase explicitly requires it.

### Prefer simple infrastructure first

The initial V2 implementation should remain in-process and in-memory where possible.

Do not introduce Redis or Kafka merely because the final architecture may eventually use them.

The progression is:

```text
In-memory V2
    ↓
Measure
    ↓
Identify actual bottlenecks
    ↓
Introduce distributed infrastructure only where justified
```

---

# 3. Existing V1

V1 uses exhaustive matchmaking.

Conceptually:

```text
Eligible parties
    ↓
Generate party combinations
    ↓
Find combinations totaling 10 players
    ↓
Generate 5v5 partitions
    ↓
Validate constraints
    ↓
Return valid match
```

V1 is intentionally computationally expensive.

It must remain available because it provides the reference behavior for V2.

When modifying matchmaking behavior, ask:

> Does this change alter a hard correctness rule, or does it only change how candidates are discovered and ranked?

Candidate discovery and optimization should not silently change domain semantics.

---

# 4. V2 Goal

V2 should evolve the matcher from:

```text
Search everything
    ↓
Validate everything
```

into:

```text
Identify relevant candidates
    ↓
Search a much smaller space
    ↓
Validate
    ↓
Score
    ↓
Select the best valid match
```

V2 has four major improvements:

1. Candidate indexing
2. Dynamic ELO search windows
3. Match-quality scoring
4. Safe concurrent reservation

These should be implemented separately.

---

# 5. V2 Implementation Phases

Implement the following phases in order.

Do not combine unrelated phases into one large refactor.

---

## Phase V2.1 — Establish V1/V2 Boundaries

### Objective

Create a clean separation between the existing exhaustive matcher and the new V2 matcher.

### Tasks

- Inspect the current matchmaking package.
- Identify the existing V1 entry point.
- Extract interfaces only where necessary.
- Introduce a clear matcher abstraction if one does not already exist.
- Keep V1 behavior unchanged.
- Make it possible for tests to execute V1 and V2 independently.

The conceptual interface should be similar to:

```go
type Matcher interface {
    FindMatch(ctx context.Context) (*match.Match, error)
}
```

Do not blindly introduce an interface if the existing architecture does not benefit from it.

The goal is separation, not abstraction for its own sake.

### Acceptance Criteria

- V1 continues passing all existing tests.
- V1 can still be invoked independently.
- V2 has a separate implementation boundary.
- No matchmaking behavior has been changed yet.

---

# Phase V2.2 — Introduce the Matchmaking Pool

### Objective

Replace the conceptual FIFO-only queue used by the matcher with an indexed matchmaking pool.

The pool should represent active matchmaking state.

It should not become a replacement for PostgreSQL or another durable database.

### The pool should support

- Queued parties
- Queue timestamps
- Game mode
- Party size
- ELO metadata
- Region
- Candidate indexes

Conceptually:

```text
MatchmakingPool
├── Entries
├── GameMode index
├── ELO index
├── PartySize index
└── Region index
```

### Important

Do not duplicate authoritative player/party state unnecessarily.

The matchmaking pool should contain the information needed for fast candidate discovery.

### Acceptance Criteria

- Parties can be added to the matchmaking pool.
- Parties can be removed.
- Parties can be looked up by ID.
- Queue timestamps are preserved.
- Relevant indexes remain consistent with pool membership.
- Existing queue behavior remains correct.
- Unit tests cover index insertion/removal/update.

---

# Phase V2.3 — ELO Bucketing

### Objective

Introduce ELO buckets to reduce the search space.

Example:

```text
1200–1300
1300–1400
1400–1500
1500–1600
...
```

The bucket size must be configurable.

Do not hard-code assumptions that make the algorithm impossible to tune later.

### Requirements

A party should be retrievable from the appropriate ELO bucket(s).

The implementation must correctly handle:

- Boundary ELO values
- ELO values outside normal ranges
- Bucket transitions
- Parties whose metadata changes
- Empty buckets

### Important

The bucket is a **candidate retrieval optimization**, not itself the matchmaking rule.

A party being in the same bucket does not mean it is automatically eligible for a match.

### Acceptance Criteria

- ELO bucket lookup works correctly.
- Boundary cases are tested.
- Index insertion/removal is tested.
- V1 remains unchanged.
- V2 can retrieve candidate parties from buckets.

---

# Phase V2.4 — Dynamic Search Windows

### Objective

Allow the matcher to expand the acceptable candidate ELO range as queue time increases.

Conceptually:

```text
Short wait
    ↓
Narrow ELO window

Longer wait
    ↓
Wider ELO window
```

The exact expansion strategy must be configurable.

For example:

```text
0–30s      → ±100
30–60s     → ±150
60–120s    → ±250
120s+      → ±400
```

These numbers are examples only.

Do not assume they are the final values.

### Requirements

The search-window calculation should be isolated from the matcher.

Something conceptually similar to:

```go
window := searchPolicy.WindowFor(queueAge)
```

This allows the simulation to later experiment with different policies.

### Acceptance Criteria

- Search window depends on queue age.
- Search window never exceeds configured maximums.
- Boundary times are tested.
- Policy can be changed without modifying matcher logic.
- Candidate retrieval respects the calculated window.

---

# Phase V2.5 — Candidate Generation

### Objective

Use the matchmaking pool and dynamic search window to produce a small candidate set instead of scanning the entire queue.

Candidate generation should apply cheap filters first.

Potential filters:

```text
Game mode
Region
ELO window
Party size
Presence / eligibility
Queue state
```

Only after candidate narrowing should the expensive combinatorial search happen.

Conceptually:

```text
Queue
 ↓
Cheap indexed filtering
 ↓
Candidate parties
 ↓
Combination generation
 ↓
Constraint validation
```

### Important

Do not move expensive social-graph or full team evaluation into the index layer unless profiling demonstrates that it is necessary.

The index exists primarily to reduce the search space.

### Acceptance Criteria

- V2 no longer scans the entire queue for normal candidate generation.
- Candidate retrieval uses indexes.
- Candidate generation remains deterministic.
- Candidate generation has dedicated tests.
- Instrumentation can count candidate evaluations.

---

# Phase V2.6 — Preserve V1 Correctness

Before adding match scoring, prove that V2 does not incorrectly produce matches that violate the existing hard constraints.

Create equivalent test scenarios for V1 and V2.

Test:

- 10-player formation
- 5v5 partitioning
- Party integrity
- Duplicate players
- ELO constraints
- Team constraints
- Social conflicts
- Game-mode restrictions
- Region restrictions

Where practical, use V1 as the correctness oracle.

For the same controlled input:

```text
V1 → valid/invalid
V2 → valid/invalid
```

The exact match selected does not need to be identical because V2 will eventually score candidates differently.

The hard constraints must remain equivalent.

---

# Phase V2.7 — Match-Quality Scoring

### Objective

Stop selecting the first valid candidate.

Instead:

```text
Generate candidates
    ↓
Validate hard constraints
    ↓
Score valid candidates
    ↓
Select lowest score
```

Implement scoring as a separate component.

Conceptually:

```go
type MatchScorer interface {
    Score(candidate Candidate) Score
}
```

Do not tightly couple scoring calculations to candidate generation.

### Score Components

The scoring system should consider:

```text
Skill balance
Queue fairness
Party composition
Social quality
```

Conceptually:

```text
score =
    skillWeight * skillPenalty +
    waitWeight * waitPenalty +
    partyWeight * partyPenalty +
    socialWeight * socialPenalty
```

Lower score is better.

### Important

Hard constraints and soft scoring must remain separate.

A hard constraint should reject a candidate.

A soft preference should make a valid candidate less desirable.

Do not turn every undesirable property into a hard rejection.

### Acceptance Criteria

- Valid candidates receive deterministic scores.
- Lowest-score valid candidate is selected.
- Scoring is independently unit tested.
- Different score components can be configured.
- First-valid behavior is removed from V2 only.
- V1 retains its original behavior.

---

# Phase V2.8 — Party Composition Scoring

### Objective

Account for the distribution of party sizes across both teams.

Examples:

```text
3 + 2 vs 3 + 2
```

should generally score better than:

```text
5 vs 1 + 1 + 1 + 1 + 1
```

when all other factors are comparable.

The scoring system should not automatically assume that asymmetric party structures are invalid.

This is a quality preference, not necessarily a hard constraint.

### Acceptance Criteria

- Party composition can be represented independently of player ELO.
- Team party distributions can be compared.
- Composition contributes to candidate score.
- Tests cover symmetric and asymmetric examples.

---

# Phase V2.9 — Social Graph Evaluation

### Objective

Make social constraints an explicit graph-based component.

Relationships may include:

```text
Friends
Avoids
Visibility / Invisible relationships
```

Candidate evaluation should be able to efficiently detect conflicts.

Conceptually:

```text
conflicts(player) ∩ candidatePlayers
```

### Requirements

- Keep relationship logic separate from matcher orchestration.
- Avoid unnecessary O(N²) relationship checks where possible.
- Preserve the existing V1 social rules.
- Add explicit tests for friend and avoid conflicts.

### Acceptance Criteria

- Social conflicts are correctly detected.
- V2 does not create matches violating hard social constraints.
- Social checks are measurable in benchmarks.
- Relationship data remains independent from matchmaking implementation.

---

# Phase V2.10 — Reservation System

### Objective

Prevent multiple matcher workers from consuming the same queue entries.

Problem:

```text
Worker A → sees Party 17
Worker B → sees Party 17

Both create valid matches
```

Solution:

```text
TryReserve()
    ↓
Generate / validate match
    ↓
Commit
```

### Required operations

Conceptually:

```go
TryReserve(entryID)
ReleaseReservation(entryID)
CommitMatch(entryIDs, matchID)
```

Reservations should have explicit state.

```text
QUEUED
   ↓
RESERVED
   ↓
MATCHED
```

Failed reservation:

```text
RESERVED → QUEUED
```

Expired reservation:

```text
RESERVED → EXPIRED → QUEUED
```

### Initial Implementation

Use Go synchronization primitives.

Do not introduce Redis yet.

### Acceptance Criteria

- Two workers cannot reserve the same entry simultaneously.
- Failed candidates release reservations.
- Successful matches commit reservations atomically.
- Reservation expiry is handled.
- Tests reproduce concurrent contention.

---

# Phase V2.11 — Multiple Matcher Workers

### Objective

Run multiple matcher workers against the same matchmaking pool.

Conceptually:

```text
                    Matchmaking Pool
                    /       |       \
                   /        |        \
             Worker 1   Worker 2   Worker N
                   \        |        /
                    \       |       /
                     Reservation
```

### Test scenarios

- Two workers targeting the same oldest party.
- Concurrent queue insertion.
- Concurrent queue removal.
- Concurrent reservation.
- Reservation failure.
- Match creation race.
- Duplicate match prevention.

Run:

```bash
go test -race ./...
```

### Acceptance Criteria

- No duplicate party assignment.
- No data races.
- No corrupted queue indexes.
- Concurrent workers remain correct.

---

# Phase V2.12 — Event Bus

### Objective

Decouple successful match creation from downstream processing.

After a match is committed:

```text
MatchCreated
    ↓
Event Bus
    ├── Notification
    ├── Game Session
    ├── Analytics
    └── Match History
```

The initial implementation should use in-process Go channels.

### Event

`MatchCreated` should contain immutable data such as:

```text
matchID
gameMode
teamA
teamB
createdAt
matchmakingMetadata
```

### Important

Do not publish `MatchCreated` before the match has been successfully committed.

A failed match creation must not produce a successful match event.

### Acceptance Criteria

- Match creation produces exactly one event.
- Consumers can subscribe independently.
- Event payload is immutable.
- Event processing is testable.
- Matchmaking does not directly depend on downstream consumers.

---

# Phase V2.13 — Simulation Engine

Only after the core V2 matcher works should large-scale simulation be introduced.

Create:

```text
internal/simulation/
├── simulation.go
├── generator.go
├── scenarios.go
├── metrics.go
└── reporter.go
```

Support:

```text
1,000 players
5,000 players
10,000 players
```

Generate variation in:

- ELO
- Party size
- Arrival time
- Region
- Presence
- Friends
- Avoids

---

# Phase V2.14 — Benchmark V1 vs V2

This phase is extremely important.

Do not claim V2 is better merely because it uses indexes.

Run equivalent workloads through both matchers.

Measure:

```text
Candidate evaluations
Matcher CPU time
Memory usage
Matches/minute
P50 queue time
P95 queue time
P99 queue time
Average ELO gap
P95 ELO gap
Rejection rate
```

The primary optimization target is:

```text
Candidate evaluations
```

The primary quality targets are:

```text
Queue time
Match quality
```

V2 should demonstrate a meaningful reduction in search cost without unacceptable degradation in matchmaking quality.

---

# Phase V2.15 — Persistence

Only after the in-memory V2 behavior is stable should durable persistence be introduced.

PostgreSQL owns:

```text
Players
Parties
Relationships
Matches
Match history
```

The active matchmaking pool remains optimized for runtime access.

Use repository interfaces to separate persistence from domain logic.

Do not make PostgreSQL the primary candidate-search engine.

---

# Phase V2.16 — Distributed State

Introduce Redis only if extending the system to multiple matcher instances.

Redis may provide:

- Shared ephemeral matchmaking state
- Presence
- TTLs
- Distributed reservations
- Coordination
- Distributed locks where genuinely necessary

Do not blindly move all in-memory structures into Redis.

Preserve the distinction between:

```text
Durable state → PostgreSQL
Ephemeral distributed state → Redis
Active local hot path → memory
```

---

# Phase V2.17 — Durable Event Transport

If the project is extended into a distributed event-driven architecture, replace the in-process event transport with Kafka.

Kafka should be responsible for durable asynchronous event delivery.

Do not introduce Kafka merely to make the project appear more distributed.

The event abstraction should make this transport replacement possible without rewriting matchmaking logic.

---

# Phase V2.18 — API

Expose the system through HTTP only after the core engine is stable.

Potential endpoints:

```text
POST /parties
POST /queue/join
POST /queue/leave

GET /queue
GET /queue/{partyID}

POST /matchmaking/find

GET /matches
GET /matches/{matchID}

POST /simulation
GET /simulation/{simulationID}

GET /stats
```

HTTP handlers must remain thin.

Do not put matchmaking algorithms inside HTTP handlers.

---

# Phase V2.19 — UI

UI is optional and comes after the backend architecture is stable.

The UI should primarily visualize:

- Queue state
- Match creation
- Team composition
- Match quality
- Matcher metrics
- Simulation results
- V1 vs V2 comparisons

Do not let frontend requirements drive the backend architecture.

---

# 6. Engineering Guidelines

## Preserve Existing Behavior

Before modifying a component:

1. Read the existing implementation.
2. Read its tests.
3. Understand current invariants.
4. Identify what V1 behavior must remain unchanged.
5. Make the smallest change required for the current phase.

Never assume the README accurately describes every implementation detail.

The repository itself is the source of truth.

---

## Do Not Over-Engineer

Avoid introducing:

- Kafka
- Redis
- Kubernetes
- Microservices
- Complex distributed locks
- External matchmaking libraries

unless the current phase requires them.

The project should earn its complexity.

---

## Keep Domain Logic Independent

Matchmaking rules should not depend on:

- HTTP
- PostgreSQL
- Redis
- Kafka
- UI

The desired dependency direction is:

```text
Transport
    ↓
Application
    ↓
Domain / Matchmaking
    ↓
Infrastructure
```

The core matchmaking algorithm should remain testable without running a server or database.

---

# 7. Testing Requirements

Every phase must add tests.

At minimum:

```bash
go test ./...
```

must pass after every phase.

For concurrency-related work:

```bash
go test -race ./...
```

must pass.

Do not remove existing tests to make a new implementation pass.

When behavior intentionally changes in V2:

- Keep V1 tests.
- Add V2 tests.
- Clearly distinguish changed selection behavior from changed correctness behavior.

---

# 8. Benchmarking Requirements

Whenever an optimization is introduced, benchmark before and after.

Do not rely on intuition.

For example:

```text
Before:
Matcher V1
5,000 players
X candidate evaluations
Y ms CPU time

After:
Matcher V2
5,000 players
A candidate evaluations
B ms CPU time
```

Also verify matchmaking quality.

An optimization that is faster but produces substantially worse matches is not automatically an improvement.

---

# 9. Git / Change Discipline

Each phase should ideally produce a logically isolated commit.

Example:

```text
feat: introduce matchmaking pool

feat: add elo candidate buckets

feat: add dynamic search windows

feat: add v2 candidate generation

feat: add match quality scoring

feat: add atomic queue reservations

feat: add concurrent matcher workers
```

Avoid massive commits containing multiple unrelated architectural changes.

---

# 10. Definition of Done for V2

Matcher V2 is considered complete when it can:

- Retrieve candidates through matchmaking indexes.
- Use dynamic ELO search windows.
- Avoid scanning the entire queue unnecessarily.
- Respect existing hard matchmaking constraints.
- Evaluate social-graph constraints.
- Evaluate party composition.
- Score valid candidates.
- Select the best valid candidate rather than the first valid candidate.
- Safely operate with multiple matcher workers.
- Atomically reserve queue entries.
- Prevent duplicate matches.
- Publish `MatchCreated` events.
- Be benchmarked against V1.
- Demonstrate measurable candidate-search reduction.
- Preserve acceptable matchmaking quality.
- Pass unit, integration, and race tests.

---

# 11. Agent Workflow

For every task:

### Step 1 — Inspect

Read the relevant implementation and tests before changing anything.

### Step 2 — Identify the boundary

Determine whether the change belongs to:

```text
Domain
Queue
Index
Candidate generation
Constraint validation
Scoring
Reservation
Concurrency
Events
Simulation
Persistence
API
```

### Step 3 — Implement minimally

Implement only the current phase.

Do not prematurely implement future phases.

### Step 4 — Test

Run relevant tests first, then:

```bash
go test ./...
```

For concurrency work:

```bash
go test -race ./...
```

### Step 5 — Review

Check:

- Existing behavior preserved
- No unnecessary abstractions
- No duplicated logic
- No race conditions
- No hidden O(N²) behavior introduced accidentally
- Index consistency
- Error handling
- Determinism where expected

### Step 6 — Report

When finishing a task, report:

```text
Implemented:
- ...

Changed:
- ...

Tests:
- ...

Benchmarks:
- ...

Known limitations:
- ...

Next phase:
- ...
```

Do not claim a performance improvement without benchmark evidence.

---

# 12. Current Starting Point

The project is currently at:

```text
V1 — Exhaustive Matchmaker
        ↓
NEXT
        ↓
V2.1 — Establish V1/V2 boundary
```

Do not start with Redis, Kafka, PostgreSQL, UI, or simulation.

Start by inspecting the existing V1 implementation and tests, then implement **Phase V2.1 only**.

The immediate objective is to create a clean foundation on which V2 can be built without destabilizing the existing matcher.