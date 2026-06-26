# Feature Flag Engine

A high-performance, single-node feature flag orchestration engine written in pure Go.
Flags are pushed to every connected client over Server-Sent Events (SSE), so evaluation
happens in local memory with **0 ms latency** — no network round-trip per check.

---

## How it works

```
┌─────────────────────────────────────────────┐
│                 Server (:8080)              │
│                                             │
│  Admin API (POST/GET/PUT/DELETE /flags)     │
│       │                                     │
│       ▼                                     │
│  In-memory Store ──► Broadcast Hub         │
│       │                    │               │
│       ▼                    ▼               │
│  flags.json          SSE Stream (/stream)  │
└─────────────────────────────────────────────┘
                             │
              ───────────────┼───────────────
             │               │               │
        SDK Client      SDK Client      SDK Client
        (local cache)  (local cache)  (local cache)
```

1. You create, toggle, or delete a flag via the HTTP Admin API.
2. The server writes the change to `flags.json` (durable storage) and broadcasts it to
   every connected SDK client over a persistent SSE connection.
3. Each SDK client keeps a local in-memory copy of all flags. Evaluating a flag is a
   simple map lookup — no network call needed.

---

## Quickstart

### Prerequisites

- [Go 1.24+](https://go.dev/dl/) — no other dependencies required.

### 1. Clone and run

```bash
git clone https://github.com/HanshalDabbiru/feature-flag-engine.git
cd feature-flag-engine
go run cmd/server/main.go
```

You should see:

```
2026/06/26 00:00:00 server started on :8080
```

The server loads any flags from `flags.json` on startup. If the file doesn't exist yet,
it starts with an empty flag set and creates the file on your first write.

### 2. Watch the live stream

Open a second terminal and run:

```bash
curl -N http://localhost:8080/stream
```

This connection stays open. Every flag change you make will appear here within
milliseconds. Leave it running while you follow the steps below.

### 3. Create a flag

Open a third terminal and run:

```bash
curl -s -X POST http://localhost:8080/flags \
  -H "Content-Type: application/json" \
  -d '{"Key":"dark-mode","Enabled":true,"DefaultValue":false}' | jq .
```

Switch back to your stream terminal — the event arrives instantly:

```
data: {"Key":"dark-mode","Enabled":true,"DefaultValue":false,...}
```

### 4. Toggle the flag on and off

```bash
curl -s -X PUT http://localhost:8080/flags/dark-mode | jq .
```

Run this a few times and watch `Enabled` flip between `true` and `false` in the stream
each time. This is the core of the push model — every connected client receives the
update the moment it happens.

### 5. List and inspect flags

```bash
# All flags
curl -s http://localhost:8080/flags | jq .

# One flag by key
curl -s http://localhost:8080/flags/dark-mode | jq .
```

### 6. Delete a flag

```bash
curl -s -X DELETE http://localhost:8080/flags/dark-mode
```

### 7. Stop the server

Press `Ctrl+C`. The server shuts down gracefully, finishing any in-flight requests
before exiting.

---

## Flag schema

| Field | Type | Description |
|---|---|---|
| `Key` | string | Unique identifier — e.g. `"dark-mode"` |
| `Description` | string | Human-readable label (optional) |
| `Enabled` | bool | Master switch. If `false`, `DefaultValue` is always returned regardless of rules |
| `DefaultValue` | bool | Returned when no rule matches or the flag is disabled |
| `Rules` | array | Ordered list of targeting rules (optional) |

### Targeting rules

Rules let you return `true` for a specific group of users and `DefaultValue` for
everyone else. Each rule has a `Value` (the result when it matches) and one or more
`Predicates` that must **all** pass (AND logic) for the rule to fire. Rules are
evaluated in order — the first match wins.

**Supported predicate operators:**

| Operator | Matches when… |
|---|---|
| `EQUALS` | the user's attribute equals any value in the list |
| `NOT_EQUALS` | the user's attribute does not equal any value in the list |
| `CONTAINS` | the user's attribute contains any value as a substring |
| `STARTS_WITH` | the user's attribute starts with any value in the list |

A flag with a rule that targets `pro` and `enterprise` plan users looks like:

```json
{
  "Key": "new-checkout",
  "Enabled": true,
  "DefaultValue": false,
  "Rules": [
    {
      "Name": "beta-users",
      "Value": true,
      "Predicates": [
        {
          "Attribute": "plan",
          "Operator": "EQUALS",
          "Values": ["pro", "enterprise"]
        }
      ]
    }
  ]
}
```

---

## Admin API reference

| Method | Path | Description |
|---|---|---|
| `POST` | `/flags` | Create a new flag |
| `GET` | `/flags` | List all flags |
| `GET` | `/flags/{key}` | Get a single flag by key |
| `PUT` | `/flags/{key}` | Toggle `Enabled` (true → false or false → true) |
| `DELETE` | `/flags/{key}` | Delete a flag |
| `GET` | `/stream` | SSE stream — connect to receive live flag updates |
| `GET` | `/health` | Returns `ok` if the server is up |

---

## Running the tests

```bash
# All packages
go test ./...

# With the race detector (recommended)
go test -race ./...

# A single package
go test ./pkg/sdk/

# Integration and load tests with verbose output
go test -race -v ./cmd/server/
```

---

## Project structure

```
cmd/server/
  main.go                  — entry point, wires all packages, graceful shutdown
  integration_test.go      — end-to-end, load, and goroutine leak tests

pkg/
  domain/                  — FeatureFlag, Rule, Predicate, and UserContext types
  store/                   — thread-safe in-memory flag cache (RWMutex)
  persistence/             — read/write flags to flags.json
  hub/                     — fan-out hub; broadcasts flag updates to all SSE clients
  api/                     — HTTP handlers (CRUD + /stream endpoint)
  sdk/                     — client SDK (SSE consumer + local flag evaluation)
```
