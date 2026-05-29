package rules

import (
	"fmt"
	"sort"

	"github.com/Flgado/trino-insights-mcp/pkg/diagnose"
	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

// SlowEmptyScan fires when a scan returned ZERO rows AND zero physical input,
// but its stage still spent meaningful wall-clock waiting on the connector.
//
// This catches the "MongoDB find() with un-indexed predicate" pattern: the
// source matched nothing but still took 2-3 seconds to confirm it. Different
// from trino.local-filter-dominates — there the source returned rows that
// Trino dropped; here the source itself returned an empty cursor while
// taking too long to do so.
//
// Severity is Info: the query worked, the result was correct, but a missing
// source-side index or a full-collection scan on the source is silently
// costing wall-clock.
type SlowEmptyScan struct {
	MinWaitMs          int64 // default 500 — ignore sub-half-second scans
	MaxScansInEvidence int   // default 5
}

func (r SlowEmptyScan) ID() string { return "trino.slow-empty-scan" }

func (r SlowEmptyScan) minWaitMs() int64 {
	if r.MinWaitMs <= 0 {
		return 500
	}
	return r.MinWaitMs
}

func (r SlowEmptyScan) maxEvidence() int {
	if r.MaxScansInEvidence <= 0 {
		return 5
	}
	return r.MaxScansInEvidence
}

type slowEmptyScanHit struct {
	scan      queryinfo.ScanPushdownFact
	stageWait int64
}

func (r SlowEmptyScan) Eval(facts *queryinfo.QueryFacts) *diagnose.Finding {
	if facts == nil || len(facts.ScanPushdown) == 0 {
		return nil
	}

	stageWaits := buildStageWaitIndex(facts.Stages)

	hits := r.collectHits(facts.ScanPushdown, stageWaits)
	if len(hits) == 0 {
		return nil
	}

	sort.Slice(hits, func(i, j int) bool {
		return hits[i].stageWait > hits[j].stageWait
	})

	return buildSlowEmptyScanFinding(r.ID(), hits, r.maxEvidence())
}

func (r SlowEmptyScan) collectHits(scans []queryinfo.ScanPushdownFact, stageWaits map[string]int64) []slowEmptyScanHit {
	min := r.minWaitMs()
	var hits []slowEmptyScanHit
	for _, s := range scans {
		if s.OutputRows != 0 || s.PhysicalInputPositions != 0 {
			continue
		}
		wait := stageWaits[s.StageID]
		if wait < min {
			continue
		}
		hits = append(hits, slowEmptyScanHit{scan: s, stageWait: wait})
	}
	return hits
}

// buildStageWaitIndex maps stage_id -> best-effort wall-clock I/O wait,
// preferring the derived IOWaitMs and falling back to scheduled - cpu when
// the derived value is missing.
func buildStageWaitIndex(stages []queryinfo.StageFact) map[string]int64 {
	idx := make(map[string]int64, len(stages))
	for _, st := range stages {
		wait := st.IOWaitMs
		if wait <= 0 {
			wait = st.TotalScheduledMs - st.TotalCPUMs
			if wait < 0 {
				wait = 0
			}
		}
		idx[st.StageID] = wait
	}
	return idx
}

func buildSlowEmptyScanFinding(ruleID string, hits []slowEmptyScanHit, maxEvidence int) *diagnose.Finding {
	top := hits[0]
	tableLabel := scanLabel(top.scan)

	title := fmt.Sprintf("Empty scan took %d ms on %s", top.stageWait, tableLabel)

	connector := top.scan.ConnectorType
	if connector == "" {
		connector = "unknown"
	}

	details := fmt.Sprintf(
		"Scan on %s (connector %s, stage %s) returned 0 rows from 0 physical input "+
			"but the stage still spent %d ms waiting on the connector. The source matched "+
			"nothing, but the round-trip itself was slow — usually a missing source-side "+
			"index covering the pushed predicate, a full-collection scan on the source, "+
			"or slow query planning on the source.",
		tableLabel, connector, top.scan.StageID, top.stageWait,
	)

	return &diagnose.Finding{
		RuleID:   ruleID,
		Severity: diagnose.SeverityInfo,
		Title:    title,
		Details:  details,
		Evidence: map[string]any{
			"scans":         buildSlowEmptyScanEvidence(hits, maxEvidence),
			"scans_matched": len(hits),
		},
		Remediation: "Verify the source has an index covering the pushed predicate. " +
			"For MongoDB, run explain() with the same filter and look for COLLSCAN. " +
			"For JDBC connectors, EXPLAIN the underlying SELECT and add a covering index. " +
			"When the filter is correct but always returns nothing for live traffic, " +
			"consider an upstream existence check (e.g. cached presence bit) so the " +
			"slow round-trip is skipped.",
	}
}

func buildSlowEmptyScanEvidence(hits []slowEmptyScanHit, maxEvidence int) []map[string]any {
	n := len(hits)
	if n > maxEvidence {
		n = maxEvidence
	}
	evidence := make([]map[string]any, 0, n)
	for i := 0; i < n; i++ {
		h := hits[i]
		ev := map[string]any{
			"stage_id":      h.scan.StageID,
			"table":         scanLabel(h.scan),
			"connector":     h.scan.ConnectorType,
			"stage_wait_ms": h.stageWait,
		}
		if h.scan.LocalFilter != "" {
			ev["local_filter"] = h.scan.LocalFilter
		}
		if len(h.scan.PushedConstraintColumns) > 0 {
			ev["pushed_constraint_columns"] = h.scan.PushedConstraintColumns
		}
		evidence = append(evidence, ev)
	}
	return evidence
}
