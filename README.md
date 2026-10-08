# Player Matchmaker

A Go-based matchmaking engine for online games that produces fair 5v5 matches from a dynamic pool of parties while balancing skill, party composition, queue time, and social-graph constraints.

The project is built as a progressive engineering exercise: starting with a correctness-first exhaustive matcher and evolving into an indexed, concurrent, measurable matchmaking system capable of handling large simulated player populations.

## The Problem

The core question this project attempts to answer is:

> **How can we efficiently produce fair 5v5 matches from a large, dynamic pool of parties while simultaneously respecting skill, party composition, wait time, and social-graph constraints?**

A naive matchmaking system can search for the first combination of players that satisfies its constraints. That approach becomes increasingly expensive as the queue grows and does not necessarily produce the best possible match.

Player Matchmaker explores how to improve this process through:

- ELO-based candidate indexing
- Dynamic skill search windows
- Match-quality scoring
- Party composition analysis
- Social-graph constraints
- Concurrent matchmaking workers
- Atomic queue-entry reservation
- Simulation and benchmarking
- Durable persistence
- Event-driven match consumption

## Key Concepts

### Dynamic Matchmaking Pool

The active queue is not treated as a simple FIFO queue.

Instead, waiting parties are maintained in an indexed matchmaking pool using dimensions such as:

- Game mode
- ELO range
- Party size
- Region
- Queue timestamp

ELO buckets allow the matcher to narrow the candidate search space before performing expensive validation.

For example:

```text
Competitive
├── 1200–1300
├── 1300–1400
├── 1400–1500
├── 1500–1600
└── ...

Casual
├── 1200–1300
├── 1300–1400
├── 1400–1500
└── ...
```

A party initially searches within a relatively narrow skill window. As its queue time increases, the window expands, trading some match quality for reduced waiting time.

## Matchmaking Evolution

The project deliberately implements matchmaking in stages.

### Matcher V1 — Exhaustive Search

The initial matcher provides a correctness baseline:

```text
Eligible parties
      ↓
Generate party combinations
      ↓
Find combinations totaling 10 players
      ↓
Generate possible 5v5 partitions
      ↓
Validate constraints
      ↓
Return valid candidates
```

This approach is intentionally expensive. Its purpose is to establish a reference implementation that can later be compared against optimized matchmaking.

### Matcher V2 — Candidate-Indexed Search

The optimized matcher narrows the search space before generating expensive combinations:

```text
Oldest eligible party
        ↓
Dynamic ELO window
        ↓
Indexed candidate retrieval
        ↓
Candidate generation
        ↓
Constraint validation
        ↓
Match scoring
        ↓
Best valid match
```

The goal is to significantly reduce candidate evaluations while preserving matchmaking quality.

## Match Quality

The matcher does not simply return the first valid match.

Every valid candidate is scored and the lowest-scoring candidate is selected.

The conceptual scoring model is:

```text
Score =
    skill_difference_weight × skill_difference
  + wait_time_weight × queue_fairness
  + party_composition_weight × party_split_penalty
  + social_weight × social_penalty
```

This allows the system to balance:

```text
Skill balance
      +
Queue fairness
      +
Party cohesion
      +
Social constraints
```

The scoring function is configurable and can be evaluated experimentally through simulation.

## Skill Model

Average ELO alone is not sufficient to describe a party or team.

The system therefore tracks:

```text
Mean ELO
ELO variance
Minimum ELO
Maximum ELO
```

For teams, additional comparisons include:

```text
Average ELO difference
ELO variance
Highest-player difference
Lowest-player difference
```

For example:

```text
1000 / 1000 / 2000
```

and:

```text
1333 / 1333 / 1333
```

have approximately the same average ELO but very different skill distributions.

The matcher can therefore evaluate both aggregate skill and skill distribution.

## Party Composition

Party structure also affects match quality.

Examples:

```text
3 + 2  vs  3 + 2
```

is highly symmetrical.

```text
3 + 2  vs  2 + 2 + 1
```

is reasonably balanced.

