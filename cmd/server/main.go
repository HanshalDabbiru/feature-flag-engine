package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	srv := &http.Server{Addr: ":8080", Handler: mux}

	go func() {
		log.Println("server started on :8080")
		if err := srv.ListenAndServe(); err != nil {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	<-quit // wait until either SIGINT or SIGTERM come through the quit channel
	log.Println("server shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)

}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "ok")
}
