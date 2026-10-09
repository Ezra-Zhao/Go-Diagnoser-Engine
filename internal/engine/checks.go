package engine

import (
	"context"
	"fmt"
	"hash/fnv"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ezra-zhao/go-diagnoser-engine/pkg/models"
)

// Check is a single diagnostic probe run against a target.
type Check struct {
	Name string
	Run  func(ctx context.Context, target string) models.CheckResult
}

// registry holds every check the engine can run, keyed by name.
var registry = map[string]Check{
	"connectivity": {
		Name: "connectivity",
		Run:  simulatedCheck("connectivity", "TCP handshake probe"),
	},
	"disk_usage": {
		Name: "disk_usage",
		Run:  simulatedCheck("disk_usage", "disk usage scrape"),
	},
	"service_health": {
		Name: "service_health",
		Run:  simulatedCheck("service_health", "/healthz query"),
	},
	// host_load is REAL: it reads the local machine's load average.
	// Safe to run anywhere (read-only /proc), and it gives the demo a
	// genuine signal alongside the simulated probes.
	"host_load": {
		Name: "host_load",
		Run:  hostLoadCheck,
	},
}

// resolveChecks maps requested names to Checks; unknown names are skipped.
// Empty input selects all registered checks, in registry order for
// deterministic output.
func resolveChecks(names []string) []Check {
	if len(names) == 0 {
		ordered := []string{"connectivity", "disk_usage", "service_health", "host_load"}
		out := make([]Check, 0, len(ordered))
		for _, n := range ordered {
			out = append(out, registry[n])
		}
		return out
	}
	var out []Check
	for _, n := range names {
		if c, ok := registry[n]; ok {
			out = append(out, c)
		}
	}
	return out
}

// seedFor derives a deterministic seed from target+check name.
//
// Design intent: the same target always yields the same simulated outcome,
// so a demo is reproducible and a retry of a failed job can be compared
// against the first run. Real nondeterminism belongs in real probes;
// simulated ones should be honest *and* stable.
func seedFor(target, name string) int64 {
	h := fnv.New64a()
	h.Write([]byte(target + "\x00" + name))
	return int64(h.Sum64())
}

// simulatedCheck builds a placeholder probe: waits a realistic amount of
// time, then returns a deterministic pass/fail derived from the target.
//
// TODO(ezra): replace bodies with real logic, e.g.
//   - connectivity -> TCP dial with timeout (net.Dialer)
//   - disk_usage   -> syscall.Statfs on the target mount
//   - service_health -> GET http://<target>/healthz
// Keep the Check signature so engine.go does not change.
func simulatedCheck(name, what string) func(context.Context, string) models.CheckResult {
	return func(ctx context.Context, target string) models.CheckResult {
		start := time.Now()
		rng := rand.New(rand.NewSource(seedFor(target, name)))
		// Deterministic but realistic latency: 50-200ms.
		delay := time.Duration(50+rng.Intn(150)) * time.Millisecond
		select {
		case <-ctx.Done():
			return models.CheckResult{
				Name:      name,
				Passed:    false,
				Detail:    "cancelled: " + ctx.Err().Error(),
				LatencyMs: time.Since(start).Milliseconds(),
			}
		case <-time.After(delay):
		}
		passed := rng.Intn(100) < 90 // SIMULATED outcome, not a real verdict
		return models.CheckResult{
			Name:      name,
			Passed:    passed,
			Detail:    fmt.Sprintf("SIMULATED %s for target=%s", what, target),
			LatencyMs: time.Since(start).Milliseconds(),
		}
	}
}

// hostLoadCheck is a REAL probe: 1-minute load average vs CPU count.
//
// Pass heuristic: load1 < NumCPU means the machine is keeping up; above
// that it is saturated. Conservative on purpose — a diagnostics tool
// should flag early, not explain away load.
func hostLoadCheck(ctx context.Context, target string) models.CheckResult {
	start := time.Now()
	load1, err := readLoadAvg()
	detail := func() string {
		if err != nil {
			return fmt.Sprintf("SIMULATED fallback (no /proc/loadavg: %v)", err)
		}
		return fmt.Sprintf("load1=%.2f cpus=%d", load1, runtime.NumCPU())
	}
	if err != nil {
		// Non-Linux (no /proc): deterministic simulated verdict.
		rng := rand.New(rand.NewSource(seedFor(target, "host_load")))
		passed := rng.Intn(100) < 95
		return models.CheckResult{
			Name: "host_load", Passed: passed, Detail: detail(),
			LatencyMs: time.Since(start).Milliseconds(),
		}
	}
	_ = ctx // local read is instant; no cancellation point needed
	passed := load1 < float64(runtime.NumCPU())
	return models.CheckResult{
		Name: "host_load", Passed: passed, Detail: detail(),
		LatencyMs: time.Since(start).Milliseconds(),
	}
}

// readLoadAvg parses /proc/loadavg (Linux only).
func readLoadAvg() (float64, error) {
	if runtime.GOOS != "linux" {
		return 0, fmt.Errorf("not linux")
	}
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0, fmt.Errorf("unexpected /proc/loadavg format")
	}
	return strconv.ParseFloat(fields[0], 64)
}
