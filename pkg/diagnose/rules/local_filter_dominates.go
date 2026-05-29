package rules

import (
	"fmt"
	"sort"

	"github.com/Flgado/trino-insights-mcp/pkg/diagnose"
	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

// LocalFilterDominates fires when a scan returned far more rows than it emitted
// AND a Trino-side filter expression sits above the scan — i.e. the connector
// over-fetched because the predicate couldn't be pushed down.
//
// This catches the "7,834 docs read -> 0 useful rows" pattern that's invisible
// to scan-too-large (the row count is small) and poor-selectivity (it's measured
// per-query, not per-scan).
type LocalFilterDominates struct {
	MinPhysicalRows    int64   // default 100  — ignore tiny scans
	MaxSelectivity     float64 // default 0.05 (5%) — output_rows / physical_input
	MaxScansInEvidence int     // default 5 — cap evidence size
}

func (r LocalFilterDominates) ID() string { return "trino.local-filter-dominates" }

func (r LocalFilterDominates) minRows() int64 {
	if r.MinPhysicalRows <= 0 {
		return 100
	}
	return r.MinPhysicalRows
}

func (r LocalFilterDominates) maxSelectivity() float64 {
	if r.MaxSelectivity <= 0 {
		return 0.05
	}
	return r.MaxSelectivity
}

func (r LocalFilterDominates) maxEvidence() int {
	if r.MaxScansInEvidence <= 0 {
		return 5
	}
	return r.MaxScansInEvidence
}

type localFilterHit struct {
	scan        queryinfo.ScanPushdownFact
	rejectedPct float64
}

func (r LocalFilterDominates) Eval(facts *queryinfo.QueryFacts) *diagnose.Finding {
	if facts == nil || len(facts.ScanPushdown) == 0 {
		return nil
	}

	hits := r.collectHits(facts.ScanPushdown)
	if len(hits) == 0 {
		return nil
	}

	sort.Slice(hits, func(i, j int) bool {
		return hits[i].scan.PhysicalInputPositions > hits[j].scan.PhysicalInputPositions
	})

	return buildLocalFilterFinding(r.ID(), hits, r.maxEvidence())
}

func (r LocalFilterDominates) collectHits(scans []queryinfo.ScanPushdownFact) []localFilterHit {
	var hits []localFilterHit
	for _, s := range scans {
		if !r.matches(s) {
			continue
		}
		sel := scanSelectivity(s)
		if sel > r.maxSelectivity() {
			continue
		}
		hits = append(hits, localFilterHit{scan: s, rejectedPct: (1 - sel) * 100})
	}
	return hits
}

func (r LocalFilterDominates) matches(s queryinfo.ScanPushdownFact) bool {
	if s.LocalFilter == "" {
		return false
	}
	return s.PhysicalInputPositions >= r.minRows()
}

func scanSelectivity(s queryinfo.ScanPushdownFact) float64 {
	if s.Selectivity > 0 {
		return s.Selectivity
	}
	if s.PhysicalInputPositions <= 0 {
		return 0
	}
	return float64(s.OutputRows) / float64(s.PhysicalInputPositions)
}

func buildLocalFilterFinding(ruleID string, hits []localFilterHit, maxEvidence int) *diagnose.Finding {
	top := hits[0]
	tableLabel := scanLabel(top.scan)

	title := fmt.Sprintf("Local filter rejects %.0f%% of rows on %s", top.rejectedPct, tableLabel)

	details := fmt.Sprintf(
		"Scan on %s in %s returned %d rows but emitted only %d after the locally-evaluated "+
			"filter %q. The connector over-fetched because Trino could not push this predicate down. "+
			"Either the predicate uses a function/JSON expression the connector cannot translate, "+
			"or the column type/collation blocks pushdown.",
		tableLabel, top.scan.StageID,
		top.scan.PhysicalInputPositions, top.scan.OutputRows,
		top.scan.LocalFilter,
	)

	evidence := buildLocalFilterEvidence(hits, maxEvidence)

	return &diagnose.Finding{
		RuleID:   ruleID,
		Severity: diagnose.SeverityWarn,
		Title:    title,
		Details:  details,
		Evidence: map[string]any{
			"scans":         evidence,
			"scans_matched": len(hits),
		},
		Remediation: "Rewrite the predicate so the connector can push it down: avoid CASE/JSON/function-wrapped " +
			"columns, ensure the column type is comparable (e.g. cast a VARCHAR computed column to a hex-safe form), " +
			"or pre-filter in a CTE that the connector understands. For MongoDB, set mongodb.projection-pushdown-enabled=true. " +
			"For JDBC connectors, check predicate-pushdown-enabled and ensure the filter is on a directly-mapped column.",
	}
}

func scanLabel(s queryinfo.ScanPushdownFact) string {
	if s.Catalog == "" {
		return s.Table
	}
	return fmt.Sprintf("%s.%s.%s", s.Catalog, s.Schema, s.Table)
}

func buildLocalFilterEvidence(hits []localFilterHit, maxEvidence int) []map[string]any {
	n := len(hits)
	if n > maxEvidence {
		n = maxEvidence
	}
	evidence := make([]map[string]any, 0, n)
	for i := 0; i < n; i++ {
		h := hits[i]
		ev := map[string]any{
			"stage_id":                 h.scan.StageID,
			"table":                    scanLabel(h.scan),
			"connector":                h.scan.ConnectorType,
			"physical_input_positions": h.scan.PhysicalInputPositions,
			"output_rows":              h.scan.OutputRows,
			"selectivity":              h.scan.Selectivity,
			"local_filter":             h.scan.LocalFilter,
		}
		if len(h.scan.PushedConstraintColumns) > 0 {
			ev["pushed_constraint_columns"] = h.scan.PushedConstraintColumns
		}
		evidence = append(evidence, ev)
	}
	return evidence
}
