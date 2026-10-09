// Package metrics exposes engine telemetry in Prometheus text exposition
// format, using only the standard library (no client_golang dependency).
//
// Design intent: a diagnostics service that cannot explain its own load
// is a black box. These metrics let an operator see queue backpressure,
// worker saturation, and per-check latency/failure rates on a Grafana
// dashboard — the same observability story told in a system-design
// interview ("how do you know your service is healthy?").
package metrics

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// Registry holds every metric the engine reports.
type Registry struct {
	jobsSubmitted  atomic.Uint64
	jobsCompleted  atomic.Uint64 // labeled by terminal status below
	jobsDone       atomic.Uint64
	jobsFailed     atomic.Uint64
	checksRun      atomic.Uint64
	checksPassed   atomic.Uint64
	checksFailed   atomic.Uint64
	checkLatencyMs atomic.Uint64 // cumulative, per check via ObserveCheck
	checkCount     atomic.Uint64

	mu         sync.Mutex
	byCheck    map[string]*checkStats // per-check latency + pass/fail
	queueDepth func() int             // sampled at scrape time (no stale gauge)
	workers    int
}

type checkStats struct {
	runs     uint64
	passed   uint64
	failed   uint64
	latency  uint64 // sum of ms
}

// New returns an empty Registry. queueDepth is called on every /metrics
// scrape so the gauge is never stale.
func New(queueDepth func() int, workers int) *Registry {
	return &Registry{
		byCheck:    make(map[string]*checkStats),
		queueDepth: queueDepth,
		workers:    workers,
	}
}

// IncSubmitted records an accepted diagnostic job.
func (r *Registry) IncSubmitted() { r.jobsSubmitted.Add(1) }

// IncCompleted records a finished job; ok=false counts it as failed.
func (r *Registry) IncCompleted(ok bool) {
	r.jobsCompleted.Add(1)
	if ok {
		r.jobsDone.Add(1)
	} else {
		r.jobsFailed.Add(1)
	}
}

// ObserveCheck records one finished probe: its latency and pass/fail.
func (r *Registry) ObserveCheck(name string, latencyMs int64, passed bool) {
	r.checksRun.Add(1)
	r.checkCount.Add(1)
	r.checkLatencyMs.Add(uint64(latencyMs))
	if passed {
		r.checksPassed.Add(1)
	} else {
		r.checksFailed.Add(1)
	}
	r.mu.Lock()
	s := r.byCheck[name]
	if s == nil {
		s = &checkStats{}
		r.byCheck[name] = s
	}
	s.runs++
	s.latency += uint64(latencyMs)
	if passed {
		s.passed++
	} else {
		s.failed++
	}
	r.mu.Unlock()
}

// Render writes the Prometheus text exposition format.
func (r *Registry) Render() string {
	var b strings.Builder
	counter := func(name, help string, v uint64) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s counter\n%s %d\n",
			name, help, name, name, v)
	}
	gauge := func(name, help string, v int) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s gauge\n%s %d\n",
			name, help, name, name, v)
	}
	counter("diagnoser_jobs_submitted_total",
		"Diagnostic jobs accepted by the API.", r.jobsSubmitted.Load())
	counter("diagnoser_jobs_completed_total",
		"Diagnostic jobs finished (any terminal state).", r.jobsCompleted.Load())
	counter("diagnoser_jobs_failed_total",
		"Diagnostic jobs that ended in error.", r.jobsFailed.Load())
	counter("diagnoser_checks_run_total",
		"Individual probes executed.", r.checksRun.Load())
	counter("diagnoser_checks_passed_total",
		"Probes that passed.", r.checksPassed.Load())
	counter("diagnoser_checks_failed_total",
		"Probes that failed.", r.checksFailed.Load())
	gauge("diagnoser_queue_depth",
		"Jobs currently waiting for a worker.", r.queueDepth())
	gauge("diagnoser_workers",
		"Worker pool size.", r.workers)

	// Per-check latency + outcome breakdown, sorted for stable output.
	r.mu.Lock()
	names := make([]string, 0, len(r.byCheck))
	for n := range r.byCheck {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		s := r.byCheck[n]
		avg := uint64(0)
		if s.runs > 0 {
			avg = s.latency / s.runs
		}
		fmt.Fprintf(&b,
			"# HELP diagnoser_check_latency_ms_avg Average probe latency in ms.\n"+
				"# TYPE diagnoser_check_latency_ms_avg gauge\n"+
				"diagnoser_check_latency_ms_avg{check=%q} %d\n"+
				"# HELP diagnoser_check_runs_total Probe runs by check and result.\n"+
				"# TYPE diagnoser_check_runs_total counter\n"+
				"diagnoser_check_runs_total{check=%q,result=\"passed\"} %d\n"+
				"diagnoser_check_runs_total{check=%q,result=\"failed\"} %d\n",
			n, avg, n, s.passed, n, s.failed)
	}
	r.mu.Unlock()
	return b.String()
}
