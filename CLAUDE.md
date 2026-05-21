# Project: Concurrent Feature Flag Engine

A high-performance, single-node feature flag orchestration engine written in native Go.
Uses a push-based SSE architecture so client SDKs evaluate flags in local memory with 0ms latency.

## Stack
- Language: Go (native only — no frameworks)
- Router: `net/http` (stdlib)
- Storage: `flags.json` (local file, no external DB)
- Streaming: Server-Sent Events (SSE)
- Concurrency: `sync.RWMutex`, goroutines, Go channels

## Planned Folder Structure
- `pkg/domain/` — core structs and types (FeatureFlag, Rule, Predicate, UserContext)
- `pkg/store/` — in-memory cache with RWMutex
- `pkg/hub/` — fan-out hub and SSE broadcast logic
- `pkg/api/` — HTTP admin CRUD endpoints
- `pkg/sdk/` — client SDK (SSE consumer + local evaluation)
- `pkg/persistence/` — flags.json read/write layer
- `cmd/server/` — main entrypoint

## Core Domain Rules
- A FeatureFlag contains an ordered list of Rules
- A Rule contains an array of Predicates joined by AND (all must match)
- Supported operators: EQUALS, NOT_EQUALS, CONTAINS, STARTS_WITH
- Flag evaluation falls through to DefaultValue if no rules match
- Flags are only evaluated if Enabled == true

## Concurrency Rules
- All reads on the memory store use RLock (concurrent, non-blocking)
- All writes use Lock (exclusive) — no exceptions
- Every SSE client connection runs in its own goroutine
- Use request.Context().Done() to detect disconnects and clean up channels
- No goroutine should be left running after a client disconnects

## Code Style
- No third-party dependencies — stdlib only
- No global variables — pass dependencies explicitly
- Errors must be returned and handled, never silently swallowed
- Keep functions small and single-purpose
- All exported types and functions must have a doc comment

## Build & Test Commands
- Build: `go build ./cmd/server/`
- Run: `go run cmd/server/main.go`
- Test all packages: `go test ./...`
- Test with race detector: `go test -race ./...`
- Test single package: `go test ./pkg/store/`

## Rules
- Never push directly to main — use feature branches
- Write tests alongside each package as it's built
- When a bug is found and fixed, log it in lessons.md so it isn't repeated
- Do not introduce a database or external dependency without explicit discussion first

## Project Plan
- Full day-by-day schedule: `docs/plan.md`
- Check this at the start of each session to confirm which day we're on