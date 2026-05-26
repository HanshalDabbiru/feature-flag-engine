package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/HanshalDabbiru/feature-flag-engine/pkg/persistence"
	"github.com/HanshalDabbiru/feature-flag-engine/pkg/store"
)

func main() {
	s := store.New()
	p := persistence.New("flags.json", s)
	err := p.Load()
	if err != nil {
		log.Fatalf("failed to load flags: %v", err)
	}

	http.HandleFunc("/health", healthHandler)
	log.Println("server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "ok")
}
