package rules

import (
	"fmt"

	"github.com/Flgado/trino-insights-mcp/pkg/diagnose"
	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

// MemoryPressure fires when peak per-task user memory is high.
// Default threshold: 1 GiB per task.
type MemoryPressure struct {
	ThresholdBytes int64 // default 1 GiB
}

func (r MemoryPressure) ID() string { return "trino.memory-pressure" }

func (r MemoryPressure) threshold() int64 {
	if r.ThresholdBytes <= 0 {
		return 1 << 30 // 1 GiB
	}
	return r.ThresholdBytes
}

func (r MemoryPressure) Eval(facts *queryinfo.QueryFacts) *diagnose.Finding {
	peak := facts.Memory.PeakTaskUserMemBytes
	if peak <= 0 || peak < r.threshold() {
		return nil
	}

	peakMiB := float64(peak) / (1024 * 1024)

	return &diagnose.Finding{
		RuleID:   "trino.memory-pressure",
		Severity: diagnose.SeverityWarn,
		Title:    "High per-task memory usage",
		Details:  fmt.Sprintf("Peak per-task user memory is %.0f MiB (threshold %.0f MiB). Tasks close to node limits risk OOM kills.", peakMiB, float64(r.threshold())/(1024*1024)),
		Evidence: map[string]any{
			"peak_task_user_mem_bytes": peak,
			"peak_task_user_mem_mib":   peakMiB,
		},
		Remediation: "Consider partitioning large aggregations, reducing join build-side cardinality, or increasing task memory limits if the cluster allows it.",
	}
}
