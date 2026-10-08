# Player Matchmaker — Project Plan

## 1. Project Vision

Build a production-oriented matchmaking engine in Go capable of efficiently producing fair 5v5 matches from a large, dynamic pool of parties while respecting:

- Player skill / ELO
- Party composition
- Queue wait time
- Game mode
- Region and latency
- Social-graph constraints
- Concurrent matchmaking workers

The project will begin with a correctness-first in-memory implementation, progressively introduce candidate indexing and match-quality scoring, and finally demonstrate the system's behavior through large-scale simulation, benchmarking, persistence, APIs, and an optional UI.

The central engineering question is:

> **How can we efficiently produce fair 5v5 matches from a large, dynamic pool of parties while simultaneously respecting skill, party composition, wait time, and social-graph constraints?**

---

# 2. Core Architecture

The system is divided into a hot matchmaking path and a durable persistence path.

```text
                         ┌──────────────────┐
                         │    HTTP API      │
                         └────────┬─────────┘
                                  │
                                  ▼
                         ┌──────────────────┐
                         │ Queue Manager    │
                         └────────┬─────────┘
                                  │
                                  ▼
                    ┌───────────────────────────┐
                    │   Matchmaking Pool        │
                    │                           │
                    │  ELO buckets              │
                    │  Game mode indexes        │
                    │  Party-size indexes       │
                    │  Region indexes           │
                    │  Queue timestamps         │
                    └─────────────┬─────────────┘
                                  │
                         ┌────────▼────────┐
                         │ Matcher Workers │
                         └────────┬────────┘
                                  │
                    ┌─────────────▼─────────────┐
                    │ Candidate Generation      │
                    │ → Constraint Validation   │
                    │ → Match Scoring           │
                    │ → Best Candidate Selection│
                    └─────────────┬─────────────┘
                                  │
                           TryReserve()
                                  │
                    ┌─────────────▼─────────────┐
                    │ Match Creation / Commit   │
                    └─────────────┬─────────────┘
                                  │
                           MatchCreated
                                  │
                    ┌─────────────▼─────────────┐
                    │       Event Bus           │
                    └───────┬───────┬───────────┘
                            │       │
                ┌───────────┘       └────────────┐
                ▼                                ▼
        Notification                       Game/Session
          Consumer                           Consumer
                │                                │
                └────────────┬───────────────────┘
                             ▼
                         Analytics

       ┌─────────────────────────────────────────────┐
       │              Durable State                  │
       │                  PostgreSQL                 │
       │                                             │
       │ players / parties / matches / relationships│
       │ match history                              │
       └─────────────────────────────────────────────┘

       Redis:
       Distributed matchmaking state / coordination /
       presence / TTLs / reservations when distributed

       Kafka:
       Durable asynchronous event transport
```

The system intentionally separates:

**Hot / ephemeral state**

- Active matchmaking pool
- Queue timestamps
- Candidate indexes
- Reservation state
- Presence
- Temporary matchmaking metadata

**Durable state**

- Players
- Parties
- Relationships
- Matches
- Match history

PostgreSQL owns durable state. The matchmaking pool is optimized for in-memory access. Redis can later provide distributed coordination and ephemeral shared state when multiple matcher instances are introduced. Kafka can provide durable asynchronous event delivery between services.

---

# 3. Architectural Principles

## 3.1 Correctness Before Optimization

The first matcher will use exhaustive candidate search.

This establishes a correctness baseline against which optimized matchmaking can be compared.

```text
Matcher V1
    ↓
Exhaustive candidate search
    ↓
Correctness baseline
```

The second matcher will introduce indexed candidate selection.

```text
Matcher V2
    ↓
Candidate indexes / ELO buckets
    ↓
Smaller search space
    ↓
Lower candidate evaluation cost
```

The simulation and benchmark suite will compare V1 and V2.

---

## 3.2 Hot Path vs Cold Path

