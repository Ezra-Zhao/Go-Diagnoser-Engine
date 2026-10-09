// Package engine runs diagnostic jobs on a bounded worker pool.
//
// Concurrency model: a fixed number of long-lived workers pull jobs from a
// buffered queue. Each job's checks fan out onto short-lived goroutines and
// are joined before the worker moves on. Shutdown is cooperative: cancel the
// context, stop accepting new jobs, drain in-flight work, then return.
package engine

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/ezra-zhao/go-diagnoser-engine/internal/metrics"
	"github.com/ezra-zhao/go-diagnoser-engine/internal/store"
	"github.com/ezra-zhao/go-diagnoser-engine/pkg/models"
)

// task is the unit of work a worker executes.
type task struct {
	jobID string
	req   models.DiagnoseRequest
}

// Engine owns the worker pool and the job queue.
type Engine struct {
	store   *store.Store
	queue   chan task
	workers int
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	// metrics is optional; AttachMetrics wires it. Nil-safe: the engine
	// runs fine without observability in tests.
	metrics *metrics.Registry
}

// Config tunes the engine.
type Config struct {
	// Workers is the pool size; <=0 defaults to runtime.NumCPU().
	Workers int
	// QueueSize bounds the number of jobs waiting for a worker; 0 disables buffering.
	QueueSize int
}

// New builds an Engine but does not start workers; call Start.
func New(st *store.Store, cfg Config) *Engine {
	if cfg.Workers <= 0 {
		cfg.Workers = runtime.NumCPU()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Engine{
		store:   st,
		queue:   make(chan task, cfg.QueueSize),
		workers: cfg.Workers,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// AttachMetrics wires the observability registry. Call before Start;
// safe to skip (engine is nil-safe without metrics).
func (e *Engine) AttachMetrics(m *metrics.Registry) {
	e.metrics = m
}

// QueueDepth reports jobs waiting for a worker; sampled by /metrics so
// the gauge is never stale.
func (e *Engine) QueueDepth() int {
	return len(e.queue)
}

// Start launches the worker goroutines.
func (e *Engine) Start() {
	for i := 0; i < e.workers; i++ {
		e.wg.Add(1)
		go e.worker(i)
	}
	log.Printf("engine: started %d workers", e.workers)
}

// Submit enqueues a job for execution. It returns an error if the engine is
// shutting down or the queue is full (fail fast instead of blocking the API).
func (e *Engine) Submit(jobID string, req models.DiagnoseRequest) error {
	select {
	case <-e.ctx.Done():
		return fmt.Errorf("engine shutting down")
	case e.queue <- task{jobID: jobID, req: req}:
		return nil
	default:
		return fmt.Errorf("job queue full, try again later")
	}
}

// Shutdown stops accepting jobs, waits for in-flight jobs with a timeout,
// and returns. No goroutines are leaked.
func (e *Engine) Shutdown(timeout time.Duration) {
	e.cancel()
	done := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		log.Print("engine: all workers stopped")
	case <-time.After(timeout):
		log.Print("engine: shutdown timed out waiting for workers")
	}
}

// worker is the long-lived goroutine body.
func (e *Engine) worker(id int) {
	defer e.wg.Done()
	for {
		select {
		case <-e.ctx.Done():
			return
		case t, ok := <-e.queue:
			if !ok {
				return
			}
			e.runJob(t)
		}
	}
}

// runJob executes one job: fan out its checks, join, persist results.
func (e *Engine) runJob(t task) {
	e.store.UpdateStatus(t.jobID, models.StatusRunning)

	checks := resolveChecks(t.req.Checks)
	results := make([]models.CheckResult, len(checks))

	var wg sync.WaitGroup
	for i, c := range checks {
		wg.Add(1)
		go func(i int, c Check) {
			defer wg.Done()
			// Each check gets its own timeout so one slow probe cannot
			// stall the whole job.
			ctx, cancel := context.WithTimeout(e.ctx, 30*time.Second)
			defer cancel()
			results[i] = c.Run(ctx, t.req.Target)
		}(i, c)
	}
	wg.Wait()

	// Record per-check telemetry before persisting.
	if e.metrics != nil {
		for _, r := range results {
			e.metrics.ObserveCheck(r.Name, r.LatencyMs, r.Passed)
		}
		e.metrics.IncCompleted(true)
	}
	e.store.SetResults(t.jobID, results)
}
