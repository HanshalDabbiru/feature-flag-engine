package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HanshalDabbiru/feature-flag-engine/pkg/domain"
)

// waitForFlag polls the client cache until key appears or the timeout elapses.
func waitForFlag(t *testing.T, c *Client, key string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if c.Get(key).Key != "" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for flag %q to appear in cache", key)
}

// TestGet_ReturnsZeroValueForMissingKey verifies that Get returns the zero value
// of domain.FeatureFlag when the requested key has never been received from the server.
func TestGet_ReturnsZeroValueForMissingKey(t *testing.T) {
	c := New("http://localhost")
	flag := c.Get("nonexistent")
	if flag.Key != "" {
		t.Errorf("expected empty Key, got %q", flag.Key)
	}
}

// TestConnect_ParsesSSEEvent verifies that a single well-formed SSE data line is
// unmarshalled into a FeatureFlag and stored in the local cache.
func TestConnect_ParsesSSEEvent(t *testing.T) {
	want := domain.FeatureFlag{Key: "checkout-v2", Enabled: true, DefaultValue: false}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(want)
		// Trailing blank line signals end of SSE event per the spec.
		fmt.Fprintf(w, "data: %s\n\n", b)
	}))
	defer srv.Close()

	c := New(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Connect runs the reconnect loop; run it in a goroutine and poll for the flag.
	go c.Connect(ctx) //nolint:errcheck

	waitForFlag(t, c, "checkout-v2", 2*time.Second)

	got := c.Get("checkout-v2")
	if got.Key != want.Key {
		t.Errorf("Key: got %q, want %q", got.Key, want.Key)
	}
	if got.Enabled != want.Enabled {
		t.Errorf("Enabled: got %v, want %v", got.Enabled, want.Enabled)
	}
}

// TestConnect_IgnoresNonDataLines verifies that comment lines (": ...") and blank
// lines are skipped and do not corrupt the cache or cause a parse error.
func TestConnect_IgnoresNonDataLines(t *testing.T) {
	want := domain.FeatureFlag{Key: "dark-mode", Enabled: true}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(want)
		fmt.Fprintf(w, ": heartbeat\n\n") // SSE comment — must be ignored
		fmt.Fprintf(w, "\n")              // bare blank line — must be ignored
		fmt.Fprintf(w, "data: %s\n\n", b)
	}))
	defer srv.Close()

	c := New(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Connect(ctx) //nolint:errcheck

	waitForFlag(t, c, "dark-mode", 2*time.Second)

	got := c.Get("dark-mode")
	if got.Key != want.Key {
		t.Errorf("Key: got %q, want %q", got.Key, want.Key)
	}
}

// TestConnect_MultipleEvents verifies that consecutive SSE events are all parsed
// and independently stored under their respective keys.
func TestConnect_MultipleEvents(t *testing.T) {
	flags := []domain.FeatureFlag{
		{Key: "flag-a", Enabled: true},
		{Key: "flag-b", Enabled: false},
		{Key: "flag-c", Enabled: true},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, f := range flags {
			b, _ := json.Marshal(f)
			fmt.Fprintf(w, "data: %s\n\n", b)
		}
	}))
	defer srv.Close()

	c := New(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Connect(ctx) //nolint:errcheck

	for _, want := range flags {
		waitForFlag(t, c, want.Key, 2*time.Second)
	}

	for _, want := range flags {
		got := c.Get(want.Key)
		if got.Key != want.Key {
			t.Errorf("Key: got %q, want %q", got.Key, want.Key)
		}
		if got.Enabled != want.Enabled {
			t.Errorf("%s Enabled: got %v, want %v", want.Key, got.Enabled, want.Enabled)
		}
	}
}

