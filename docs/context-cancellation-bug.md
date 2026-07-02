# Bug Report: Context Cancellation Not Propagated in SDK HTTP Requests

## Overview

While writing a goroutine leak test, we discovered that the SDK's `Connect` function
could get permanently stuck — even after the caller told it to stop. This document
explains what went wrong, why it went wrong, and how the fix works. Every concept is
explained from the ground up.

---

## Background Concepts

### What is a Goroutine?

A goroutine is Go's version of a lightweight thread. You start one with the `go` keyword:

```go
go doSomething()
```

This runs `doSomething` concurrently with the rest of your program. Goroutines are cheap
— you can have hundreds or thousands. The problem is that if a goroutine gets stuck and
never exits, it sits in memory forever consuming resources. This is called a **goroutine
leak**.

### What is Context?

In Go, a `context.Context` is a value you pass into long-running functions to give them
a way to be cancelled. Think of it like a shared "stop" signal:

```go
ctx, cancel := context.WithCancel(context.Background())

go doLongWork(ctx) // pass the context in

cancel() // later, signal the work to stop
```

When `cancel()` is called, the context's `Done()` channel is closed. Any code waiting
on `<-ctx.Done()` will immediately unblock and can clean up.

The contract in Go is: **if you accept a `context.Context`, you are responsible for
stopping when it is cancelled.** This is one of Go's most important conventions.

### What is SSE (Server-Sent Events)?

Server-Sent Events is a protocol where a client opens a single long-lived HTTP connection
to a server, and the server pushes updates down that connection over time. There is no
back-and-forth — the server just streams data whenever something new happens.

In this project, SDK clients open a `GET /stream` connection. The server holds it open
and sends a message every time a feature flag changes. The SDK reads those messages,
updates its local cache, and closes the loop.

```
Client ──── GET /stream ────► Server
           ◄── data: {...} ───
           ◄── data: {...} ───
           ◄── data: {...} ───
           (connection stays open indefinitely)
```

### What is http.Get?

`http.Get(url)` is the simplest way to make an HTTP request in Go:

```go
resp, err := http.Get("http://example.com/data")
```

It is a **blocking** call — the current goroutine waits until the server responds.
Critically, `http.Get` has **no way to be cancelled**. Once called, it runs until
the server responds or the network fails. There is no way to inject a context into it.

### What is http.NewRequestWithContext?

This is the context-aware alternative to `http.Get`. You build a request and attach a
context to it:

```go
req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
resp, err := http.DefaultClient.Do(req)
```

Now, if `ctx` is cancelled at any point — even while the request is already in flight
— the HTTP client immediately abandons the request and `Do` returns an error. This is
the correct way to make cancellable HTTP calls in Go.

---

## The Bug

### Where it lived: `pkg/sdk/sdk.go`

The `Connect` function is the heart of the SDK. Its job is to open an SSE connection,
read flag updates, and — when the context is cancelled — shut down cleanly.

Here is the relevant code **before the fix**:

```go
func (c *Client) Connect(ctx context.Context) error {
    for {
        // ❌ BUG: http.Get ignores ctx entirely
        resp, err := http.Get(c.serverURL + "/stream")
        if err != nil {
            return err
        }

        // ... read SSE events from resp.Body ...

        // Reconnect delay: wait 1ms, then loop back to the http.Get above
        timer := time.NewTimer(c.reconnectDelay)
        select {
        case <-timer.C:
            continue           // timer fired first → reconnect
        case <-ctx.Done():
            timer.Stop()
            return nil         // context cancelled → clean exit
        }
    }
}
```

See the issue? The `ctx` that was passed in is completely ignored for the actual HTTP
request. The `select` at the bottom does check `ctx.Done()`, but only **after the
SSE stream ends**. While the HTTP call is in progress, the context has no effect.

### Why Does the Reconnect Timer Make This Worse?

After an SSE connection ends (for any reason), `Connect` waits for the reconnect delay
before trying again. The `select` here is a race between two channels:

- `timer.C` fires after `reconnectDelay` (1 ms in tests)
- `ctx.Done()` is already closed if the context was cancelled

In Go, when **multiple channels in a `select` are ready at the same time**, Go picks
one **at random**. This is specified in the language spec — it is not first-come,
first-served.

With a 1 ms reconnect delay, there is a real chance the timer fires before the goroutine
even reaches the `select` statement. This happens because:

1. `cancel()` is called — ctx is now done.
2. The goroutine closes `resp.Body` (SSE stream ends).
3. The goroutine calls `connCancel()` and creates a 1 ms timer.
4. The goroutine is **scheduled out by the OS** — another goroutine runs instead.
5. **1 ms passes**.
6. The goroutine resumes and reaches the `select`.
7. NOW both `timer.C` **and** `ctx.Done()` are ready at the same time.
8. Go picks randomly — 50 % of the time it picks `timer.C`.
9. The goroutine loops back to `http.Get` and starts reconnecting.

