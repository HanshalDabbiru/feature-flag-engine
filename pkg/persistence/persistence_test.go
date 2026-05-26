package persistence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/HanshalDabbiru/feature-flag-engine/pkg/domain"
	"github.com/HanshalDabbiru/feature-flag-engine/pkg/store"
)

func makeFlag(key string, enabled bool) domain.FeatureFlag {
	return domain.FeatureFlag{
		Key:          key,
		Description:  "test flag",
		Enabled:      enabled,
		DefaultValue: false,
	}
}

func makeFlagWithRules(key string) domain.FeatureFlag {
	return domain.FeatureFlag{
		Key:         key,
		Description: "flag with rules",
		Enabled:     true,
		Rules: []domain.Rule{
			{
				Name: "us-pro-users",
				Predicates: []domain.Predicate{
					{Attribute: "country", Operator: domain.EQUALS, Values: []string{"US"}},
					{Attribute: "plan", Operator: domain.STARTS_WITH, Values: []string{"pro"}},
				},
				Value: true,
			},
		},
		DefaultValue: false,
	}
}

// TestFlush_WritesFlags verifies that Flush serialises the store's flags to disk.
func TestFlush_WritesFlags(t *testing.T) {
	s := store.New()
	flag := makeFlag("flag-a", true)
	s.Set("flag-a", flag)

	p := New(filepath.Join(t.TempDir(), "flags.json"), s)
	if err := p.Flush(); err != nil {
		t.Fatalf("Flush returned unexpected error: %v", err)
	}

	data, err := os.ReadFile(p.path)
	if err != nil {
		t.Fatalf("could not read flushed file: %v", err)
	}

	var got []domain.FeatureFlag
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("flushed file is not valid JSON: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 flag, got %d", len(got))
	}
	if got[0].Key != "flag-a" {
		t.Errorf("expected key %q, got %q", "flag-a", got[0].Key)
	}
	if got[0].Enabled != true {
		t.Errorf("expected Enabled=true, got %v", got[0].Enabled)
	}
}

// TestFlush_EmptyStore verifies that Flush writes an empty JSON array when the
// store contains no flags.
func TestFlush_EmptyStore(t *testing.T) {
	s := store.New()
	p := New(filepath.Join(t.TempDir(), "flags.json"), s)

	if err := p.Flush(); err != nil {
		t.Fatalf("Flush returned unexpected error: %v", err)
	}

	data, err := os.ReadFile(p.path)
	if err != nil {
		t.Fatalf("could not read flushed file: %v", err)
	}

	var got []domain.FeatureFlag
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("flushed file is not valid JSON: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d items", len(got))
	}
}

// TestFlush_InvalidPath verifies that Flush returns an error when the target
// path is not writable.
func TestFlush_InvalidPath(t *testing.T) {
	s := store.New()
	s.Set("x", makeFlag("x", true))

	p := New("/no/such/directory/flags.json", s)
	if err := p.Flush(); err == nil {
		t.Error("expected error for unwritable path, got nil")
	}
}

// TestFlush_PreservesRulesAndPredicates verifies that nested Rule and Predicate
// data survives a Flush.
func TestFlush_PreservesRulesAndPredicates(t *testing.T) {
	s := store.New()
	flag := makeFlagWithRules("complex-flag")
	s.Set("complex-flag", flag)

	p := New(filepath.Join(t.TempDir(), "flags.json"), s)
	if err := p.Flush(); err != nil {
		t.Fatalf("Flush returned unexpected error: %v", err)
	}

	data, err := os.ReadFile(p.path)
	if err != nil {
		t.Fatalf("could not read flushed file: %v", err)
	}

	var got []domain.FeatureFlag
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("flushed file is not valid JSON: %v", err)
	}

	if len(got) != 1 || len(got[0].Rules) != 1 {
		t.Fatalf("expected 1 flag with 1 rule, got %+v", got)
	}
	rule := got[0].Rules[0]
	if rule.Name != "us-pro-users" {
		t.Errorf("expected rule name %q, got %q", "us-pro-users", rule.Name)
	}
	if len(rule.Predicates) != 2 {
		t.Errorf("expected 2 predicates, got %d", len(rule.Predicates))
	}
}

