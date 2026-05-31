package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/HanshalDabbiru/feature-flag-engine/pkg/domain"
	"github.com/HanshalDabbiru/feature-flag-engine/pkg/hub"
	"github.com/HanshalDabbiru/feature-flag-engine/pkg/persistence"
	"github.com/HanshalDabbiru/feature-flag-engine/pkg/store"
)

// Handler holds the dependencies for the HTTP API endpoints.
type Handler struct {
	store       *store.Store
	persistence *persistence.Persistence
	hub         *hub.Hub
}

// New returns a Handler wired to the given store and persistence layer.
func New(s *store.Store, p *persistence.Persistence, h *hub.Hub) *Handler {
	return &Handler{store: s, persistence: p, hub: h}
}

// CreateFlag handles POST /flags.
func (h *Handler) CreateFlag(w http.ResponseWriter, r *http.Request) {
	var flag domain.FeatureFlag
	if err := json.NewDecoder(r.Body).Decode(&flag); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	h.store.Set(flag.Key, flag)
	if err := h.persistence.Flush(); err != nil {
		http.Error(w, "failed to save flag", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(flag)

	h.hub.Broadcast(flag)
}

// ListFlags handles GET /flags.
func (h *Handler) ListFlags(w http.ResponseWriter, r *http.Request) {
	flags := h.store.List()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(flags)
}

// GetFlag handles GET /flags/{key}.
func (h *Handler) GetFlag(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	flag := h.store.Get(key)
	if flag.Key == "" {
		http.Error(w, "flag not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(flag)
}

// DeleteFlag handles DELETE /flags/{key}, removing the flag from the store and
// flushing the change to disk. Responds 204 No Content on success.
func (h *Handler) DeleteFlag(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	flag := h.store.Get(key)
	h.store.Delete(key)
	if err := h.persistence.Flush(); err != nil {
		http.Error(w, "failed to save changes", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)

	h.hub.Broadcast(flag)
}

// ToggleFlag handles PUT /flags/{key}, flipping the Enabled field.
func (h *Handler) ToggleFlag(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	flag := h.store.Get(key)
	flag.Enabled = !flag.Enabled
	h.store.Set(key, flag)
	if err := h.persistence.Flush(); err != nil {
		http.Error(w, "failed to save flag", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(flag)

	h.hub.Broadcast(flag)
}

// Stream handles GET /stream, registering the client with the hub and writing
// flag updates as Server-Sent Events until the client disconnects.
func (h *Handler) Stream(w http.ResponseWriter, r *http.Request) {
	id, ch := h.hub.Register()
	defer h.hub.Unregister(id)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	for {
		select {
		case flag := <-ch:
			data, err := json.Marshal(flag)
			if err != nil {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			w.(http.Flusher).Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// RegisterRoutes registers all API routes on mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/flags", h.routeFlags)
	mux.HandleFunc("/flags/{key}", h.routeFlagByKey)
	mux.HandleFunc("/stream", h.Stream)
}

// routeFlags is the single mux entry point for /flags. Dispatching here keeps
// CreateFlag and ListFlags as pure handlers that can be called directly in tests.
func (h *Handler) routeFlags(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.CreateFlag(w, r)
	case http.MethodGet:
		h.ListFlags(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) routeFlagByKey(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.GetFlag(w, r)
	case http.MethodPut:
		h.ToggleFlag(w, r)
	case http.MethodDelete:
		h.DeleteFlag(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
