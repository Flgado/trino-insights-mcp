package rules

import (
	"fmt"

	"github.com/Flgado/trino-insights-mcp/pkg/diagnose"
	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

// RowExplosion fires when any non-infrastructure operator produces significantly
// more output rows than input rows, indicating a join fan-out, CROSS JOIN,
// or UNNEST that multiplies data volume.
type RowExplosion struct {
	Threshold    float64 // output/input ratio threshold (default 10.0)
	MinInputRows int64   // ignore tiny operators (default 10000)
}

func (r RowExplosion) ID() string { return "trino.row-explosion" }

func (r RowExplosion) threshold() float64 {
	if r.Threshold <= 0 {
		return 10.0
	}
	return r.Threshold
}

func (r RowExplosion) minInputRows() int64 {
	if r.MinInputRows <= 0 {
		return 10000
	}
	return r.MinInputRows
}

func (r RowExplosion) Eval(facts *queryinfo.QueryFacts) *diagnose.Finding {
	var worstAmp float64
	var worstOp queryinfo.OperatorFact
	var worstStage queryinfo.StageFact

	for _, s := range facts.Stages {
		for _, op := range s.Operators {
			if op.InputRows < r.minInputRows() {
				continue
			}
			if op.Amplification > worstAmp {
				worstAmp = op.Amplification
				worstOp = op
				worstStage = s
			}
		}
	}

	if worstAmp < r.threshold() {
		return nil
	}

	label := queryinfo.FormatStageLabel(worstStage)

	return &diagnose.Finding{
		RuleID:   "trino.row-explosion",
		Severity: diagnose.SeverityWarn,
		Title:    fmt.Sprintf("Row explosion in %s (%s)", label, worstOp.OperatorType),
		Details: fmt.Sprintf("%s in %s produced %.1fx more rows than it consumed (%d in -> %d out). This typically indicates a join fan-out, CROSS JOIN, or UNNEST that multiplies data volume.",
			worstOp.OperatorType, label, worstAmp, worstOp.InputRows, worstOp.OutputRows),
		Evidence: map[string]any{
			"stage_id":         worstStage.StageID,
			"primary_operator": worstStage.PrimaryOperator,
			"operator_type":    worstOp.OperatorType,
			"input_rows":       worstOp.InputRows,
			"output_rows":      worstOp.OutputRows,
			"amplification":    worstAmp,
		},
		Remediation: "Check the SQL for CROSS JOINs, non-equi joins with many-to-many relationships, or UNNEST calls on large arrays. Add filters before the join or consider pre-aggregating one side.",
	}
}