```text
5  vs  1 + 1 + 1 + 1 + 1
```

may be undesirable even when both teams have identical average ELO.

Party composition is therefore incorporated into match scoring rather than relying exclusively on player skill.

## Social Graph

Friend and avoid relationships are represented as graph connections.

For example:

```text
Player A
├── friends → B, C, D
└── avoids  → X, Y
```

Candidate matches are checked against these relationships.

Conceptually:

```text
conflicts(player) ∩ candidate_players
```

is used to identify social conflicts.

The social graph remains separate from matchmaking logic so that relationship data can be replaced or extended independently.

## Queue Entry Lifecycle

Queue entries have explicit states:

```text
QUEUED
   ↓
RESERVED
   ↓
MATCHED
   ↓
ACCEPTED
   ↓
STARTED
```

Failure paths include:

```text
QUEUED  → CANCELLED
RESERVED → EXPIRED
MATCHED → REJECTED
```

The reservation stage is particularly important when multiple matcher workers operate concurrently.

Without reservation, two workers could observe the same party and attempt to place it into different matches simultaneously.

The matcher therefore uses operations conceptually equivalent to:

```text
TryReserve(entryID)
ReleaseReservation(entryID)
CommitMatch(entryIDs, matchID)
```

## Concurrency

The intended runtime architecture supports multiple matchmaking workers:

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
Match Creation
```

Concurrency concerns include:

- Mutex correctness
- Atomic reservation
- Race conditions
- Duplicate match prevention
- Concurrent queue operations
- Reservation expiry
- Worker contention
- Idempotent match creation

The project is tested with Go's race detector.

## Architecture

The system separates fast ephemeral matchmaking state from durable application state.

```text
                         ┌───────────────────┐
                         │      HTTP API     │
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
                    │ Region indexes             │
                    │ Queue timestamps           │
                    └─────────────┬──────────────┘
                                  │
                         ┌────────▼────────┐
                         │ Matcher Workers │
                         └────────┬─────────┘
                                  │
                    ┌─────────────▼─────────────┐
                    │ Candidate Generation      │
                    │ Constraint Validation     │
                    │ Match Scoring             │
                    │ Best Candidate Selection  │
                    └─────────────┬─────────────┘
                                  │
                           Reservation
                                  │
                    ┌─────────────▼─────────────┐
                    │     Match Creation        │
                    └─────────────┬─────────────┘
                                  │
                            MatchCreated
                                  │
                    ┌─────────────▼─────────────┐
                    │       Event Bus            │
                    └───────┬────────┬───────────┘
                            │        │
                   ┌────────▼──┐  ┌─▼────────────┐
                   │Notification│  │ Game/Session │
                   │  Consumer  │  │   Consumer   │
                   └────────────┘  └──────────────┘

             ┌──────────────────────────────┐
             │         PostgreSQL           │
             │                              │
             │ Players                      │
             │ Parties                      │
             │ Relationships               │
             │ Matches                      │
             │ Match History                │
             └──────────────────────────────┘
