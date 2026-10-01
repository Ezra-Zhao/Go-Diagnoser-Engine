// Package models defines the shared domain types for the diagnostics engine.
package models

import "time"

// JobStatus is the lifecycle state of a diagnostic job.
type JobStatus string

const (
	StatusQueued  JobStatus = "queued"
	StatusRunning JobStatus = "running"
	StatusDone    JobStatus = "done"
	StatusFailed  JobStatus = "failed"
)

// DiagnoseRequest is the payload accepted by POST /api/v1/diagnose.
type DiagnoseRequest struct {
	// Target is the node/host under diagnosis (hostname, IP, or node ID).
	Target string `json:"target"`
	// Checks optionally narrows which checks run; empty means all registered checks.
	Checks []string `json:"checks,omitempty"`
}

// CheckResult is the outcome of a single diagnostic check.
type CheckResult struct {
	Name      string `json:"name"`
	Passed    bool   `json:"passed"`
	Detail    string `json:"detail"`
	LatencyMs int64  `json:"latency_ms"`
}

// Job is a diagnostic job and its accumulated results.
type Job struct {
	ID        string        `json:"id"`
	Target    string        `json:"target"`
	Status    JobStatus     `json:"status"`
	CreatedAt time.Time     `json:"created_at"`
	Results   []CheckResult `json:"results,omitempty"`
	Error     string        `json:"error,omitempty"`
}
