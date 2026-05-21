# Feature Flag Engine — 3 Week Build Plan

## Week 1 — Foundation & Core Data Layer

### Day 1 — Project Scaffolding
- go mod init
- Create full folder structure (pkg/domain, pkg/store, pkg/hub, pkg/api, pkg/sdk, pkg/persistence, cmd/server)
- Create minimal cmd/server/main.go that compiles and runs
- Create .gitignore for Go
- Update CLAUDE.md with build and test commands

### Day 2 — Domain Types
- Write pkg/domain/domain.go
- Define all structs: FeatureFlag, Rule, Predicate, UserContext
- Define Operator constants: EQUALS, NOT_EQUALS, CONTAINS, STARTS_WITH
- Write domain unit tests

### Day 3 — Memory Store
- Build pkg/store/ with sync.RWMutex
- Implement Get, Set, Delete, List operations
- Write concurrent read tests
- Run go test -race

### Day 4 — File Persistence
- Build pkg/persistence/
- Implement flush store → flags.json on write
- Implement rehydrate from flags.json on boot
- Write persistence tests

### Day 5 — Store + Persistence Integration
- Wire store writes to trigger persistence flush
- Implement boot sequence: load from disk into store
- Integration test: write flag → restart server → flag still exists

---

## Week 2 — API, Streaming & Fan-Out Hub

### Day 6 — Admin API Skeleton
- Set up net/http router
- POST /flags — create a flag
- GET /flags — list all flags

### Day 7 — Admin API (continued)
- GET /flags/:key — get single flag
- PUT /flags/:key — update and toggle flag
- Proper error handling and JSON responses

### Day 8 — API Tests & cmd/server
- Write HTTP handler unit tests
- Wire everything up in cmd/server/main.go
- Smoke test all endpoints with curl

### Day 9 — Fan-Out Hub
- Build pkg/hub/
- Client channel registry (map of channels)
- Register and unregister operations
- Broadcast state delta to all connected clients

### Day 10 — SSE Streaming Endpoint
- GET /stream endpoint
- Set correct SSE headers (Content-Type: text/event-stream, Cache-Control, Connection)
- Spawn goroutine per client connection
- Monitor context.Done() for disconnect and clean up channel

### Day 11 — Hub + API Integration
- Wire PUT /flags/:key toggle → hub broadcast
- Push state delta on every flag write
- Manual test with multiple connected clients

### Day 12 — Hub Tests & Race Detector
- Write hub unit tests
- Run go test -race across all packages
- Fix any race conditions found

---

## Week 3 — Client SDK, Rule Evaluation & Polish

### Day 13 — SDK: SSE Connection
- Build pkg/sdk/
- Open persistent HTTP connection to /stream
- Parse incoming SSE events
- Store received flags in local cache

### Day 14 — SDK: Rule Evaluation
- Evaluate rules against UserContext
- Implement all predicate operators (EQUALS, NOT_EQUALS, CONTAINS, STARTS_WITH)
- AND logic across predicates within a rule
- Fall through to DefaultValue if no rules match
- Only evaluate if flag.Enabled == true

### Day 15 — SDK: Reconnection & Tests
- Auto-reconnect on disconnect
- Write SDK unit tests
- Test all operator types against edge cases

### Day 16 — End-to-End Integration
- Full flow test: toggle flag via API → SSE broadcast → SDK evaluates locally
- Multi-client broadcast test
- Verify 0ms local evaluation (no network call on Evaluate)

### Day 17 — Error Handling Pass
- Audit every error return across all packages
- Graceful shutdown on os.Signal
- Handle malformed JSON input edge cases

### Day 18 — Race Detector & Load Test
- go test -race across entire codebase
- Simulate 100 concurrent SSE clients
- Check for goroutine leaks on disconnect

### Day 19 — Docs & README
- Write README with quickstart guide
- Document all API endpoints with request/response examples
- Add example SDK usage code

### Day 20 — Final Polish
- Review all exported doc comments
- Clean up any remaining TODOs
- Final go vet and go test ./...

### Day 21 — Buffer / Stretch Goals
- Catch up on anything that slipped
- Stretch: Makefile with common targets
- Stretch: Docker support