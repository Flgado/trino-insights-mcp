package rules

import (
	"fmt"
	"sort"

	"github.com/Flgado/trino-insights-mcp/pkg/diagnose"
	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

// DivergentScanRowcounts fires when sibling scans of the same (catalog, schema, table)
// return wildly different row counts. This is empirical evidence that a value-specific
// predicate IS being pushed to the source — useful to distinguish "pushdown works
// for these literals but not others" cases that the optimizer rule summaries can't tell you.
//
// Concrete win: the working session had 4 sibling user_membership scans returning
// 8999 / 412 / 47 / 69 rows depending on status. The 200×+ spread proves status
// IS pushing, even though docs claim VARCHAR predicates may not push to MySQL.
type DivergentScanRowcounts struct {
	MinScans int     // default 2
	MinRows  int64   // default 100 — ignore tiny scans
	RatioMin float64 // default 10 — max/min must be >= this
}

func (r DivergentScanRowcounts) ID() string { return "trino.divergent-scan-rowcounts" }

func (r DivergentScanRowcounts) minScans() int {
	if r.MinScans <= 0 {
		return 2
	}
	return r.MinScans
}

func (r DivergentScanRowcounts) minRows() int64 {
	if r.MinRows <= 0 {
		return 100
	}
	return r.MinRows
}

func (r DivergentScanRowcounts) ratioMin() float64 {
	if r.RatioMin <= 0 {
		return 10
	}
	return r.RatioMin
}

type divergentGroup struct {
	key   scanGroupKey
	scans []queryinfo.ScanPushdownFact
	minR  int64
	maxR  int64
}

func (r DivergentScanRowcounts) Eval(facts *queryinfo.QueryFacts) *diagnose.Finding {
	if facts == nil || len(facts.ScanPushdown) == 0 {
		return nil
	}

	groups := groupScansByTable(facts.ScanPushdown, true)
	hits := r.findDivergentGroups(groups)
	if len(hits) == 0 {
		return nil
	}

	sort.Slice(hits, func(i, j int) bool {
		return hits[i].ratio() > hits[j].ratio()
	})

	return buildDivergentFinding(r.ID(), hits)
}

func (g divergentGroup) ratio() float64 {
	if g.minR <= 0 {
		return 0
	}
	return float64(g.maxR) / float64(g.minR)
}

func (r DivergentScanRowcounts) findDivergentGroups(groups map[scanGroupKey][]queryinfo.ScanPushdownFact) []divergentGroup {
	var hits []divergentGroup
	for k, g := range groups {
		if len(g) < r.minScans() {
			continue
		}
		minR, maxR := minMaxPhysicalRows(g)
		if maxR < r.minRows() || minR <= 0 {
			continue
		}
		if float64(maxR)/float64(minR) < r.ratioMin() {
			continue
		}
		hits = append(hits, divergentGroup{key: k, scans: g, minR: minR, maxR: maxR})
	}
	return hits
}

func minMaxPhysicalRows(scans []queryinfo.ScanPushdownFact) (minR, maxR int64) {
	minR, maxR = scans[0].PhysicalInputPositions, scans[0].PhysicalInputPositions
	for _, s := range scans[1:] {
		if s.PhysicalInputPositions < minR {
			minR = s.PhysicalInputPositions
		}
		if s.PhysicalInputPositions > maxR {
			maxR = s.PhysicalInputPositions
		}
	}
	return
}

func buildDivergentFinding(ruleID string, hits []divergentGroup) *diagnose.Finding {
	top := hits[0]
	rows := make([]int64, 0, len(top.scans))
	for _, s := range top.scans {
		rows = append(rows, s.PhysicalInputPositions)
	}

	title := fmt.Sprintf("Sibling scans of %s have %dx row-count spread (pushdown is value-sensitive)",
		top.key.String(), int(top.ratio()))

	details := fmt.Sprintf(
		"Table %s is scanned %d times with row counts %v. The spread between max (%d) and min (%d) "+
			"is large enough to prove the connector IS pushing a value-specific predicate (otherwise all "+
			"scans would return the same row count). This is useful evidence when docs claim a predicate "+
			"\"may not push\" — the row counts show what actually happened. Look at the SQL to identify "+
			"which equality/IN literal differs between the scans.",
		top.key.String(), len(top.scans), rows, top.maxR, top.minR,
	)

	evidence := make([]map[string]any, 0, len(hits))
	for _, h := range hits {
		evidence = append(evidence, map[string]any{
			"table": h.key.String(),
			"min":   h.minR,
			"max":   h.maxR,
			"ratio": h.ratio(),
			"scans": scansToEvidence(h.scans),
		})
	}

	return &diagnose.Finding{
		RuleID:   ruleID,
		Severity: diagnose.SeverityInfo,
		Title:    title,
		Details:  details,
		Evidence: map[string]any{
			"divergent_groups": evidence,
		},
		Remediation: "No remediation needed — this finding is observational. Use it to confirm pushdown " +
			"behaviour when answering 'is this predicate being pushed?' questions. Combine with the SQL " +
			"to identify which literal in which CTE arm caused each row count.",
	}
}

func scansToEvidence(scans []queryinfo.ScanPushdownFact) []map[string]any {
	out := make([]map[string]any, 0, len(scans))
	for _, s := range scans {
		out = append(out, map[string]any{
			"stage_id":                  s.StageID,
			"physical_input_positions":  s.PhysicalInputPositions,
			"output_rows":               s.OutputRows,
			"local_filter":              s.LocalFilter,
			"pushed_constraint_columns": s.PushedConstraintColumns,
		})
	}
	return out
}
