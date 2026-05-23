package store

import (
	"fmt"
	"sync"
	"testing"
	"github.com/HanshalDabbiru/feature-flag-engine/pkg/domain"
)

func newStore() *Store {
	return &Store{flags: make(map[string]domain.FeatureFlag)}
}

func makeFlag(key string, enabled bool) domain.FeatureFlag {
	return domain.FeatureFlag{
		Key:          key,
		Description:  "test flag",
		Enabled:      enabled,
		DefaultValue: false,
	}
}

func TestGet_ExistingKey(t *testing.T) {
	s := newStore()
	flag := makeFlag("my-flag", true)
	s.flags["my-flag"] = flag

	got := s.Get("my-flag")

	if got.Key != flag.Key {
		t.Errorf("expected key %q, got %q", flag.Key, got.Key)
	}
	if got.Enabled != flag.Enabled {
		t.Errorf("expected Enabled %v, got %v", flag.Enabled, got.Enabled)
	}
}

func TestGet_MissingKey(t *testing.T) {
	s := newStore()

	got := s.Get("does-not-exist")

	if got.Key != "" {
		t.Errorf("expected zero-value flag, got key %q", got.Key)
	}
}

func TestSet_StoresFlag(t *testing.T) {
	s := newStore()
	flag := makeFlag("feature-x", true)

	s.Set("feature-x", flag)

	stored, ok := s.flags["feature-x"]
	if !ok {
		t.Fatal("expected flag to be stored, but key not found")
	}
	if stored.Key != flag.Key {
		t.Errorf("expected key %q, got %q", flag.Key, stored.Key)
	}
}

func TestSet_OverwritesExistingFlag(t *testing.T) {
	s := newStore()
	s.flags["flag-a"] = makeFlag("flag-a", true)

	updated := makeFlag("flag-a", false)
	s.Set("flag-a", updated)

	stored := s.flags["flag-a"]
	if stored.Enabled != false {
		t.Errorf("expected Enabled=false after overwrite, got %v", stored.Enabled)
	}
}

func TestDelete_RemovesExistingKey(t *testing.T) {
	s := newStore()
	s.flags["flag-b"] = makeFlag("flag-b", true)

	s.Delete("flag-b")

	if _, ok := s.flags["flag-b"]; ok {
		t.Error("expected flag to be deleted, but it still exists")
	}
}

func TestDelete_NonExistentKeyIsNoOp(t *testing.T) {
	s := newStore()

	// should not panic
	s.Delete("ghost-flag")
}

func TestList_EmptyStore(t *testing.T) {
	s := newStore()

	got := s.List()

	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d items", len(got))
	}
}

func TestList_ReturnsAllFlags(t *testing.T) {
	s := newStore()
	s.flags["alpha"] = makeFlag("alpha", true)
	s.flags["beta"] = makeFlag("beta", false)
	s.flags["gamma"] = makeFlag("gamma", true)

	got := s.List()

	if len(got) != 3 {
		t.Errorf("expected 3 flags, got %d", len(got))
	}
}

func TestList_ReturnsCopy(t *testing.T) {
	s := newStore()
	s.flags["x"] = makeFlag("x", true)

	got := s.List()
	got[0].Enabled = false

	// mutation of the returned slice must not affect the store
	if s.flags["x"].Enabled != true {
		t.Error("List returned a reference; mutating it changed the store")
	}
}

// TestConcurrent fires 50 writers and 50 readers simultaneously against
// overlapping keys. The WaitGroup ensures the test does not exit until every
// goroutine has finished.
func TestConcurrent(t *testing.T) {
	s := newStore()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("flag-%d", i)
			s.Set(key, domain.FeatureFlag{Key: key})
		}(i)
	}

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("flag-%d", i)
			s.Get(key)
		}(i)
	}

	wg.Wait()
}
