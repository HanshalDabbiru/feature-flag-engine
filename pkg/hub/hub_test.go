package hub

import (
	"sync"
	"testing"

	"github.com/HanshalDabbiru/feature-flag-engine/pkg/domain"
)

func TestRegister(t *testing.T) {
	h := New()

	id1, ch1 := h.Register()
	id2, ch2 := h.Register()

	if id1 == id2 {
		t.Errorf("expected distinct IDs, got %d twice", id1)
	}
	if ch1 == nil {
		t.Error("expected non-nil channel for first client")
	}
	if ch2 == nil {
		t.Error("expected non-nil channel for second client")
	}
}

func TestUnregister(t *testing.T) {
	h := New()
	id, ch := h.Register()

	h.Unregister(id)

	// reading from a closed channel returns the zero value with ok=false
	_, ok := <-ch
	if ok {
		t.Error("expected channel to be closed after Unregister")
	}
}

func TestUnregister_NotFound(t *testing.T) {
	h := New()

	// should not panic for an ID that was never registered
	h.Unregister(999)
}

func TestBroadcast(t *testing.T) {
	h := New()
	flag := domain.FeatureFlag{Key: "broadcast-flag", Enabled: true}

	_, ch1 := h.Register()
	_, ch2 := h.Register()

	h.Broadcast(flag)

	for i, ch := range []chan domain.FeatureFlag{ch1, ch2} {
		select {
		case got := <-ch:
			if got.Key != flag.Key {
				t.Errorf("client %d: expected key %q, got %q", i+1, flag.Key, got.Key)
			}
			if got.Enabled != flag.Enabled {
				t.Errorf("client %d: expected Enabled=%v, got %v", i+1, flag.Enabled, got.Enabled)
			}
		default:
			t.Errorf("client %d: channel did not receive the broadcast", i+1)
		}
	}
}

func TestBroadcast_NoClients(t *testing.T) {
	h := New()

	// should not panic or block when there are no registered clients
	h.Broadcast(domain.FeatureFlag{Key: "no-clients-flag"})
}

// TestConcurrent registers 50 clients, broadcasts a flag to all of them, and
// unregisters them concurrently. Run with -race to verify no data races.
func TestConcurrent(t *testing.T) {
	h := New()
	const n = 50
	flag := domain.FeatureFlag{Key: "concurrent-flag"}

	type entry struct {
		id uint64
		ch chan domain.FeatureFlag
	}

	entries := make([]entry, n)
	var mu sync.Mutex

	// registerWg ensures all clients are registered before Broadcast is called.
	// doneWg waits for all goroutines to finish receiving and unregistering.
	var registerWg, doneWg sync.WaitGroup

	for i := 0; i < n; i++ {
		registerWg.Add(1)
		doneWg.Add(1)
		go func(i int) {
			defer doneWg.Done()

			id, ch := h.Register()
			mu.Lock()
			entries[i] = entry{id: id, ch: ch}
			mu.Unlock()
			registerWg.Done()

			<-ch // block until Broadcast delivers the flag
			h.Unregister(id)
		}(i)
	}

	// wait for all clients to be registered before broadcasting
	registerWg.Wait()
	h.Broadcast(flag)
	doneWg.Wait()
}