// TestConnect_ContextCancellation verifies that cancelling the context causes
// Connect to return promptly instead of blocking indefinitely on the open stream.
func TestConnect_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Flush headers immediately so http.Get on the client side returns before
		// we block — without this the client would never receive a response.
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Hold the connection open until the client disconnects.
		<-r.Context().Done()
	}))
	defer srv.Close()

	c := New(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())

	// Buffered so the goroutine doesn't leak if the test times out before Connect returns.
	done := make(chan error, 1)
	go func() {
		done <- c.Connect(ctx)
	}()

	// Closing the context triggers resp.Body.Close() inside Connect, which unblocks the scanner.
	cancel()

	select {
	case <-done:
		// Connect returned — success
	case <-time.After(2 * time.Second):
		t.Fatal("Connect did not return after context cancellation")
	}
}

// TestConnect_ServerError verifies that a non-200 response from the server is
// treated as an error rather than silently producing an empty stream.
func TestConnect_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(srv.URL)
	if err := c.Connect(context.Background()); err == nil {
		t.Fatal("expected non-nil error for HTTP 500, got nil")
	}
}

// TestConnect_ReconnectsAfterDisconnect verifies that Connect automatically
// reconnects after a dropped connection, accumulating flags from both sessions.
func TestConnect_ReconnectsAfterDisconnect(t *testing.T) {
	var reqCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Atomically increment so concurrent reconnects are counted correctly.
		n := atomic.AddInt32(&reqCount, 1)
		var key string
		switch n {
		case 1:
			key = "flag-a"
		case 2:
			key = "flag-b"
		default:
			// Extra reconnects after both flags are delivered — close immediately.
			return
		}
		b, _ := json.Marshal(domain.FeatureFlag{Key: key, Enabled: true})
		fmt.Fprintf(w, "data: %s\n\n", b)
	}))
	defer srv.Close()

	c := New(srv.URL)
	// 1ms delay so the test doesn't wait 2 seconds between reconnects.
	c.reconnectDelay = 1 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Connect(ctx) //nolint:errcheck

	// Block until the second connection has delivered its flag.
	waitForFlag(t, c, "flag-a", 2*time.Second)
	waitForFlag(t, c, "flag-b", 2*time.Second)

	cancel()

	if got := c.Get("flag-a"); got.Key != "flag-a" {
		t.Errorf("flag-a: got key %q", got.Key)
	}
	if got := c.Get("flag-b"); got.Key != "flag-b" {
		t.Errorf("flag-b: got key %q", got.Key)
	}
}

// TestConnect_ContextCancelledDuringDelay verifies that cancelling the context
// while Connect is waiting to reconnect causes it to return immediately rather
// than sleeping out the full reconnect delay.
func TestConnect_ContextCancelledDuringDelay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Close the connection immediately without writing anything.
	}))
	defer srv.Close()

	c := New(srv.URL)
	// Long delay so the test would block for 10 seconds if cancellation is not respected.
	c.reconnectDelay = 10 * time.Second

	ctx, cancel := context.WithCancel(context.Background())

	// Buffered so the goroutine doesn't leak if the test times out before Connect returns.
	done := make(chan error, 1)
	go func() {
		done <- c.Connect(ctx)
	}()

	// Give the first connection attempt time to complete before cancelling.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Connect returned — cancellation was respected during the delay.
	case <-time.After(1 * time.Second):
		t.Fatal("Connect did not return promptly after context cancellation during reconnect delay")
	}
}

// newClientWithFlag builds a client and seeds its cache directly, bypassing Connect.
func newClientWithFlag(flag domain.FeatureFlag) *Client {
	c := New("http://localhost")
	c.flags[flag.Key] = flag
	return c
}

// TestEvaluate_FlagNotFound verifies that evaluating an unknown key returns false
// (the zero value of bool, matching DefaultValue of a zero FeatureFlag).
func TestEvaluate_FlagNotFound(t *testing.T) {
	c := New("http://localhost")
	if got := c.Evaluate("missing", domain.UserContext{}); got != false {
		t.Errorf("expected false, got %v", got)
	}
}

