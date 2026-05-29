package rules

import (
	"fmt"

	"github.com/Flgado/trino-insights-mcp/pkg/diagnose"
	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

// LongBlocked fires when blocked time >= 40% of scheduled time AND the
// blocked total is non-trivial in absolute terms (default >= 2,000 ms).
//
// The absolute floor exists because the ratio alone is noisy on sub-second
// queries: a query that spent 600 ms blocked out of 1,000 ms scheduled is
// 60% blocked but the absolute wait is negligible — flagging it is just
// noise that erodes trust in the rule engine.
type LongBlocked struct {
	Threshold    float64 // default 0.40 (40%)
	MinBlockedMs int64   // default 2000 — absolute floor; ignore tiny waits
}

func (r LongBlocked) ID() string { return "trino.long-blocked" }

func (r LongBlocked) threshold() float64 {
	if r.Threshold <= 0 {
		return 0.40
	}
	return r.Threshold
}

func (r LongBlocked) minBlockedMs() int64 {
	if r.MinBlockedMs <= 0 {
		return 2000
	}
	return r.MinBlockedMs
}

func (r LongBlocked) Eval(facts *queryinfo.QueryFacts) *diagnose.Finding {
	scheduled := facts.Time.TotalScheduledMs
	blocked := facts.Time.TotalBlockedMs

	if scheduled <= 0 || blocked <= 0 {
		return nil
	}

	if blocked < r.minBlockedMs() {
		return nil
	}

	ratio := float64(blocked) / float64(scheduled)
	if ratio < r.threshold() {
		return nil
	}

	return &diagnose.Finding{
		RuleID:   "trino.long-blocked",
		Severity: diagnose.SeverityWarn,
		Title:    "Query spent significant time blocked",
		Details:  fmt.Sprintf("Blocked %d ms out of %d ms scheduled (%.0f%%). The query is waiting on I/O, memory, or network rather than making progress.", blocked, scheduled, ratio*100),
		Evidence: map[string]any{
			"total_blocked_ms":   blocked,
			"total_scheduled_ms": scheduled,
			"ratio":              ratio,
		},
		Remediation: "Check blocked reasons. Common causes: waiting for memory (spill-to-disk in progress), network saturation between stages, or slow connector I/O.",
	}
}
