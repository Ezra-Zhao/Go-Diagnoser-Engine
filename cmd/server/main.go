// Command server is the entrypoint for the diagnostics engine API.
//
// Configuration (all optional, via environment):
//
//	PORT       - HTTP listen port (default "8080")
//	WORKERS    - engine worker pool size (default runtime.NumCPU())
//	QUEUE_SIZE - max queued jobs before the API returns 503 (default 100)
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/ezra-zhao/go-diagnoser-engine/internal/api"
	"github.com/ezra-zhao/go-diagnoser-engine/internal/engine"
	"github.com/ezra-zhao/go-diagnoser-engine/internal/store"
)

func main() {
	port := envOr("PORT", "8080")
	workers := envIntOr("WORKERS", 0) // 0 -> engine defaults to NumCPU
	queueSize := envIntOr("QUEUE_SIZE", 100)

	st := store.New()
	eng := engine.New(st, engine.Config{Workers: workers, QueueSize: queueSize})
	eng.Start()

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      api.NewServer(eng, st),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Run the HTTP server in the background; shut down gracefully on signal.
	go func() {
		log.Printf("listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Print("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
	eng.Shutdown(15 * time.Second) // drain in-flight jobs, leak no goroutines
	log.Print("bye")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envIntOr(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
