package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HanshalDabbiru/feature-flag-engine/pkg/api"
	"github.com/HanshalDabbiru/feature-flag-engine/pkg/domain"
	"github.com/HanshalDabbiru/feature-flag-engine/pkg/hub"
	"github.com/HanshalDabbiru/feature-flag-engine/pkg/persistence"
	"github.com/HanshalDabbiru/feature-flag-engine/pkg/sdk"
	"github.com/HanshalDabbiru/feature-flag-engine/pkg/store"
)

// newTestServer wires up the full stack in-process and returns an httptest.Server
// together with a pre-configured SDK client pointing at it.
func newTestServer(t *testing.T) (*httptest.Server, *sdk.Client) {
	t.Helper()
	s := store.New()
	p := persistence.New(filepath.Join(t.TempDir(), "flags.json"), s)
	h := hub.New()
	handler := api.New(s, p, h)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := sdk.New(srv.URL)
	client.SetReconnectDelay(1 * time.Millisecond)
	return srv, client
}

// waitForCondition polls cond every 5 ms until it returns true or the timeout elapses.
func waitForCondition(t *testing.T, cond func() bool, timeout time.Duration, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal(msg)
}

// TestEndToEnd_CreateFlagReachesSDK verifies the full path: a POST to /flags is
// stored, broadcast over SSE, and received by the SDK client's local cache.
func TestEndToEnd_CreateFlagReachesSDK(t *testing.T) {
	srv, client := newTestServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go client.Connect(ctx) //nolint:errcheck

	body := `{"Key":"e2e-flag","Enabled":true,"DefaultValue":false}`
	resp, err := http.Post(srv.URL+"/flags", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /flags: %v", err)
	}
	// Decode the response to confirm the server accepted and echoed the flag.
	var created domain.FeatureFlag
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decoding POST response: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /flags: got %d, want 201", resp.StatusCode)
	}

	waitForCondition(t, func() bool {
		return client.Get("e2e-flag").Key != ""
	}, 3*time.Second, "timed out waiting for e2e-flag to appear in SDK cache")

	if got := client.Get("e2e-flag"); !got.Enabled {
		t.Errorf("Enabled: got false, want true")
	}
}

// TestEndToEnd_ToggleFlagReachesSDK verifies that a PUT to /flags/{key} broadcasts
// the updated flag and the SDK cache reflects the new Enabled state.
func TestEndToEnd_ToggleFlagReachesSDK(t *testing.T) {
	srv, client := newTestServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go client.Connect(ctx) //nolint:errcheck

	// Create the flag with Enabled=true.
	body := `{"Key":"e2e-flag","Enabled":true,"DefaultValue":false}`
	resp, err := http.Post(srv.URL+"/flags", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /flags: %v", err)
	}
	resp.Body.Close()

	// Wait for the create event to land in the cache.
	waitForCondition(t, func() bool {
		return client.Get("e2e-flag").Key != ""
	}, 3*time.Second, "timed out waiting for e2e-flag to appear after create")

	// Toggle: flips Enabled from true to false.
	req, err := http.NewRequest(http.MethodPut, srv.URL+"/flags/e2e-flag", nil)
	if err != nil {
		t.Fatalf("building PUT request: %v", err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /flags/e2e-flag: %v", err)
	}
	// Decode to confirm the server returned the toggled flag.
	var toggled domain.FeatureFlag
	if err := json.NewDecoder(resp.Body).Decode(&toggled); err != nil {
		t.Fatalf("decoding PUT response: %v", err)
	}
	resp.Body.Close()

	// Poll until the toggle SSE event updates the SDK cache.
	waitForCondition(t, func() bool {
		return !client.Get("e2e-flag").Enabled
	}, 3*time.Second, "timed out waiting for e2e-flag to become disabled in SDK cache")
}

// TestEndToEnd_MultiClientBroadcast verifies that a single flag creation is
// broadcast to all connected SDK clients simultaneously.
func TestEndToEnd_MultiClientBroadcast(t *testing.T) {
	srv, client1 := newTestServer(t)

	client2 := sdk.New(srv.URL)
	client2.SetReconnectDelay(1 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go client1.Connect(ctx) //nolint:errcheck
	go client2.Connect(ctx) //nolint:errcheck

	// Allow both SSE connections to register with the hub before broadcasting.
	time.Sleep(20 * time.Millisecond)

	body := `{"Key":"e2e-flag","Enabled":true,"DefaultValue":false}`
	resp, err := http.Post(srv.URL+"/flags", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /flags: %v", err)
	}
	resp.Body.Close()

	waitForCondition(t, func() bool {
		return client1.Get("e2e-flag").Key != ""
	}, 3*time.Second, "client1: timed out waiting for e2e-flag")

	waitForCondition(t, func() bool {
		return client2.Get("e2e-flag").Key != ""
	}, 3*time.Second, "client2: timed out waiting for e2e-flag")

	if got := client1.Get("e2e-flag"); !got.Enabled {
		t.Errorf("client1 Enabled: got false, want true")
	}
	if got := client2.Get("e2e-flag"); !got.Enabled {
		t.Errorf("client2 Enabled: got false, want true")
	}
}