The matcher should not repeatedly query PostgreSQL for active queue candidates.

The hot path operates primarily against memory:

```text
Queue Entry
    ↓
Matchmaking Pool
    ↓
Indexes / Buckets
    ↓
Candidate Generation
    ↓
Constraint Validation
    ↓
Scoring
```

PostgreSQL is responsible for persistence and historical data rather than being the primary matchmaking search structure.

---

## 3.3 Match Quality Over First Valid Match

The matcher must not simply return the first valid combination.

Instead:

```text
Generate candidates
        ↓
Validate constraints
        ↓
Score valid candidates
        ↓
Select lowest-score candidate
```

This allows the system to explicitly balance:

- Skill quality
- Queue fairness
- Party composition
- Social constraints

---

# 4. Domain Model

The core domain consists of:

```text
Player
Party
QueueEntry
Match
Team
Relationship
MatchmakingMetadata
```

## Player

A player contains:

- ID
- ELO/MMR
- Rank
- Presence
- Region
- Latency information
- Relationship information

Presence may include:

```text
Online
Away
Offline
```

The matchmaking rules determine whether each state is eligible for a particular queue/match.

---

## Party

A party contains:

- Party ID
- Player members
- Party size
- Game mode
- Average ELO
- ELO variance
- Minimum ELO
- Maximum ELO
- Queue timestamp

Parties can contain 1–5 players.

Competitive matchmaking allows parties of up to 3 players.

Casual matchmaking allows parties of up to 5 players.

---

# 5. ELO / Skill Model

Average ELO alone is insufficient for evaluating party and team balance.

For each party and team calculate:

```text
mean ELO
ELO variance
minimum ELO
maximum ELO
```

For teams additionally calculate:

```text
average ELO
ELO variance
highest-player difference
lowest-player difference
```

This allows the matcher to distinguish between:

```text
1000 / 1000 / 2000
```

and:

```text
1333 / 1333 / 1333
```

even though both have approximately the same average ELO.

The matcher can therefore evaluate both aggregate skill and skill distribution.

---

# 6. Matchmaking Pool

The queue will not remain a simple FIFO queue.

Instead, the system will maintain a matchmaking pool containing waiting parties and indexes that allow efficient candidate retrieval.

The pool will index parties by:

- Game mode
- ELO range
- Party size
- Region
- Queue timestamp

Additional dimensions such as latency can be incorporated into candidate filtering.

Conceptually:

```text
MatchmakingPool
├── Competitive
│   ├── ELO 1200–1300
│   ├── ELO 1300–1400
│   ├── ELO 1400–1500
│   └── ...
│
└── Casual
    ├── ELO 1200–1300
    ├── ELO 1300–1400
    └── ...
```

The implementation should avoid assuming that these indexes are independent queues. A party may belong to multiple indexes simultaneously.

---

# 7. Dynamic ELO Search Window

A party initially searches within a narrow ELO range.

As queue time increases, the search window expands.

Conceptually:

```text
Queue time increases
        ↓
Search window expands
        ↓
More candidates become eligible
        ↓
Queue time decreases at the cost of potentially weaker matches
```

The exact values will be configurable rather than hard-coded.

For example:

```text
0–30s      → narrow search
30–60s     → wider search
60–120s    → wider search
120s+      → maximum configured search range
```

This creates an explicit tradeoff between:

```text
Match quality
        vs
Waiting time
```

The simulation will be used to evaluate the effect of different expansion rates.

---

# 8. Candidate Generation

Candidate generation is responsible for finding a limited set of potentially valid parties.

The matcher should not immediately perform expensive constraint validation against the entire queue.

Candidate generation should use:

- Game mode
- ELO window
- Region
- Party size
- Queue age
- Other cheap eligibility filters

The result is a smaller candidate set that proceeds to expensive validation.

---

# 9. Matcher V1 — Exhaustive Search

The first matcher implementation will retain the existing combinatorial approach.

