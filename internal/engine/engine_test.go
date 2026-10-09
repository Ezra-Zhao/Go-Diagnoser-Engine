package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/ezra-zhao/go-diagnoser-engine/internal/metrics"
	"github.com/ezra-zhao/go-diagnoser-engine/internal/store"
	"github.com/ezra-zhao/go-diagnoser-engine/pkg/models"
)

// waitFor polls the store until the job reaches a terminal state.
func waitFor(t *testing.T, st *store.Store, id string) *models.Job {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		job, ok := st.Get(id)
		if !ok {
			t.Fatalf("job %s vanished from store", id)
		}
		if job.Status == models.StatusDone || job.Status == models.StatusFailed {
			return job
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("job %s did not finish in time", id)
	return nil
}

func newTestEngine(workers, queue int) (*Engine, *store.Store, *metrics.Registry) {
	st := store.New()
	eng := New(st, Config{Workers: workers, QueueSize: queue})
	reg := metrics.New(eng.QueueDepth, workers)
	eng.AttachMetrics(reg)
	eng.Start()
	return eng, st, reg
}

func TestJobRunsEndToEnd(t *testing.T) {
	eng, st, _ := newTestEngine(2, 10)
	defer eng.Shutdown(5 * time.Second)

	job := &models.Job{ID: "test-job-1", Target: "node-42", Status: models.StatusQueued}
	if err := st.Create(job); err != nil {
		t.Fatal(err)
	}
	if err := eng.Submit(job.ID, models.DiagnoseRequest{Target: "node-42"}); err != nil {
		t.Fatal(err)
	}
	done := waitFor(t, st, job.ID)
	if len(done.Results) == 0 {
		t.Fatal("expected check results, got none")
	}
	for _, r := range done.Results {
		if r.Name == "" || r.LatencyMs < 0 {
			t.Errorf("malformed result: %+v", r)
		}
	}
}

func TestSelectedChecksOnly(t *testing.T) {
	eng, st, _ := newTestEngine(2, 10)
	defer eng.Shutdown(5 * time.Second)

	job := &models.Job{ID: "test-job-2", Target: "node-7", Status: models.StatusQueued}
	if err := st.Create(job); err != nil {
		t.Fatal(err)
	}
	req := models.DiagnoseRequest{Target: "node-7", Checks: []string{"host_load"}}
	if err := eng.Submit(job.ID, req); err != nil {
		t.Fatal(err)
	}
	done := waitFor(t, st, job.ID)
	if len(done.Results) != 1 || done.Results[0].Name != "host_load" {
		t.Fatalf("expected exactly the host_load check, got %+v", done.Results)
	}
}

func TestSimulatedChecksAreDeterministic(t *testing.T) {
	// Same target -> same outcome, so demos and retries are comparable.
	a := registry["connectivity"].Run(t.Context(), "node-99")
	b := registry["connectivity"].Run(t.Context(), "node-99")
	if a.Passed != b.Passed {
		t.Errorf("simulated check not deterministic: %v vs %v", a.Passed, b.Passed)
	}
}

func TestQueueFullFailsFast(t *testing.T) {
	// 1 worker, queue size 0: with the worker busy, Submit must fail fast
	// instead of blocking the API handler.
	eng, _, _ := newTestEngine(1, 0)
	defer eng.Shutdown(5 * time.Second)

	submitted := 0
	for i := 0; i < 20; i++ {
		err := eng.Submit("job-overflow", models.DiagnoseRequest{Target: "x"})
		if err != nil {
			return // fail-fast 503 path works
		}
		submitted++
	}
	t.Errorf("queue never filled after %d submits; backpressure broken?", submitted)
}

func TestMetricsRecorded(t *testing.T) {
	eng, st, reg := newTestEngine(2, 10)
	defer eng.Shutdown(5 * time.Second)

	job := &models.Job{ID: "test-job-3", Target: "node-1", Status: models.StatusQueued}
	if err := st.Create(job); err != nil {
		t.Fatal(err)
	}
	if err := eng.Submit(job.ID, models.DiagnoseRequest{Target: "node-1"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, st, job.ID)

	out := reg.Render()
	for _, want := range []string{
		"diagnoser_jobs_submitted_total",
		"diagnoser_checks_run_total",
		"diagnoser_queue_depth",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("metrics output missing %q", want)
		}
	}
}
