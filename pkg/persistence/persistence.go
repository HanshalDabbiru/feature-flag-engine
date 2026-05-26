package persistence

import (
	"encoding/json"
	"os"

	"github.com/HanshalDabbiru/feature-flag-engine/pkg/domain"
	"github.com/HanshalDabbiru/feature-flag-engine/pkg/store"
)

// Persistence handles reading and writing flags to disk.
type Persistence struct {
	path  string
	store *store.Store
}

// New creates a new Persistence instance.
func New(path string, store *store.Store) *Persistence {
	return &Persistence{path: path, store: store}
}

// Flush serialises all flags currently held in the store to the configured
// JSON file, overwriting any previous contents.
func (p *Persistence) Flush() error {
	flags := p.store.List()
	data, err := json.Marshal(flags)
	if err != nil {
		return err
	}
	return os.WriteFile(p.path, data, 0644)
}

// Load reads the JSON file at the configured path and populates the store with
// its flags. If the file does not exist, Load returns nil and leaves the store
// unchanged — the engine treats a missing file as an empty flag set.
func (p *Persistence) Load() error {
	data, err := os.ReadFile(p.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var flags []domain.FeatureFlag
	err = json.Unmarshal(data, &flags)
	if err != nil {
		return err
	}

	for _, flag := range flags {
		p.store.Set(flag.Key, flag)
	}
	return nil
}
