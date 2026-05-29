package rules

import (
	"fmt"
	"strings"

	"github.com/Flgado/trino-insights-mcp/pkg/diagnose"
	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

// PoorSelectivity fires when output_rows / processed_rows < 0.0001 (0.01%),
// meaning the query processes a huge amount of data but emits very few rows.
//
// The rule deliberately does NOT fire on summary aggregations (COUNT, SUM,
// AVG, GROUP BY producing < AggregationOutputFloor groups) — those are *meant*
// to reduce billions of rows to a handful of output rows; flagging them as
// poor selectivity is noise that drowns out real signals.
type PoorSelectivity struct {
	Threshold              float64 // default 0.0001 (0.01%)
	AggregationOutputFloor int     // default 100 — when an aggregation operator emits <= this many rows, treat the query as a summary and suppress the finding
}

func (r PoorSelectivity) ID() string { return "trino.poor-selectivity" }

func (r PoorSelectivity) threshold() float64 {
	if r.Threshold <= 0 {
		return 0.0001
	}
	return r.Threshold
}

func (r PoorSelectivity) aggregationOutputFloor() int {
	if r.AggregationOutputFloor <= 0 {
		return 100
	}
	return r.AggregationOutputFloor
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

	if r.isSummaryAggregation(facts) {
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

// isSummaryAggregation returns true when the plan contains an aggregation
// operator whose output is small (<= AggregationOutputFloor rows). In that
// case the low selectivity is the *purpose* of the query, not a problem.
//
// We check stage operators (which carry input_rows/output_rows) rather than
// the SQL text because the SQL is not part of QueryFacts. Trino emits
// AggregationOperator for hash aggregation, StreamingAggregationOperator for
// pre-sorted aggregation, and HashAggregationOperator on older versions —
// matching by suffix covers all three without false positives.
func (r PoorSelectivity) isSummaryAggregation(facts *queryinfo.QueryFacts) bool {
	floor := int64(r.aggregationOutputFloor())
	for _, st := range facts.Stages {
		for _, op := range st.Operators {
			if !isAggregationOperator(op.OperatorType) {
				continue
			}
			if op.OutputRows > 0 && op.OutputRows <= floor {
				return true
			}
		}
	}
	return false
}

func isAggregationOperator(name string) bool {
	return strings.HasSuffix(name, "AggregationOperator")
}