```text
Queue
 ↓
Eligible parties
 ↓
Generate combinations
 ↓
Find combinations totaling 10 players
 ↓
Generate 5v5 partitions
 ↓
Validate constraints
 ↓
Return valid candidates
```

V1 is intentionally correctness-first.

Its purpose is to:

- Establish baseline behavior
- Validate domain rules
- Provide a reference implementation
- Generate benchmark data
- Provide a correctness oracle for Matcher V2

V1 may be computationally expensive at large queue sizes. This is expected.

---

# 10. Matcher V2 — Candidate-Indexed Search

Matcher V2 will replace unrestricted queue scanning with indexed candidate selection.

The matcher will:

```text
Select oldest eligible party
        ↓
Determine dynamic ELO window
        ↓
Retrieve candidates from indexes
        ↓
Generate viable party combinations
        ↓
Validate constraints
        ↓
Score valid matches
        ↓
Select best candidate
```

The primary objective is reducing the number of candidate combinations evaluated.

The simulation will compare V1 and V2 under equivalent workloads.

---

# 11. Constraint Validation

Every candidate match must satisfy all mandatory constraints.

## Skill Constraints

Evaluate:

- Team average ELO difference
- Within-team ELO spread
- ELO variance
- Highest-player difference
- Lowest-player difference

## Party Constraints

Validate:

- Maximum party size
- Game-mode restrictions
- Valid 5v5 composition
- Party members remain together
- No duplicate players

## Region / Latency Constraints

Candidates must satisfy configured region and latency requirements.

## Social Constraints

Candidates must not violate friend/avoid rules.

Constraints are divided into:

```text
Hard constraints
    ↓
Candidate is invalid if violated

Soft constraints
    ↓
Candidate remains valid but receives a worse score
```

This distinction is important because not every undesirable property should necessarily make a candidate impossible.

---

# 12. Social Graph

Friends and avoids will be represented as graph relationships.

Example:

```text
Player A
 ├── friends → B, C, D
 └── avoids  → X, Y
```

During candidate evaluation, the matcher can construct a conflict set and test:

```text
conflicts(player) ∩ candidate_players
```

Social relationships should be represented independently from matchmaking logic.

The matcher consumes relationship information but does not own the social graph.

This keeps the social model replaceable and testable.

---

# 13. Party Composition Scoring

Average ELO alone does not guarantee a fair or desirable match.

Party composition will therefore contribute to match quality.

Examples:

```text
3 + 2  vs  3 + 2
```

is highly cohesive and symmetrical.

```text
3 + 2  vs  2 + 2 + 1
```

is acceptable.

```text
5  vs  1 + 1 + 1 + 1 + 1
```

may be undesirable despite identical average ELO.

The system will therefore calculate a party-composition metric that evaluates how evenly party structures are distributed between teams.

This metric is a soft constraint unless a specific game mode requires otherwise.

---

# 14. Match Scoring

Instead of selecting the first valid match, the matcher ranks valid candidates.

The conceptual scoring model is:

```text
Score =
    skill_difference_weight × skill_difference
  + wait_time_weight × queue_fairness
  + party_composition_weight × party_split_penalty
  + social_weight × social_penalty
```

Lower scores are preferred.

The exact scoring function will be configurable and tuned using simulation.

The scoring system must balance:

```text
Skill balance
      +
Queue fairness
      +
Party composition
      +
Social quality
```

The scoring model should remain deterministic for identical inputs.

---

# 15. Match Selection

The matcher will:

1. Generate a candidate set.
2. Remove candidates violating hard constraints.
3. Calculate matchmaking metadata.
4. Score remaining candidates.
5. Select the lowest-scoring candidate.
6. Reserve all participating queue entries.
7. Commit the match.
8. Publish a `MatchCreated` event.

No match should be committed merely because it is the first valid candidate encountered.

---

# 16. Queue Entry Lifecycle

