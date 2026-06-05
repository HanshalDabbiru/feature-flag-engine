package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HanshalDabbiru/feature-flag-engine/pkg/domain"
)

func TestGet_ReturnsZeroValueForMissingKey(t *testing.T) {
	c := New("http://localhost")
	flag := c.Get("nonexistent")
	if flag.Key != "" {
		t.Errorf("expected empty Key, got %q", flag.Key)
	}
}

func TestConnect_ParsesSSEEvent(t *testing.T) {
	want := domain.FeatureFlag{Key: "checkout-v2", Enabled: true, DefaultValue: false}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(want)
		fmt.Fprintf(w, "data: %s\n\n", b)
	}))
	defer srv.Close()

	c := New(srv.URL)
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect returned unexpected error: %v", err)
	}

	got := c.Get("checkout-v2")
	if got.Key != want.Key {
		t.Errorf("Key: got %q, want %q", got.Key, want.Key)
	}
	if got.Enabled != want.Enabled {
		t.Errorf("Enabled: got %v, want %v", got.Enabled, want.Enabled)
	}
}

func TestConnect_IgnoresNonDataLines(t *testing.T) {
	want := domain.FeatureFlag{Key: "dark-mode", Enabled: true}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(want)
		fmt.Fprintf(w, ": heartbeat\n\n")
		fmt.Fprintf(w, "\n")
		fmt.Fprintf(w, "data: %s\n\n", b)
	}))
	defer srv.Close()

	c := New(srv.URL)
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect returned unexpected error: %v", err)
	}

	got := c.Get("dark-mode")
	if got.Key != want.Key {
		t.Errorf("Key: got %q, want %q", got.Key, want.Key)
	}
}

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
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect returned unexpected error: %v", err)
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

func TestConnect_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	defer srv.Close()

	c := New(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- c.Connect(ctx)
	}()

	cancel()

	select {
	case <-done:
		// Connect returned — success
	case <-time.After(2 * time.Second):
		t.Fatal("Connect did not return after context cancellation")
	}
}

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