// TestEvaluate_FlagDisabled verifies that a disabled flag always returns DefaultValue
// regardless of whether its rules would otherwise match.
func TestEvaluate_FlagDisabled(t *testing.T) {
	flag := domain.FeatureFlag{
		Key:     "my-flag",
		Enabled: false,
		Rules: []domain.Rule{
			{Predicates: []domain.Predicate{{Attribute: "country", Operator: domain.EQUALS, Values: []string{"US"}}}, Value: true},
		},
		DefaultValue: false,
	}
	c := newClientWithFlag(flag)
	if got := c.Evaluate("my-flag", domain.UserContext{"country": "US"}); got != false {
		t.Errorf("expected false for disabled flag, got %v", got)
	}
}

// TestEvaluate_NoRulesReturnsDefault verifies that an enabled flag with no rules
// falls through to DefaultValue.
func TestEvaluate_NoRulesReturnsDefault(t *testing.T) {
	flag := domain.FeatureFlag{Key: "my-flag", Enabled: true, DefaultValue: true}
	c := newClientWithFlag(flag)
	if got := c.Evaluate("my-flag", domain.UserContext{}); got != true {
		t.Errorf("expected true (DefaultValue), got %v", got)
	}
}

// TestEvaluate_EqualsOperator verifies EQUALS matches the exact value and rejects others.
func TestEvaluate_EqualsOperator(t *testing.T) {
	flag := domain.FeatureFlag{
		Key:     "my-flag",
		Enabled: true,
		Rules: []domain.Rule{
			{Predicates: []domain.Predicate{{Attribute: "country", Operator: domain.EQUALS, Values: []string{"US"}}}, Value: true},
		},
		DefaultValue: false,
	}
	c := newClientWithFlag(flag)
	if got := c.Evaluate("my-flag", domain.UserContext{"country": "US"}); got != true {
		t.Errorf("US: expected true, got %v", got)
	}
	if got := c.Evaluate("my-flag", domain.UserContext{"country": "CA"}); got != false {
		t.Errorf("CA: expected false, got %v", got)
	}
}

// TestEvaluate_NotEqualsOperator verifies NOT_EQUALS passes for non-matching values
// and fails for the listed value.
func TestEvaluate_NotEqualsOperator(t *testing.T) {
	flag := domain.FeatureFlag{
		Key:     "my-flag",
		Enabled: true,
		Rules: []domain.Rule{
			{Predicates: []domain.Predicate{{Attribute: "country", Operator: domain.NOT_EQUALS, Values: []string{"US"}}}, Value: true},
		},
		DefaultValue: false,
	}
	c := newClientWithFlag(flag)
	if got := c.Evaluate("my-flag", domain.UserContext{"country": "US"}); got != false {
		t.Errorf("US: expected false, got %v", got)
	}
	if got := c.Evaluate("my-flag", domain.UserContext{"country": "CA"}); got != true {
		t.Errorf("CA: expected true, got %v", got)
	}
}

// TestEvaluate_ContainsOperator verifies CONTAINS matches when the attribute value
// contains the predicate string and misses when it does not.
func TestEvaluate_ContainsOperator(t *testing.T) {
	flag := domain.FeatureFlag{
		Key:     "my-flag",
		Enabled: true,
		Rules: []domain.Rule{
			{Predicates: []domain.Predicate{{Attribute: "email", Operator: domain.CONTAINS, Values: []string{"@corp.com"}}}, Value: true},
		},
		DefaultValue: false,
	}
	c := newClientWithFlag(flag)
	if got := c.Evaluate("my-flag", domain.UserContext{"email": "user@corp.com"}); got != true {
		t.Errorf("corp email: expected true, got %v", got)
	}
	if got := c.Evaluate("my-flag", domain.UserContext{"email": "user@gmail.com"}); got != false {
		t.Errorf("gmail email: expected false, got %v", got)
	}
}

// TestEvaluate_StartsWithOperator verifies STARTS_WITH matches on the correct prefix
// and rejects values with a different prefix.
func TestEvaluate_StartsWithOperator(t *testing.T) {
	flag := domain.FeatureFlag{
		Key:     "my-flag",
		Enabled: true,
		Rules: []domain.Rule{
			{Predicates: []domain.Predicate{{Attribute: "plan", Operator: domain.STARTS_WITH, Values: []string{"pro"}}}, Value: true},
		},
		DefaultValue: false,
	}
	c := newClientWithFlag(flag)
	if got := c.Evaluate("my-flag", domain.UserContext{"plan": "pro-annual"}); got != true {
		t.Errorf("pro-annual: expected true, got %v", got)
	}
	if got := c.Evaluate("my-flag", domain.UserContext{"plan": "free"}); got != false {
		t.Errorf("free: expected false, got %v", got)
	}
}