Every queue entry has an explicit lifecycle.

```text
QUEUED
   │
   ▼
RESERVED
   │
   ▼
MATCHED
   │
   ▼
ACCEPTED
   │
   ▼
STARTED
```

Failure paths:

```text
QUEUED ───────────────→ CANCELLED

RESERVED ─────────────→ EXPIRED

MATCHED ──────────────→ REJECTED
```

The matcher owns the transition through queueing and matching states.

The downstream game/session system owns the later lifecycle where appropriate.

---

# 17. Reservation and Atomic Claiming

Concurrent matcher workers create a race condition.

Example:

```text
Worker A → sees Party 17
Worker B → sees Party 17

Worker A → generates Match X
Worker B → generates Match Y
```

The same party must never be committed into two matches.

Before a candidate becomes committed, all participating queue entries must therefore be atomically reserved.

Core operations:

```text
TryReserve(entryID)
ReleaseReservation(entryID)
CommitMatch(entryIDs, matchID)
```

A candidate is eligible for final match creation only if all required reservations succeed.

If any reservation fails:

```text
Release successful reservations
Discard candidate
Continue searching
```

The initial implementation can use Go synchronization primitives.

When the system becomes distributed, reservation state can move to Redis or another distributed coordination mechanism.

---

# 18. Concurrency Model

The system should support multiple matcher workers.

Conceptually:

```text
HTTP API
    ↓
Queue
    ↓
Matchmaking Pool
    ↓
┌──────────┬──────────┬──────────┐
│ Worker 1 │ Worker 2 │ Worker N │
└──────────┴──────────┴──────────┘
    ↓
Reservation
    ↓
Match creation
```

Concurrency concerns to explicitly test:

- Mutex correctness
- Atomic reservation
- Race conditions
- Duplicate match prevention
- Concurrent queue insertion/removal
- Concurrent cancellation
- Worker contention
- Idempotent match creation

The implementation must be race-tested using Go's race detector.

---

# 19. Event Architecture

After a match is successfully committed, the matcher publishes an immutable `MatchCreated` event.

The event contains:

```text
matchID
gameMode
teamA
teamB
createdAt
matchmakingMetadata
```

Consumers independently react to this event.

Potential consumers:

```text
Notification Consumer
Game / Session Consumer
Analytics Consumer
Match History Consumer
```

The matchmaking engine should publish the fact that a match was created rather than directly calling downstream services.

---

# 20. Event Transport

The initial implementation should use an in-process Go event bus / channels.

This provides:

- Low latency
- Simple development
- Easy testing
- No unnecessary infrastructure

The event abstraction should allow the transport to later be replaced.

For distributed deployments:

```text
Go channels
     ↓
Redis / Kafka
```

Kafka should be used as durable asynchronous event transport if the project is extended into a distributed architecture.

Redis should handle ephemeral distributed coordination/state rather than replacing Kafka's event-stream role.

The project does not need Kafka, RabbitMQ, and Redis simultaneously.

---

# 21. Persistence Architecture

PostgreSQL will own durable state.

Persist:

```text
players
parties
relationships
matches
match history
```

The active matchmaking pool owns:

```text
queued parties
reservation state
queue timestamps
candidate indexes
temporary matchmaking metadata
```

This creates a clear hot/cold architecture:

```text
PostgreSQL
    ↓
Durable source of truth

Memory / Redis
    ↓
Fast active matchmaking state
```

Repository interfaces should separate domain logic from persistence.

Initial repositories:

```text
PlayerRepository
PartyRepository
RelationshipRepository
MatchRepository
```

A dedicated `QueueRepository` is only necessary if queue state itself needs durable persistence. The active matchmaking queue should primarily remain an in-memory/distributed runtime structure.

---

# 22. Simulation Engine

Create:

```text
internal/simulation/
├── simulation.go
├── generator.go
├── scenarios.go
├── metrics.go
└── reporter.go
```

