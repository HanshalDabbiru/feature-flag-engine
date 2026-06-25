package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HanshalDabbiru/feature-flag-engine/pkg/domain"
	"github.com/HanshalDabbiru/feature-flag-engine/pkg/hub"
	"github.com/HanshalDabbiru/feature-flag-engine/pkg/persistence"
	"github.com/HanshalDabbiru/feature-flag-engine/pkg/store"
)

// newHandler returns a Handler and its underlying store for test setup.
// Persistence is backed by a temp file so Flush has no side effects.
func newHandler(t *testing.T) (*Handler, *store.Store) {
	s := store.New()
	p := persistence.New(filepath.Join(t.TempDir(), "flags.json"), s)
	return New(s, p, hub.New()), s
}

func TestCreateFlag(t *testing.T) {
	h, s := newHandler(t)
	body := `{"Key":"feature-x","Description":"test flag","Enabled":true,"DefaultValue":false}`
	r := httptest.NewRequest(http.MethodPost, "/flags", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.CreateFlag(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	var got domain.FeatureFlag
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("could not decode response body: %v", err)
	}
	if got.Key != "feature-x" {
		t.Errorf("expected key %q, got %q", "feature-x", got.Key)
	}
	if !got.Enabled {
		t.Errorf("expected Enabled=true, got false")
	}
	if stored := s.Get("feature-x"); stored.Key != "feature-x" {
		t.Error("flag not found in store after create")
	}
}

func TestCreateFlag_InvalidJSON(t *testing.T) {
	h, _ := newHandler(t)
	r := httptest.NewRequest(http.MethodPost, "/flags", strings.NewReader("not json"))
	w := httptest.NewRecorder()

	h.CreateFlag(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestListFlags_Empty(t *testing.T) {
	h, _ := newHandler(t)
	r := httptest.NewRequest(http.MethodGet, "/flags", nil)
	w := httptest.NewRecorder()

	h.ListFlags(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got []domain.FeatureFlag
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("could not decode response body: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty array, got %d items", len(got))
	}
}

func TestListFlags(t *testing.T) {
	h, s := newHandler(t)
	s.Set("alpha", domain.FeatureFlag{Key: "alpha", Enabled: true})
	s.Set("beta", domain.FeatureFlag{Key: "beta", Enabled: false})

	r := httptest.NewRequest(http.MethodGet, "/flags", nil)
	w := httptest.NewRecorder()

	h.ListFlags(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got []domain.FeatureFlag
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("could not decode response body: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 flags, got %d", len(got))
	}
	// map iteration order is non-deterministic, so check by key
	byKey := make(map[string]domain.FeatureFlag, len(got))
	for _, f := range got {
		byKey[f.Key] = f
	}
	if _, ok := byKey["alpha"]; !ok {
		t.Error("expected key \"alpha\" in response")
	}
	if _, ok := byKey["beta"]; !ok {
		t.Error("expected key \"beta\" in response")
	}
}

func TestGetFlag(t *testing.T) {
	h, s := newHandler(t)
	s.Set("my-flag", domain.FeatureFlag{Key: "my-flag", Enabled: true})

	r := httptest.NewRequest(http.MethodGet, "/flags/my-flag", nil)
	r.SetPathValue("key", "my-flag")
	w := httptest.NewRecorder()

	h.GetFlag(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got domain.FeatureFlag
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("could not decode response body: %v", err)
	}
	if got.Key != "my-flag" {
		t.Errorf("expected key %q, got %q", "my-flag", got.Key)
	}
}

func TestGetFlag_NotFound(t *testing.T) {
	h, _ := newHandler(t)
	r := httptest.NewRequest(http.MethodGet, "/flags/ghost", nil)
	r.SetPathValue("key", "ghost")
	w := httptest.NewRecorder()

	h.GetFlag(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestToggleFlag(t *testing.T) {
	h, s := newHandler(t)
	s.Set("toggle-me", domain.FeatureFlag{Key: "toggle-me", Enabled: false})

	r := httptest.NewRequest(http.MethodPut, "/flags/toggle-me", nil)
	r.SetPathValue("key", "toggle-me")
	w := httptest.NewRecorder()

	h.ToggleFlag(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got domain.FeatureFlag
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("could not decode response body: %v", err)
	}
	if !got.Enabled {
		t.Errorf("expected Enabled=true after toggle from false, got false")
	}
	// verify the store was also updated
	if stored := s.Get("toggle-me"); !stored.Enabled {
		t.Error("store not updated after toggle")
	}
}

func TestDeleteFlag(t *testing.T) {
	h, s := newHandler(t)
	s.Set("bye-flag", domain.FeatureFlag{Key: "bye-flag", Enabled: true})

	r := httptest.NewRequest(http.MethodDelete, "/flags/bye-flag", nil)
	r.SetPathValue("key", "bye-flag")
	w := httptest.NewRecorder()

	h.DeleteFlag(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
	if stored := s.Get("bye-flag"); stored.Key != "" {
		t.Error("flag still present in store after delete")
	}
}

func TestDeleteFlag_NotFound(t *testing.T) {
	h, _ := newHandler(t)
	r := httptest.NewRequest(http.MethodDelete, "/flags/ghost", nil)
	r.SetPathValue("key", "ghost")
	w := httptest.NewRecorder()

	h.DeleteFlag(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing key, got %d", w.Code)
	}
}
