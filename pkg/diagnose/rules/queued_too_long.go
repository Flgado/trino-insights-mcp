package rules

import (
	"fmt"

	"github.com/Flgado/trino-insights-mcp/pkg/diagnose"
	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

// QueuedTooLong fires when queued time is >= 30% of elapsed time.
type QueuedTooLong struct {
	Threshold float64 // default 0.30
}

func (r QueuedTooLong) ID() string { return "trino.queued-too-long" }

func (r QueuedTooLong) threshold() float64 {
	if r.Threshold <= 0 {
		return 0.30
	}
	return r.Threshold
}

func (r QueuedTooLong) Eval(facts *queryinfo.QueryFacts) *diagnose.Finding {
	elapsed := facts.Time.ElapsedMs
	queued := facts.Time.QueuedMs
	if elapsed <= 0 || queued <= 0 {
		return nil
	}

	ratio := float64(queued) / float64(elapsed)
	if ratio < r.threshold() {
		return nil
	}

	return &diagnose.Finding{
		RuleID:   "trino.queued-too-long",
		Severity: diagnose.SeverityWarn,
		Title:    "Query spent too long queued",
		Details:  fmt.Sprintf("Queued %d ms out of %d ms elapsed (%.0f%%). The cluster may be overloaded or resource group limits are too tight.", queued, elapsed, ratio*100),
		Evidence: map[string]any{
			"queued_ms":  queued,
			"elapsed_ms": elapsed,
			"ratio":      ratio,
		},
		Remediation: "Check resource group configuration, cluster load (cluster_health), and whether other heavy queries are consuming the queue slots.",
	}
}
