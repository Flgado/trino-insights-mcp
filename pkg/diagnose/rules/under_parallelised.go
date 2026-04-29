package rules

import (
	"fmt"

	"github.com/Flgado/trino-insights-mcp/pkg/diagnose"
	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

// UnderParallelised fires when the query has very few drivers/splits relative
// to its elapsed time, suggesting it could benefit from more parallelism.
// Heuristic: elapsed > 30s AND total_drivers < 4.
type UnderParallelised struct {
	MinElapsedMs int64 // default 30_000
	MinDrivers   int   // default 4
}

func (r UnderParallelised) ID() string { return "trino.under-parallelised" }

func (r UnderParallelised) minElapsed() int64 {
	if r.MinElapsedMs <= 0 {
		return 30_000
	}
	return r.MinElapsedMs
}

func (r UnderParallelised) minDrivers() int {
	if r.MinDrivers <= 0 {
		return 4
	}
	return r.MinDrivers
}

func (r UnderParallelised) Eval(facts *queryinfo.QueryFacts) *diagnose.Finding {
	if facts.Time.ElapsedMs < r.minElapsed() {
		return nil
	}

	drivers := facts.Tasks.TotalDrivers
	if drivers >= r.minDrivers() {
		return nil
	}

	return &diagnose.Finding{
		RuleID:   "trino.under-parallelised",
		Severity: diagnose.SeverityInfo,
		Title:    "Under-parallelised query",
		Details:  fmt.Sprintf("Query ran for %d ms with only %d drivers. More parallelism could reduce wall-clock time.", facts.Time.ElapsedMs, drivers),
		Evidence: map[string]any{
			"elapsed_ms":    facts.Time.ElapsedMs,
			"total_drivers": facts.Tasks.TotalDrivers,
		},
		Remediation: "Check if the table has too few partitions/buckets, or if the connector limits split generation. Consider repartitioning the source data.",
	}
}
