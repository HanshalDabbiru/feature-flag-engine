package store

import (
	"sync"
	"github.com/HanshalDabbiru/feature-flag-engine/pkg/domain"
)

// Store is the in-memory cache of all feature flags. All methods are safe for
// concurrent use: reads acquire a shared RLock; writes acquire an exclusive Lock.
type Store struct {
	mu sync.RWMutex
	flags map[string]domain.FeatureFlag
}

// Get returns the FeatureFlag for the given key, or a zero-value FeatureFlag
// if the key does not exist.
func (s *Store) Get(key string) domain.FeatureFlag {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.flags[key]
}

// Set inserts or overwrites the FeatureFlag at the given key.
func (s *Store) Set(key string, flag domain.FeatureFlag) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flags[key] = flag
}

// Delete removes the FeatureFlag at the given key. It is a no-op if the key
// does not exist.
func (s *Store) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.flags, key)
}

// List returns a snapshot of all feature flags as a new slice. Callers may
// mutate the returned slice without affecting the store.
func (s *Store) List() []domain.FeatureFlag {
	s.mu.RLock()
	
	defer s.mu.RUnlock()
	flags := make([]domain.FeatureFlag, 0, len(s.flags))
	for _, flag := range s.flags {
		flags = append(flags, flag)
	}
	return flags
}