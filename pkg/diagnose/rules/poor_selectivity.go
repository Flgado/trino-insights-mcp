package rules

import (
	"fmt"

	"github.com/Flgado/trino-insights-mcp/pkg/diagnose"
	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

// PoorSelectivity fires when output_rows / processed_rows < 0.0001 (0.01%),
// meaning the query processes a huge amount of data but emits very few rows.
type PoorSelectivity struct {
	Threshold float64 // default 0.0001
}

func (r PoorSelectivity) ID() string { return "trino.poor-selectivity" }

func (r PoorSelectivity) threshold() float64 {
	if r.Threshold <= 0 {
		return 0.0001
	}
	return r.Threshold
}

func (r PoorSelectivity) Eval(facts *queryinfo.QueryFacts) *diagnose.Finding {
	processed := facts.IO.ProcessedInputPos
	output := facts.IO.OutputPositions

	if processed <= 1000 {
		return nil
	}

	ratio := float64(output) / float64(processed)
	if ratio >= r.threshold() {
		return nil
	}

	return &diagnose.Finding{
		RuleID:   "trino.poor-selectivity",
		Severity: diagnose.SeverityInfo,
		Title:    "Poor selectivity",
		Details:  fmt.Sprintf("Output %d rows from %d processed (selectivity %.6f%%). The query reads vastly more data than it returns.", output, processed, ratio*100),
		Evidence: map[string]any{
			"output_rows":    output,
			"processed_rows": processed,
			"selectivity":    ratio,
		},
		Remediation: "Push filters closer to the scan (WHERE on partition columns), use materialized views, or pre-filter in a staging table.",
	}
}
