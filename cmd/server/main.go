package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/HanshalDabbiru/feature-flag-engine/pkg/api"
	"github.com/HanshalDabbiru/feature-flag-engine/pkg/hub"
	"github.com/HanshalDabbiru/feature-flag-engine/pkg/persistence"
	"github.com/HanshalDabbiru/feature-flag-engine/pkg/store"
)

func main() {
	s := store.New()
	p := persistence.New("flags.json", s)
	h := hub.New()
	err := p.Load()
	if err != nil {
		log.Fatalf("failed to load flags: %v", err)
	}

	handler := api.New(s, p, h)
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	handler.RegisterRoutes(mux)
	log.Println("server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "ok")
}