With 100 goroutines all doing this at once on a system under load (especially with
Go's race detector enabled, which slows everything down 5–10×), a significant number
of goroutines will pick the timer and start reconnecting.

### Why Do Reconnecting Goroutines Get Permanently Stuck?

Once a goroutine wins the timer race and calls `http.Get`:

1. `http.Get` establishes a TCP connection to the server.
2. It sends an HTTP `GET /stream` request.
3. It waits for the server to respond with `200 OK`.

This wait happens inside Go's HTTP transport in a `select` statement:

```
waiting for: server response OR connection closed OR request cancelled OR context done
```

Since `http.Get` was called **without a context**, the "context done" case is simply
absent — it is a `nil` channel, which in Go's `select` means it can never fire. The
goroutine will wait **forever** unless the server responds or the connection is
forcibly closed.

Meanwhile, if the server is busy handling 100 existing SSE connections plus 100 new
reconnect requests, it might take a very long time to respond to each new request.
With the race detector adding overhead, "a very long time" can exceed the entire test
timeout.

### The Symptom: `wg.Wait()` Hangs Forever

The test used a `sync.WaitGroup` to wait for all 100 `Connect` goroutines to exit:

```go
cancel()
wg.Wait() // ← hangs here indefinitely
```

Because some goroutines were stuck inside `http.Get` (which ignores the cancelled
context), `wg.Done()` was never called for those goroutines. `wg.Wait()` blocked
until the 30-second test timeout killed everything.

---

## The Fix

### Change in `pkg/sdk/sdk.go`

Replace `http.Get` with `http.NewRequestWithContext` so the context is wired into
the HTTP layer:

```go
func (c *Client) Connect(ctx context.Context) error {
    for {
        // ✅ FIX: build the request with ctx so cancellation propagates
        req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.serverURL+"/stream", nil)
        if err != nil {
            return err
        }
        resp, err := http.DefaultClient.Do(req)
        if err != nil {
            // When ctx is cancelled, Do returns an error. Treat this as a clean exit,
            // not an unexpected failure.
            if ctx.Err() != nil {
                return nil
            }
            return err
        }

        // ... rest of the function is unchanged ...
    }
}
```

### Why This Works

With the context attached to the request, `http.DefaultClient.Do(req)` now
participates in context cancellation at every stage:

| Stage | Before fix | After fix |
|---|---|---|
| Waiting for TCP connection | Cannot be cancelled | Cancelled immediately |
| Waiting for server response headers | Cannot be cancelled | Cancelled immediately |
| Reading response body | Already cancelled (connCtx handled this) | Also cancelled |
| Reconnect select | Already worked | Still works |

Now, when `cancel()` is called:

**Path A — goroutine is in the reconnect select:**
```
cancel() fires
→ ctx.Done() fires in the select
→ Connect returns nil ✓
```

**Path B — goroutine won the timer race and is inside Do:**
```
cancel() fires
→ Do() sees ctx is cancelled and returns immediately with an error
→ ctx.Err() != nil, so we return nil (clean exit)
→ goroutine exits ✓
```

In both cases, every goroutine exits. `wg.Done()` is called for all 100.
`wg.Wait()` returns promptly. No goroutine is left running after the test.

---

## The Leak Test

The test `TestLoadTest_NoGoroutineLeakOnDisconnect` in
`cmd/server/integration_test.go` verifies the full cleanup chain:

```
client goroutine cancelled
    → resp.Body closed
        → server detects TCP disconnect
            → r.Context().Done() fires in SSE handler
                → handler returns
                    → defer hub.Unregister(id) runs
                        → hub.Len() decrements
```

Instead of using the fragile `runtime.NumGoroutine()` (which includes Go's internal
HTTP transport goroutines that are unrelated to our code), the test checks
`hub.Len()` — the number of currently registered SSE clients on the server side.
When this reaches 0, every `Stream` handler has exited and every `Unregister` has
run. That is a precise, deterministic signal that the server has fully cleaned up.

The `Len()` method added to `pkg/hub/hub.go` uses `RLock` (a read lock) because
it only reads the map — multiple goroutines can call `Len()` simultaneously without
blocking each other:

```go
func (h *Hub) Len() int {
    h.mu.RLock()
    defer h.mu.RUnlock()
    return len(h.clients)
}
```

---

## Lesson

> **Any function that accepts a `context.Context` must propagate it to every
> blocking operation inside it.** Accepting a context but ignoring it in a nested
> call is the same as not accepting it at all. In Go, the idiom `http.Get(url)` is
> only appropriate for throwaway scripts. In any production or test-facing code,
> use `http.NewRequestWithContext(ctx, ...)` so that cancellation, deadlines, and
> timeouts work end-to-end.

This bug class — "context accepted but not propagated" — is one of the most common
sources of goroutine leaks in Go codebases. The race detector (`go test -race`) will
not catch it, because it is not a data race. A goroutine leak test like
`TestLoadTest_NoGoroutineLeakOnDisconnect` is specifically designed to surface it.