// TestEvaluate_MultipleValuesOR verifies that EQUALS treats Values as an OR list —
// matching any one value is sufficient to satisfy the predicate.
func TestEvaluate_MultipleValuesOR(t *testing.T) {
	flag := domain.FeatureFlag{
		Key:     "my-flag",
		Enabled: true,
		Rules: []domain.Rule{
			{Predicates: []domain.Predicate{{Attribute: "country", Operator: domain.EQUALS, Values: []string{"US", "CA", "GB"}}}, Value: true},
		},
		DefaultValue: false,
	}
	c := newClientWithFlag(flag)
	if got := c.Evaluate("my-flag", domain.UserContext{"country": "CA"}); got != true {
		t.Errorf("CA: expected true, got %v", got)
	}
	if got := c.Evaluate("my-flag", domain.UserContext{"country": "AU"}); got != false {
		t.Errorf("AU: expected false, got %v", got)
	}
}

// TestEvaluate_MultiPredicateAND verifies that all predicates in a rule must match
// (AND logic) — failing any single predicate causes the rule to be skipped.
func TestEvaluate_MultiPredicateAND(t *testing.T) {
	flag := domain.FeatureFlag{
		Key:     "my-flag",
		Enabled: true,
		Rules: []domain.Rule{
			{
				Predicates: []domain.Predicate{
					{Attribute: "country", Operator: domain.EQUALS, Values: []string{"US"}},
					{Attribute: "plan", Operator: domain.STARTS_WITH, Values: []string{"pro"}},
				},
				Value: true,
			},
		},
		DefaultValue: false,
	}
	c := newClientWithFlag(flag)

	// Both predicates match — rule fires.
	if got := c.Evaluate("my-flag", domain.UserContext{"country": "US", "plan": "pro-annual"}); got != true {
		t.Errorf("US+pro: expected true, got %v", got)
	}
	// Country matches but plan does not — rule must not fire.
	if got := c.Evaluate("my-flag", domain.UserContext{"country": "US", "plan": "free"}); got != false {
		t.Errorf("US+free: expected false, got %v", got)
	}
	// Plan matches but country does not — rule must not fire.
	if got := c.Evaluate("my-flag", domain.UserContext{"country": "CA", "plan": "pro-annual"}); got != false {
		t.Errorf("CA+pro: expected false, got %v", got)
	}
}

// TestEvaluate_FirstMatchingRuleWins verifies that rules are evaluated in order and
// the value from the first matching rule is returned immediately.
func TestEvaluate_FirstMatchingRuleWins(t *testing.T) {
	flag := domain.FeatureFlag{
		Key:     "my-flag",
		Enabled: true,
		Rules: []domain.Rule{
			// Rule 1: country=US → true
			{Predicates: []domain.Predicate{{Attribute: "country", Operator: domain.EQUALS, Values: []string{"US"}}}, Value: true},
			// Rule 2: plan=pro → false (would win for a non-US pro user)
			{Predicates: []domain.Predicate{{Attribute: "plan", Operator: domain.EQUALS, Values: []string{"pro"}}}, Value: false},
		},
		DefaultValue: false,
	}
	c := newClientWithFlag(flag)

	// A US pro user must get true — rule 1 matches first.
	if got := c.Evaluate("my-flag", domain.UserContext{"country": "US", "plan": "pro"}); got != true {
		t.Errorf("US pro: expected true (rule 1 wins), got %v", got)
	}
	// A non-US pro user falls through to rule 2 → false.
	if got := c.Evaluate("my-flag", domain.UserContext{"country": "CA", "plan": "pro"}); got != false {
		t.Errorf("CA pro: expected false (rule 2), got %v", got)
	}
}
