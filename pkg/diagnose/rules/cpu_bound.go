package rules

import (
	"fmt"

	"github.com/Flgado/trino-insights-mcp/pkg/diagnose"
	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

// CPUBound fires when the CPU/scheduled ratio is high, indicating the query
// is spending most of its scheduled time doing actual CPU work rather than
// waiting on I/O or memory.
type CPUBound struct {
	Threshold float64 // default 0.8
}

func (r CPUBound) ID() string { return "trino.cpu-bound" }

func (r CPUBound) threshold() float64 {
	if r.Threshold <= 0 {
		return 0.8
	}
	return r.Threshold
}

func (r CPUBound) Eval(facts *queryinfo.QueryFacts) *diagnose.Finding {
	scheduled := facts.Time.TotalScheduledMs
	if scheduled <= 0 {
		return nil
	}

	ratio := float64(facts.Time.TotalCPUMs) / float64(scheduled)
	if ratio < r.threshold() {
		return nil
	}

	return &diagnose.Finding{
		RuleID:   "trino.cpu-bound",
		Severity: diagnose.SeverityWarn,
		Title:    "Query is CPU-bound",
		Details:  fmt.Sprintf("CPU/scheduled ratio is %.1f%% (threshold %.0f%%). The query spends most of its time in CPU — look for expensive expressions, large hash joins, or skewed stages.", ratio*100, r.threshold()*100),
		Evidence: map[string]any{
			"total_cpu_ms":       facts.Time.TotalCPUMs,
			"total_scheduled_ms": facts.Time.TotalScheduledMs,
			"ratio":              ratio,
		},
		Remediation: "CPU-bound is usually a symptom, not the root cause. Check for stage skew, hot join keys, expensive UDFs/regexp, or window functions over huge frames. Call get_query_sql to see the SQL.",
	}
}