```

### Durable State

PostgreSQL owns durable entities:

- Players
- Parties
- Relationships
- Matches
- Match history

### Ephemeral State

The active matchmaking system owns:

- Queued parties
- Reservation state
- Queue timestamps
- Candidate indexes
- Temporary matchmaking metadata

The separation allows the matcher to operate against fast in-memory structures without turning PostgreSQL into the primary matchmaking search engine.

### Distributed State

Redis can be introduced when multiple matchmaking instances require shared ephemeral state, coordination, presence, TTLs, or distributed reservations.

### Event Transport

The initial implementation uses an in-process Go event bus.

Kafka can later provide durable asynchronous event transport when the system becomes distributed.

Kafka and Redis are therefore not required for the initial version.

## MatchCreated Event

After a match is successfully committed, the matchmaking system publishes an immutable `MatchCreated` event.

The event contains:

```text
matchID
gameMode
teamA
teamB
createdAt
matchmakingMetadata
```

Consumers can independently react to the event for:

- Player notification
- Game/session creation
- Analytics
- Match history

The matchmaking engine publishes the fact that a match was created rather than directly coupling itself to downstream services.

## Simulation

The project includes a simulation engine for evaluating matchmaking behavior under controlled workloads.

Target populations include:

```text
1,000 players
5,000 players
10,000 players
```

The simulator can vary:

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

Example scenarios include:

```text
Normal Load
Large Queue
Stress Test
High Party Load
Skill Polarization
Socially Dense Queue
Burst Arrival
Slow Arrival
```

## Metrics

The simulator measures both matchmaking quality and system performance.

### Queue Metrics

- P50 queue time
- P95 queue time
- P99 queue time
- Average queue time
- Queue depth

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
- Rejected candidates
- Rejection reason distribution
- Matcher CPU time
- Matchmaking latency
- Reservation failures

The primary performance experiment compares Matcher V1 against Matcher V2.

The desired result is to demonstrate a measurable reduction in candidate evaluations without unacceptable degradation in matchmaking quality.

For example:

> At 5,000 queued players, the candidate-indexed matcher reduced candidate evaluations by X% compared with exhaustive search while maintaining a P95 queue time of Y seconds.

## API

The HTTP API exposes the matchmaking engine and simulation system.

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

The HTTP layer is intentionally separated from the matchmaking domain so that the core engine can also be used directly by simulations and tests.

## Project Structure

```text
player-matchmaker/
├── cmd/
│   └── server/
│       └── main.go
│
├── internal/
│   ├── player/
│   │   ├── player.go
│   │   └── player_test.go
│   │
│   ├── party/
│   │   ├── party.go
│   │   └── party_test.go
│   │
│   ├── queue/
│   │   ├── queue.go
│   │   ├── reservation.go
│   │   └── queue_test.go
│   │
│   ├── match/
│   │   ├── match.go
│   │   └── match_test.go
│   │
│   ├── matchmaking/
│   │   ├── matcher.go
│   │   ├── scoring.go
│   │   ├── constraints.go
│   │   ├── index.go
│   │   └── matcher_test.go
│   │
│   ├── relationship/
│   │   └── graph.go
│   │
│   ├── simulation/
│   │   ├── simulation.go
│   │   ├── generator.go
│   │   ├── scenarios.go
│   │   ├── metrics.go
│   │   └── reporter.go
│   │
│   ├── events/
│   │   └── bus.go
│   │
│   └── persistence/
│       ├── player_repository.go
│       ├── party_repository.go
│       ├── relationship_repository.go
│       └── match_repository.go
│
├── migrations/
├── docs/
├── go.mod
├── go.sum
├── Plan.md
└── README.md
```

The exact structure may evolve as implementation progresses.

## Development Roadmap

The project is developed incrementally:

```text
Domain Model
     ↓
Matchmaking Pool
     ↓
Matcher V1 — Exhaustive
     ↓
Matcher V2 — Candidate Indexed
     ↓
Match Scoring
     ↓
Reservation & Concurrency
     ↓
Event System
     ↓
Simulation
     ↓
Metrics & Benchmarking
     ↓
PostgreSQL Persistence
     ↓
Distributed State
     ↓
HTTP API
     ↓
UI
```

The detailed implementation plan is maintained in [`Plan.md`](Plan.md).

## Running

The project requires Go 1.23+.

Clone the repository and run:

```bash
go run ./cmd/server
```

Run the complete test suite:

```bash
go test ./...
```

Run with the race detector:

```bash
go test -race ./...
```

Build the server:

```bash
go build -o player-matchmaker ./cmd/server
```

## Design Goals

The project prioritizes:

1. Correctness
2. Measurable matchmaking quality
3. Efficient candidate selection
4. Safe concurrent matchmaking
5. Clear separation of durable and ephemeral state
6. Decoupling through events
7. Reproducible simulation
8. Quantitative performance analysis

The objective is not simply to build an API that returns ten players.

The objective is to demonstrate the engineering progression from a correct but computationally expensive matchmaking algorithm into an efficient, concurrent, quality-aware matchmaking system that can be measured and reasoned about under realistic workloads.