The simulation engine should model realistic player arrival and matchmaking behavior rather than simply generating 5,000 players and invoking the matcher once.

---

# 23. Simulation Population

Generate approximately:

```text
1,000 players
5,000 players
10,000 players
```

Player attributes should vary across:

- ELO
- Rank
- Presence
- Region
- Party size
- Arrival time
- Friend relationships
- Avoid relationships

The baseline ELO distribution can use:

```text
Normal distribution
Mean ≈ 1500
Standard deviation ≈ 300
```

Parameters should remain configurable.

---

# 24. Simulation Scenarios

The simulator should support multiple workload scenarios.

## Scenario A — Normal Load

Approximately 1,000 players with ordinary ELO, party-size, and arrival distributions.

## Scenario B — Large Queue

Approximately 5,000 players.

Primary benchmark scenario.

## Scenario C — Stress Test

Approximately 10,000 players.

Used to expose scalability problems.

## Scenario D — High Party Load

A large percentage of players arrive in parties of 3–5.

## Scenario E — Skill Polarization

Large populations exist at extreme ELO ranges with fewer players in the middle.

## Scenario F — Socially Dense Queue

Higher friend and avoid graph density.

## Scenario G — Burst Arrival

Large numbers of players enter the queue within a short period.

## Scenario H — Slow Arrival

Players enter gradually over time.

---

# 25. Simulation Variables

The simulation should allow controlled variation of:

- ELO distribution
- Party-size distribution
- Friend graph density
- Avoid graph density
- Arrival rate
- Online percentage
- Region distribution
- Latency distribution
- Skill-gap expansion rate
- Match scoring weights
- Queue size

This allows the matcher to be evaluated experimentally rather than through a single fixed benchmark.

---

# 26. Simulation Metrics

Collect at minimum:

### Queue Metrics

- P50 queue time
- P95 queue time
- P99 queue time
- Average queue time
- Queue depth over time

### Match Quality

- Average ELO gap
- P95 ELO gap
- Within-team ELO spread
- Team ELO variance
- Party composition score
- Social constraint violations

### Matcher Performance

- Matches per minute
- Candidate combinations examined
- Candidate combinations rejected
- Rejection reason distribution
- Matcher CPU time
- Matchmaking latency
- Successful matches per worker

### Concurrency

- Reservation failures
- Duplicate match attempts
- Worker contention
- Race detector failures

---

# 27. Matcher Benchmark

The most important benchmark will compare Matcher V1 and Matcher V2 under equivalent workloads.

Example result:

> At 5,000 queued players, the candidate-indexed matcher reduced candidate evaluations by X% compared with exhaustive search while maintaining a P95 queue time of Y seconds.

The benchmark should measure both:

```text
Performance
```

and:

```text
Match quality
```

An optimization is only considered successful if it reduces computational cost without producing unacceptable matchmaking quality degradation.

---

# 28. Metrics and Observability

The matcher should expose internal metrics suitable for both simulation and the future UI.

Useful runtime metrics include:

```text
queue_depth
matches_created
matchmaking_latency
candidate_evaluations
candidate_rejections
reservation_failures
active_workers
average_elo_gap
p95_queue_time
```

Metrics should be collected independently from the matchmaking algorithm where possible.

---

# 29. HTTP API

The HTTP layer exposes the matchmaking engine without embedding matchmaking logic inside handlers.

Core endpoints:

```text
POST /parties
POST /queue/join
POST /queue/leave
GET  /queue
GET  /queue/{partyID}

POST /matchmaking/find
GET  /matches
GET  /matches/{matchID}

POST /simulation
GET  /simulation/{simulationID}

GET  /stats
```

Handlers should translate HTTP requests into domain/application operations.

They should not contain matchmaking algorithms.

---

# 30. API Responsibilities

The API should support:

- Party creation
- Queue entry
- Queue removal
- Queue inspection
- Match inspection
- Manual matchmaking trigger
- Simulation configuration
- Simulation execution
- Metrics retrieval

The matchmaking engine should remain usable independently of HTTP.

---

# 31. UI

The UI is an observability and control layer rather than the primary engineering focus.

Core screens:

### Dashboard

Display:

- Current queue depth
- Active matcher workers
- Recent matches
- Queue-time metrics
- ELO balance metrics

### Party Creation

Allow users to:

- Create parties
- Select players
- Select game mode
- Configure party members

### Queue Management

Display:

- Queued parties
- Queue time
- ELO
- Region
- Matchmaking state

### Match Viewer

Visualize:

```text
Team A
5 players

vs

Team B
5 players
```

with:

- ELO
- party composition
- team ELO
- match score
- matchmaking metadata

### Simulation Control

Allow configuration of:

- Player count
- ELO distribution
- Party distribution
- Social graph density
- Arrival rate
- Matcher version

### Simulation Results

Display:

- Queue-time distributions
- Match quality
- Candidate evaluations
- Rejection reasons
- Matcher performance
- V1 vs V2 comparison

---

# 32. Testing Strategy

Testing should exist at multiple levels.

## Unit Tests

Test:

- Player validation
- Party validation
- ELO calculations
- Team calculations
- Relationship constraints
- Candidate validation
- Match scoring
- Queue lifecycle transitions
- Reservation logic

## Matcher Tests

Test:

- Valid 5v5 matches
- Invalid combinations
- Party-size combinations
- ELO constraints
- Dynamic ELO windows
- Social conflicts
- Party composition scoring
- Best-candidate selection

## Concurrency Tests

Test:

- Multiple workers processing the same queue
- Concurrent reservations
- Concurrent queue joins/leaves
- Duplicate match prevention
- Reservation expiry
- Idempotency

Run:

```bash
go test -race ./...
```

## Integration Tests

Test:

```text
Create Party
    ↓
Join Queue
    ↓
Matcher
    ↓
Reservation
    ↓
Match Creation
    ↓
MatchCreated Event
    ↓
Consumer
```

## Simulation Tests

Verify:

- deterministic scenarios
- reproducible seeds
- edge-case workloads
- performance thresholds
- V1/V2 correctness equivalence

---

# 33. Performance Testing

Performance testing should focus on the matchmaking algorithm rather than only total application throughput.

Benchmark:

```text
1k players
5k players
10k players
```

Compare:

```text
Matcher V1
vs
Matcher V2
```

Measure:

```text
candidate evaluations
CPU time
memory usage
matches/minute
P50 queue time
P95 queue time
P99 queue time
```

The optimized matcher must preserve correctness while reducing the search space.

---

# 34. Implementation Phases

## Phase 1 — Domain Model

Implement and stabilize:

- Player
- Party
- Team
- Match
- Relationship
- QueueEntry
- Matchmaking metadata
- ELO calculations

Goal:

> A correct, thoroughly tested domain model.

---

## Phase 2 — Matchmaking Pool

Replace the conceptual FIFO queue with an indexed matchmaking pool.

Implement:

- Game-mode indexes
- ELO buckets
- Party-size indexes
- Region indexes
- Queue timestamps
- Entry lifecycle

Goal:

> Efficiently retrieve candidate parties from a dynamic queue.

---

## Phase 3 — Matcher V1

Implement exhaustive matchmaking.

Flow:

```text
Eligible parties
→ combinations
→ 10-player candidates
→ 5v5 partitions
→ hard constraints
→ valid matches
```

Goal:

> Establish a correctness baseline.

---

## Phase 4 — Matcher V2

Implement:

- Candidate indexing
- Dynamic ELO search windows
- Candidate narrowing
- Efficient social conflict checks

Goal:

> Reduce the number of candidate evaluations without compromising correctness.

---

## Phase 5 — Match Scoring

Implement:

- Skill score
- Queue fairness score
- Party composition score
- Social score
- Combined match score

Replace first-valid selection with best-valid selection.

Goal:

> Produce the highest-quality valid match rather than merely any valid match.

---

## Phase 6 — Reservation & Concurrency

Implement:

- Queue state transitions
- Atomic reservations
- Reservation release
- Reservation expiry
- Multiple matcher workers
- Idempotent match creation

Goal:

> Prevent duplicate matches and make concurrent matchmaking safe.

---

## Phase 7 — Event System

Implement an in-process event bus using Go channels.

Create:

```text
MatchCreated
```

and consumers for:

- Notifications
- Game/session handling
- Analytics
- Match history

Goal:

> Decouple matchmaking from downstream systems.

---

## Phase 8 — Simulation Engine

Implement:

- Player generator
- Party generator
- Relationship graph generator
- Arrival simulation
- Multiple workload scenarios
- Configurable parameters

Goal:

> Create realistic matchmaking workloads.

---

## Phase 9 — Metrics & Benchmarking

Implement:

- Queue-time metrics
- Match-quality metrics
- Candidate evaluation metrics
- Rejection statistics
- CPU measurements
- V1/V2 comparison

Goal:

> Demonstrate quantitatively that the optimized matcher improves scalability while preserving match quality.

---

## Phase 10 — PostgreSQL Persistence

Introduce repository interfaces and PostgreSQL implementations.

Persist:

- Players
- Parties
- Relationships
- Matches
- Match history

Keep the active matchmaking pool in memory.

Goal:

> Separate durable state from high-performance matchmaking state.

---

## Phase 11 — Distributed State

Introduce Redis only when the architecture requires multiple matcher instances.

Redis may provide:

- Shared ephemeral state
- Presence
- TTLs
- Distributed reservations/locks
- Coordination between matcher instances

Goal:

> Extend the matcher beyond a single process without changing core matchmaking semantics.

---

## Phase 12 — Durable Event Transport

If distributed event processing is required, replace the in-process event transport with Kafka.

Goal:

> Provide durable asynchronous event delivery for downstream consumers.

Kafka is not required for the initial implementation.

---

## Phase 13 — HTTP API

Expose the system through HTTP.

Implement:

- Party APIs
- Queue APIs
- Match APIs
- Simulation APIs
- Statistics APIs

Goal:

> Make the matchmaking engine externally controllable and observable.

---

## Phase 14 — UI

Build the visualization/control layer.

Goal:

> Allow users to observe queue behavior, inspect matches, run simulations, and compare matcher versions.

---

## Phase 15 — Final Testing & Documentation

Complete:

- Unit tests
- Integration tests
- Concurrency tests
- Race testing
- Load testing
- API documentation
- Architecture documentation
- Benchmark results
- Setup instructions

Goal:

> Produce a reproducible, measurable engineering project rather than a matchmaking prototype.

---

# 35. Final Architecture

The intended final architecture is:

```text
                         ┌───────────────────┐
                         │    Web / UI       │
                         └─────────┬─────────┘
                                   │
                                   ▼
                         ┌───────────────────┐
                         │     HTTP API      │
                         └─────────┬─────────┘
                                   │
                                   ▼
                         ┌───────────────────┐
                         │  Queue Manager    │
                         └─────────┬─────────┘
                                   │
                                   ▼
                    ┌────────────────────────────┐
                    │    Matchmaking Pool        │
                    │                            │
                    │ ELO buckets                │
                    │ Game-mode indexes          │
                    │ Party-size indexes         │
                    │ Region indexes              │
                    │ Queue timestamps            │
                    └─────────────┬──────────────┘
                                  │
                     ┌────────────▼────────────┐
                     │    Matcher Workers      │
                     └────────────┬────────────┘
                                  │
                    ┌─────────────▼─────────────┐
                    │ Candidate Generation      │
                    │ Constraint Validation     │
                    │ Match Scoring             │
                    │ Best Candidate Selection  │
                    └─────────────┬─────────────┘
                                  │
                           TryReserve()
                                  │
                    ┌─────────────▼─────────────┐
                    │     Match Commit          │
                    └─────────────┬─────────────┘
                                  │
                            MatchCreated
                                  │
                    ┌─────────────▼─────────────┐
                    │       Event Bus            │
                    └───────┬────────┬───────────┘
                            │        │
                  ┌─────────▼──┐  ┌─▼────────────┐
                  │ Notification│  │ Game/Session │
                  │  Consumer   │  │   Consumer   │
                  └─────────────┘  └──────────────┘
                            │
                            ▼
                       Analytics

             ┌──────────────────────────────┐
             │         PostgreSQL           │
             │                              │
             │ Players                      │
             │ Parties                      │
             │ Relationships               │
             │ Matches                      │
             │ Match History                │
             └──────────────────────────────┘

             ┌──────────────────────────────┐
             │            Redis             │
             │                              │
             │ Distributed ephemeral state  │
             │ Presence / TTLs              │
             │ Reservations / coordination  │
             └──────────────────────────────┘

             ┌──────────────────────────────┐
             │            Kafka             │
             │                              │
             │ Durable asynchronous events  │
             └──────────────────────────────┘
```

Redis and Kafka are **not required simultaneously in the initial version**. The first implementation should use in-memory matchmaking state and Go channels. PostgreSQL, Redis, and Kafka are introduced progressively when their respective architectural problems actually need them.

---

# 36. Success Criteria

The project is considered successful when:

- [ ] Parties can be represented and validated correctly.
- [ ] Matchmaking supports valid 5v5 party combinations.
- [ ] The matchmaking pool uses indexed candidate retrieval.
- [ ] ELO search windows expand with queue time.
- [ ] Matcher V1 provides an exhaustive correctness baseline.
- [ ] Matcher V2 reduces candidate evaluations relative to V1.
- [ ] Match selection uses quality scoring rather than first-valid selection.
- [ ] Party composition affects match quality.
- [ ] Social graph constraints are enforced.
- [ ] Queue entries have explicit lifecycle states.
- [ ] Concurrent workers cannot match the same party twice.
- [ ] Reservations are atomic and safely released.
- [ ] Match creation is idempotent.
- [ ] `MatchCreated` events are published after successful match creation.
- [ ] Downstream consumers are decoupled from the matcher.
- [ ] PostgreSQL stores durable domain state and match history.
- [ ] Active matchmaking state remains optimized for fast access.
- [ ] Simulation supports 1k, 5k, and 10k-player workloads.
- [ ] Simulation supports multiple workload scenarios.
- [ ] P50/P95/P99 queue times are measured.
- [ ] Match quality is measured quantitatively.
- [ ] Candidate evaluations and rejection reasons are measured.
- [ ] Matcher V1 and V2 can be benchmarked against equivalent workloads.
- [ ] Concurrent execution passes Go race testing.
- [ ] HTTP API exposes core functionality.
- [ ] UI can visualize matchmaking and simulation behavior.
- [ ] Architecture and benchmark results are documented.

# 37. End State

The final project should demonstrate more than the ability to construct a 5v5 match.

It should demonstrate the complete engineering progression:

```text
Correctness
    ↓
Exhaustive Matchmaking
    ↓
Candidate Indexing
    ↓
Dynamic Search Expansion
    ↓
Match Quality Scoring
    ↓
Reservation & Concurrency
    ↓
Simulation
    ↓
Benchmarking
    ↓
Persistence
    ↓
Event-Driven Architecture
    ↓
Distributed Coordination
```

The most important result is a measurable demonstration that the system can move from a naive exhaustive matchmaking strategy toward an indexed, concurrent, quality-aware matchmaking engine while preserving correctness and acceptable matchmaking quality.