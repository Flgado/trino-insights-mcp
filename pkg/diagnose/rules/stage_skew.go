package rules

import (
	"fmt"

	"github.com/Flgado/trino-insights-mcp/pkg/diagnose"
	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

// StageSkew detects data skew by comparing per-task CPU within a stage.
//
// Primary mode: for each stage with >1 task, compare MaxTaskCPUMs / P50TaskCPUMs.
// Fallback mode: when all stages have <=1 task, compare stage-vs-stage CPU (max / avg).
type StageSkew struct {
	TaskSkewFactor  float64 // max/p50 threshold for per-task skew (default 3.0)
	StageSkewFactor float64 // max/avg threshold for stage-level fallback (default 5.0)
}

func (r StageSkew) ID() string { return "trino.stage-skew" }

func (r StageSkew) taskSkewFactor() float64 {
	if r.TaskSkewFactor <= 0 {
		return 3.0
	}
	return r.TaskSkewFactor
}

func (r StageSkew) stageSkewFactor() float64 {
	if r.StageSkewFactor <= 0 {
		return 5.0
	}
	return r.StageSkewFactor
}

func (r StageSkew) Eval(facts *queryinfo.QueryFacts) *diagnose.Finding {
	if len(facts.Stages) < 2 {
		return nil
	}

	// Primary: per-task skew within a stage
	if finding := r.evalTaskSkew(facts); finding != nil {
		return finding
	}

	// Fallback: stage-vs-stage when all stages have <=1 task
	return r.evalStageFallback(facts)
}

func (r StageSkew) evalTaskSkew(facts *queryinfo.QueryFacts) *diagnose.Finding {
	threshold := r.taskSkewFactor()
	var worstRatio float64
	var worstStage queryinfo.StageFact

	for _, s := range facts.Stages {
		if s.TaskCount <= 1 || s.P50TaskCPUMs <= 0 {
			continue
		}
		ratio := float64(s.MaxTaskCPUMs) / float64(s.P50TaskCPUMs)
		if ratio > worstRatio {
			worstRatio = ratio
			worstStage = s
		}
	}

	if worstRatio < threshold {
		return nil
	}

	label := queryinfo.FormatStageLabel(worstStage)

	return &diagnose.Finding{
		RuleID:   "trino.stage-skew",
		Severity: diagnose.SeverityWarn,
		Title:    fmt.Sprintf("Per-task skew in %s", label),
		Details: fmt.Sprintf("In %s the slowest task used %d ms CPU vs median %d ms (%.1fx skew across %d tasks). This typically means hot partition keys or uneven data distribution.",
			label, worstStage.MaxTaskCPUMs, worstStage.P50TaskCPUMs, worstRatio, worstStage.TaskCount),
		Evidence: map[string]any{
			"stage_id":         worstStage.StageID,
			"primary_operator": worstStage.PrimaryOperator,
			"task_count":       worstStage.TaskCount,
			"max_task_cpu_ms":  worstStage.MaxTaskCPUMs,
			"p50_task_cpu_ms":  worstStage.P50TaskCPUMs,
			"skew_factor":      worstRatio,
		},
		Remediation: "Examine the SQL for the join/group-by key used in this stage. Consider adding a salted key, pre-filtering, or redistributing the data.",
	}
}

func (r StageSkew) evalStageFallback(facts *queryinfo.QueryFacts) *diagnose.Finding {
	var maxCPU int64
	var maxIdx int
	var total int64
	for i, s := range facts.Stages {
		total += s.TotalCPUMs
		if s.TotalCPUMs > maxCPU {
			maxCPU = s.TotalCPUMs
			maxIdx = i
		}
	}

	avg := total / int64(len(facts.Stages))
	if avg <= 0 {
		return nil
	}

	ratio := float64(maxCPU) / float64(avg)
	if ratio < r.stageSkewFactor() {
		return nil
	}

	hot := facts.Stages[maxIdx]
	label := queryinfo.FormatStageLabel(hot)

	return &diagnose.Finding{
		RuleID:   "trino.stage-skew",
		Severity: diagnose.SeverityWarn,
		Title:    fmt.Sprintf("Stage-level skew in %s", label),
		Details: fmt.Sprintf("%s used %d ms CPU vs average %d ms (%.1fx skew). This typically means hot partition keys or uneven data distribution.",
			capitalize(label), maxCPU, avg, ratio),
		Evidence: map[string]any{
			"stage_id":         hot.StageID,
			"primary_operator": hot.PrimaryOperator,
			"hot_cpu_ms":       maxCPU,
			"avg_cpu_ms":       avg,
			"skew_factor":      ratio,
			"stage_count":      len(facts.Stages),
		},
		Remediation: "Examine the SQL for the join/group-by key used in the hot stage. Consider adding a salted key, pre-filtering, or redistributing the data.",
	}
}
