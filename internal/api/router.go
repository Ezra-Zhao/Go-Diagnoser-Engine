// Package api exposes the diagnostics engine over HTTP (REST).
package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/ezra-zhao/go-diagnoser-engine/internal/engine"
	"github.com/ezra-zhao/go-diagnoser-engine/internal/metrics"
	"github.com/ezra-zhao/go-diagnoser-engine/internal/store"
	"github.com/ezra-zhao/go-diagnoser-engine/pkg/models"
)

// maxBody caps request bodies to keep oversized payloads from tying up handlers.
const maxBody = 1 << 20 // 1 MiB

// Server wires the HTTP routes to the engine and the store.
type Server struct {
	engine  *engine.Engine
	store   *store.Store
	metrics *metrics.Registry
}

// NewServer returns an http.Handler with all routes registered.
func NewServer(eng *engine.Engine, st *store.Store, reg *metrics.Registry) http.Handler {
	s := &Server{engine: eng, store: st, metrics: reg}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /metrics", s.metricsHandler)
	mux.HandleFunc("POST /api/v1/diagnose", s.createJob)
	mux.HandleFunc("GET /api/v1/jobs/{id}", s.getJob)
	return mux
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// metricsHandler exposes engine telemetry in Prometheus text format.
func (s *Server) metricsHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(s.metrics.Render()))
}

// createJob accepts a diagnostic request, enqueues it, and returns 202 with
// the job ID. The client polls GET /api/v1/jobs/{id} for the result.
func (s *Server) createJob(w http.ResponseWriter, r *http.Request) {
	var req models.DiagnoseRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.Target == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target is required"})
		return
	}

	job := &models.Job{
		ID:        newJobID(),
		Target:    req.Target,
		Status:    models.StatusQueued,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.store.Create(job); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create job"})
		return
	}
	if err := s.engine.Submit(job.ID, req); err != nil {
		s.store.SetError(job.ID, err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	s.metrics.IncSubmitted()
	writeJSON(w, http.StatusAccepted, map[string]any{
		"id":     job.ID,
		"status": models.StatusQueued,
	})
}

// getJob returns the current state (and results, when done) of one job.
func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, ok := s.store.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // encoding to the wire cannot fail meaningfully here
}

// newJobID returns a random 128-bit hex ID using only the standard library.
func newJobID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is effectively impossible on supported
		// platforms; fall back to a timestamp-based ID rather than crashing.
		return "job-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(b[:])
}
