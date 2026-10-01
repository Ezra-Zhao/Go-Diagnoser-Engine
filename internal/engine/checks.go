package engine

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/ezra-zhao/go-diagnoser-engine/pkg/models"
)

// Check is a single diagnostic probe run against a target.
//
// IMPORTANT: the checks registered below are SIMULATED placeholders. They
// produce plausible-looking data so the API -> worker pool -> results path
// can be exercised end to end. They perform NO real probing.
//
// TODO(ezra): implement the real diagnostic rules here. Suggested mapping:
//   - CheckConnectivity -> real ICMP/TCP probe (or SSH to the node)
//   - CheckDiskUsage    -> parse `df` / node exporter metrics
//   - CheckServiceHealth-> query the service's /healthz endpoint
//   - (new) CheckPCIeTriage -> PCIe/XID error classification rules; this is
//     the domain logic that makes this project a real portfolio piece.
type Check struct {
	Name string
	Run  func(ctx context.Context, target string) models.CheckResult
}

// registry holds every check the engine can run, keyed by name.
var registry = map[string]Check{
	"connectivity": {
		Name: "connectivity",
		Run:  simulatedCheck("connectivity", "SIMULATED: ICMP/TCP probe not implemented"),
	},
	"disk_usage": {
		Name: "disk_usage",
		Run:  simulatedCheck("disk_usage", "SIMULATED: disk usage scrape not implemented"),
	},
	"service_health": {
		Name: "service_health",
		Run:  simulatedCheck("service_health", "SIMULATED: /healthz query not implemented"),
	},
}

// resolveChecks maps requested names to Checks; unknown names are skipped
// with a note in the job detail. Empty input selects all registered checks.
func resolveChecks(names []string) []Check {
	if len(names) == 0 {
		out := make([]Check, 0, len(registry))
		for _, c := range registry {
			out = append(out, c)
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

// simulatedCheck builds a placeholder probe: waits a realistic amount of
// time, then returns a random pass/fail. Replace the body with real logic;
// keep the signature so engine.go does not change.
func simulatedCheck(name, detail string) func(context.Context, string) models.CheckResult {
	return func(ctx context.Context, target string) models.CheckResult {
		start := time.Now()
		// Simulate probe latency without blocking shutdown.
		select {
		case <-ctx.Done():
			return models.CheckResult{
				Name:      name,
				Passed:    false,
				Detail:    "cancelled: " + ctx.Err().Error(),
				LatencyMs: time.Since(start).Milliseconds(),
			}
		case <-time.After(time.Duration(50+rand.Intn(150)) * time.Millisecond):
		}
		passed := rand.Intn(100) < 90 // SIMULATED outcome, not a real verdict
		return models.CheckResult{
			Name:      name,
			Passed:    passed,
			Detail:    fmt.Sprintf("%s (target=%s)", detail, target),
			LatencyMs: time.Since(start).Milliseconds(),
		}
	}
}
