package rules

import (
	"fmt"

	"github.com/Flgado/trino-insights-mcp/pkg/diagnose"
	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

// DiskSpill fires when the query has spilled data to disk and identifies
// which operator is responsible using per-operator spill data.
type DiskSpill struct{}

func (DiskSpill) ID() string { return "trino.disk-spill" }

func (DiskSpill) Eval(facts *queryinfo.QueryFacts) *diagnose.Finding {
	spilled := facts.Memory.SpilledBytes
	if spilled <= 0 {
		return nil
	}

	spilledMiB := float64(spilled) / (1024 * 1024)

	spillOp, spillStage := findSpillingOperator(facts)

	title := "Query spilled to disk"
	details := fmt.Sprintf("Spilled %.1f MiB to disk.", spilledMiB)
	evidence := map[string]any{
		"spilled_bytes": spilled,
		"spilled_mib":   spilledMiB,
	}

	if spillOp != "" {
		title = fmt.Sprintf("Query spilled to disk (%s)", spillOp)
		if spillStage != "" {
			label := fmt.Sprintf("stage %s", queryinfo.StageShortID(spillStage))
			details = fmt.Sprintf("%s in %s spilled %.1f MiB to disk.", spillOp, label, spilledMiB)
		} else {
			details = fmt.Sprintf("%s spilled %.1f MiB to disk.", spillOp, spilledMiB)
		}
		evidence["spill_operator"] = spillOp
		evidence["spill_stage_id"] = spillStage
	}

	return &diagnose.Finding{
		RuleID:      "trino.disk-spill",
		Severity:    diagnose.SeverityWarn,
		Title:       title,
		Details:     details,
		Evidence:    evidence,
		Remediation: "Reduce the build side of the join (filter early, pre-aggregate), increase memory limits, or repartition the data to reduce per-task cardinality.",
	}
}

// findSpillingOperator scans stage operators for the one with the most spill.
func findSpillingOperator(facts *queryinfo.QueryFacts) (operatorType, stageID string) {
	var maxSpill int64
	for _, s := range facts.Stages {
		for _, op := range s.Operators {
			if op.SpilledBytes > maxSpill {
				maxSpill = op.SpilledBytes
				operatorType = op.OperatorType
				stageID = s.StageID
			}
		}
	}
	return
}
