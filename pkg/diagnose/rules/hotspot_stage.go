package rules

import (
	"fmt"

	"github.com/Flgado/trino-insights-mcp/pkg/diagnose"
	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

// HotspotStage fires when a single stage carries >= 60% of the total query CPU.
type HotspotStage struct {
	Threshold float64 // default 0.60
}

func (r HotspotStage) ID() string { return "trino.hotspot-stage" }

func (r HotspotStage) threshold() float64 {
	if r.Threshold <= 0 {
		return 0.60
	}
	return r.Threshold
}

func (r HotspotStage) Eval(facts *queryinfo.QueryFacts) *diagnose.Finding {
	if len(facts.Stages) < 2 {
		return nil
	}

	var totalCPU int64
	var maxCPU int64
	var maxIdx int

	for i, s := range facts.Stages {
		totalCPU += s.TotalCPUMs
		if s.TotalCPUMs > maxCPU {
			maxCPU = s.TotalCPUMs
			maxIdx = i
		}
	}

	if totalCPU <= 0 {
		return nil
	}

	pct := float64(maxCPU) / float64(totalCPU)
	if pct < r.threshold() {
		return nil
	}

	hot := facts.Stages[maxIdx]
	label := queryinfo.FormatStageLabel(hot)
	pctInt := int(pct * 100)

	title := fmt.Sprintf("Hotspot %s dominates CPU", label)

	return &diagnose.Finding{
		RuleID:  "trino.hotspot-stage",
		Severity: diagnose.SeverityWarn,
		Title:   title,
		Details: fmt.Sprintf("%s carries %d%% of total query CPU (%d ms of %d ms).",
			capitalize(label), pctInt, maxCPU, totalCPU),
		Evidence: map[string]any{
			"hot_stage_id":     hot.StageID,
			"primary_operator": hot.PrimaryOperator,
			"hot_cpu_ms":       maxCPU,
			"total_cpu_ms":     totalCPU,
			"cpu_pct":          pctInt,
		},
		Remediation: "Focus optimization on this stage. Call get_query_sql and identify the SQL clause that maps to the dominant operator.",
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	b := []byte(s)
	if b[0] >= 'a' && b[0] <= 'z' {
		b[0] -= 'a' - 'A'
	}
	return string(b)
}
