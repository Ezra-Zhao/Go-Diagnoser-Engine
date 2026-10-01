// Package store provides job persistence.
//
// Current implementation is in-memory and safe for concurrent use.
// TODO(ezra): swap the in-memory backend for PostgreSQL or Redis when the
// service needs durability across restarts. Keep the Store API unchanged so
// callers (api, engine) do not need to change.
package store

import (
	"fmt"
	"sync"

	"github.com/ezra-zhao/go-diagnoser-engine/pkg/models"
)

// Store is a concurrency-safe job repository.
type Store struct {
	mu   sync.RWMutex
	jobs map[string]*models.Job
}

// New returns an empty Store.
func New() *Store {
	return &Store{jobs: make(map[string]*models.Job)}
}

// Create inserts a new job; it returns an error if the ID already exists.
func (s *Store) Create(job *models.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[job.ID]; ok {
		return fmt.Errorf("job %s already exists", job.ID)
	}
	s.jobs[job.ID] = job
	return nil
}

// Get returns a copy of the job with the given ID, or false if absent.
// A copy is returned so callers cannot mutate stored state without Update.
func (s *Store) Get(id string) (*models.Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[id]
	if !ok {
		return nil, false
	}
	cp := *j
	cp.Results = append([]models.CheckResult(nil), j.Results...)
	return &cp, true
}

// UpdateStatus sets the job status.
func (s *Store) UpdateStatus(id string, st models.JobStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[id]; ok {
		j.Status = st
	}
}

// SetResults stores the final check results and marks the job done.
func (s *Store) SetResults(id string, results []models.CheckResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[id]; ok {
		j.Results = results
		j.Status = models.StatusDone
	}
}

// SetError records a failure and marks the job failed.
func (s *Store) SetError(id string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[id]; ok {
		j.Error = err.Error()
		j.Status = models.StatusFailed
	}
}