// TestLoad_PopulatesStore verifies that Load reads flags from disk and inserts
// them into the store.
func TestLoad_PopulatesStore(t *testing.T) {
	flags := []domain.FeatureFlag{
		makeFlag("alpha", true),
		makeFlag("beta", false),
	}
	data, _ := json.Marshal(flags)

	path := filepath.Join(t.TempDir(), "flags.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("could not write fixture: %v", err)
	}

	s := store.New()
	p := New(path, s)
	if err := p.Load(); err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}

	for _, flag := range flags {
		got := s.Get(flag.Key)
		if got.Key != flag.Key {
			t.Errorf("expected key %q in store, got %q", flag.Key, got.Key)
		}
		if got.Enabled != flag.Enabled {
			t.Errorf("key %q: expected Enabled=%v, got %v", flag.Key, flag.Enabled, got.Enabled)
		}
	}
}

// TestLoad_FileNotExist verifies that Load treats a missing file as an empty
// dataset and returns nil.
func TestLoad_FileNotExist(t *testing.T) {
	s := store.New()
	p := New(filepath.Join(t.TempDir(), "missing.json"), s)

	if err := p.Load(); err != nil {
		t.Errorf("expected nil error for missing file, got %v", err)
	}

	if list := s.List(); len(list) != 0 {
		t.Errorf("expected empty store after loading missing file, got %d flags", len(list))
	}
}

// TestLoad_InvalidJSON verifies that Load returns an error when the file
// contains malformed JSON.
func TestLoad_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flags.json")
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatalf("could not write fixture: %v", err)
	}

	s := store.New()
	p := New(path, s)
	if err := p.Load(); err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

// TestLoad_EmptyArray verifies that Load handles an empty JSON array without
// error and leaves the store empty.
func TestLoad_EmptyArray(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flags.json")
	if err := os.WriteFile(path, []byte("[]"), 0644); err != nil {
		t.Fatalf("could not write fixture: %v", err)
	}

	s := store.New()
	p := New(path, s)
	if err := p.Load(); err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}

	if list := s.List(); len(list) != 0 {
		t.Errorf("expected empty store, got %d flags", len(list))
	}
}

// TestLoad_PreservesRulesAndPredicates verifies that nested Rule and Predicate
// data is correctly deserialised into the store.
func TestLoad_PreservesRulesAndPredicates(t *testing.T) {
	flags := []domain.FeatureFlag{makeFlagWithRules("complex-flag")}
	data, _ := json.Marshal(flags)

	path := filepath.Join(t.TempDir(), "flags.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("could not write fixture: %v", err)
	}

	s := store.New()
	p := New(path, s)
	if err := p.Load(); err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}

	got := s.Get("complex-flag")
	if len(got.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(got.Rules))
	}
	if len(got.Rules[0].Predicates) != 2 {
		t.Errorf("expected 2 predicates, got %d", len(got.Rules[0].Predicates))
	}
	if got.Rules[0].Predicates[0].Operator != domain.EQUALS {
		t.Errorf("expected operator EQUALS, got %q", got.Rules[0].Predicates[0].Operator)
	}
}

// TestFlushThenLoad verifies that a round-trip through Flush and Load produces
// an identical set of flags in the store.
func TestFlushThenLoad(t *testing.T) {
	src := store.New()
	src.Set("alpha", makeFlag("alpha", true))
	src.Set("beta", makeFlag("beta", false))
	src.Set("gamma", makeFlagWithRules("gamma"))

	path := filepath.Join(t.TempDir(), "flags.json")
	if err := New(path, src).Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	dst := store.New()
	if err := New(path, dst).Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	for _, orig := range src.List() {
		got := dst.Get(orig.Key)
		if got.Key == "" {
			t.Errorf("key %q missing after round-trip", orig.Key)
			continue
		}
		if got.Enabled != orig.Enabled {
			t.Errorf("key %q: Enabled mismatch: want %v, got %v", orig.Key, orig.Enabled, got.Enabled)
		}
		if len(got.Rules) != len(orig.Rules) {
			t.Errorf("key %q: Rules length mismatch: want %d, got %d", orig.Key, len(orig.Rules), len(got.Rules))
		}
	}
}
