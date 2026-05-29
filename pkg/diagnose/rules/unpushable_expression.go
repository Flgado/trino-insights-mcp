package rules

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Flgado/trino-insights-mcp/pkg/diagnose"
	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

// UnpushableExpression fires when a scan's LocalFilter contains a syntactic
// pattern we know cannot be pushed to its target connector. It is the
// causal companion to LocalFilterDominates: that rule says "this scan is
// wasteful"; this rule says "and here's the specific SQL fragment that
// caused it, with a connector-aware rewrite recipe."
//
// The detector is intentionally syntactic (regex over the plan's
// filterPredicate text). We never claim a pattern is unpushable unless we
// have a concrete remediation for it. If the LocalFilter has no recognised
// pattern, the rule stays silent and the LLM can fall back to reading the
// raw text.
//
// Scope of this PR (Option B in the design discussion):
//   - Connector-agnostic patterns: function wrappers, CAST wrappers,
//     column arithmetic, CASE expressions.
//   - MongoDB-specific: CARDINALITY, element_at, JSON_EXTRACT.
//   - JDBC-specific (mysql + postgresql): COALESCE wrappers, JSON_EXTRACT.
//
// Iceberg / Hive / Memory patterns are deliberately left for a follow-up
// PR once we've validated the framework on real federated queries.
type UnpushableExpression struct {
	// MinPhysicalRows skips scans below this row count. The signal here is
	// the SHAPE of the predicate, not its size, so the default is 0 — even
	// a small scan with an unpushable predicate is worth flagging because
	// the rewrite recipe is independent of cardinality.
	MinPhysicalRows int64
	// MaxHitsInEvidence caps how many (scan × pattern) hits land in
	// evidence. Default 10.
	MaxHitsInEvidence int
}

func (r UnpushableExpression) ID() string { return "trino.unpushable-expression" }

func (r UnpushableExpression) maxEvidence() int {
	if r.MaxHitsInEvidence <= 0 {
		return 10
	}
	return r.MaxHitsInEvidence
}

// unpushablePattern is one entry in the catalogue. The re is matched against
// the scan's LocalFilter; if applicableConnectors is empty the pattern is
// connector-agnostic, otherwise the scan's ConnectorType must contain one
// of the listed substrings (case-insensitive).
type unpushablePattern struct {
	id                   string
	re                   *regexp.Regexp
	applicableConnectors []string
	reason               string
	remediation          string
}

// patternHit is one (scan × pattern) match.
type patternHit struct {
	scan        queryinfo.ScanPushdownFact
	pattern     *unpushablePattern
	matchedExpr string
}

// unpushableCatalogue is the source of truth. Order is significant only for
// stable iteration and evidence ordering — within a single scan we report
// all patterns that match.
//
// Regex design note: plan-text rendering of column references varies by
// connector. Mongo emits double-quoted identifiers ("col"); Iceberg, MySQL,
// and Postgres emit bare identifiers (col) in fused ScanFilterProject text;
// some connectors emit aliased forms (col_10). Therefore the function-call
// patterns below do NOT anchor on a quoted column — they match the call
// shape only, and rely on the broader semantic argument: any occurrence
// of these calls inside a LocalFilter is by definition unpushable, because
// LocalFilter is exactly the text the connector REFUSED to push.
//
// `[^,)]+` after the open paren captures the first argument up to a comma
// or close-paren, which gives a useful diagnostic fragment without
// requiring full paren-matching.
var unpushableCatalogue = []unpushablePattern{
	{
		id: "wrap.function",
		re: regexp.MustCompile(`(?i)\b(lower|upper|trim|ltrim|rtrim|substring|substr|replace|concat)\s*\(\s*[^,)]+`),
		reason: "Predicate wraps an expression in a scalar function (e.g. lower(col)). " +
			"Most connectors need a functional / expression index to push such a predicate to the source; " +
			"without one the column is streamed back and Trino evaluates the function locally on every row.",
		remediation: "Either expose the function result as a generated / computed column at the source " +
			"(MySQL & Postgres both support functional indexes), or eliminate the wrapper at query-write time " +
			"(e.g. store data already lower-cased, compare a quoted literal directly without normalisation).",
	},
	{
		id: "wrap.cast",
		re: regexp.MustCompile(`(?i)\bCAST\s*\(\s*[^)]*?\s+AS\b`),
		reason: "Predicate wraps a column in a CAST. The connector sees a different type than the source " +
			"column and almost never translates the cast into source SQL, so the entire column is streamed back " +
			"and the cast + comparison both run in Trino.",
		remediation: "Match the predicate to the column's native type instead of casting the column. " +
			"If the literal side is the wrong type, cast the literal — that's free; casting the column is fatal. " +
			"If the column genuinely has the wrong type at the source, fix it upstream.",
	},
	{
		// Arithmetic stays anchored on a column-like token because without
		// it the pattern would false-positive on constant arithmetic that
		// Trino occasionally leaves un-folded inside larger expressions.
		// Both quoted and bare identifiers are accepted.
		id: "wrap.arithmetic",
		re: regexp.MustCompile(`(?i)(?:"[^"]+"|\b[a-z_][a-z0-9_]*)\s*[+\-*/]\s*[\d"]`),
		reason: "Predicate performs arithmetic on a column (col + N, col * N, etc.). This is almost " +
			"universally unpushable — connectors expect predicates with the column alone on one side and a " +
			"literal on the other.",
		remediation: "Move the arithmetic to the literal side of the comparison: rewrite `col + 1 > 100` " +
			"as `col > 99`. The result set is identical and the connector can push the rewritten form.",
	},
	{
		id: "wrap.case",
		re: regexp.MustCompile(`(?i)\bCASE\s+WHEN\b`),
		reason: "Predicate contains a CASE expression. CASE always evaluates locally in Trino — no " +
			"connector translates it to source SQL.",
		remediation: "Lift the CASE out of the predicate. `CASE WHEN col='A' OR col='B' THEN 1 END = 1` " +
			"is just `col IN ('A','B')`. If the CASE encodes business logic, materialise that logic upstream " +
			"as a derived column or a view.",
	},

	{
		id:                   "mongo.cardinality",
		re:                   regexp.MustCompile(`(?i)\bCARDINALITY\s*\(\s*[^,)]+`),
		applicableConnectors: []string{"mongo"},
		reason: "Trino's MongoDB connector does not translate CARDINALITY(array) to MongoDB's $size " +
			"operator. The entire array column is streamed back and Trino evaluates CARDINALITY row-by-row.",
		remediation: "If you're checking for a non-empty array, rewrite as " +
			"`element_at(\"col\", 1) IS NOT NULL` (still requires the field to exist) or store an explicit " +
			"`array_length` integer column alongside the array. Setting " +
			"mongodb.projection-pushdown-enabled=true only helps projections, not predicates.",
	},
	{
		id:                   "mongo.element-at",
		re:                   regexp.MustCompile(`(?i)\belement_at\s*\(\s*[^,)]+`),
		applicableConnectors: []string{"mongo"},
		reason: "element_at() / array_position() have no MongoDB equivalent in the connector. The array " +
			"is fully materialised in Trino before the index lookup.",
		remediation: "Pre-project the specific array element you need at the source by storing it as its " +
			"own field, or restructure the document so the relevant value lives at top level. UNNEST + a " +
			"row-number filter inside a CTE Trino can still planar more efficiently than element_at.",
	},

	{
		id:                   "jdbc.coalesce",
		re:                   regexp.MustCompile(`(?i)\bCOALESCE\s*\(\s*[^,)]+`),
		applicableConnectors: []string{"mysql", "postgresql", "postgres"},
		reason: "Trino's JDBC connector cannot synthesise `(col = X OR col IS NULL)` from a `COALESCE(col, default) = X` " +
			"predicate, so the wrapper kills pushdown and the full column is streamed back. The same applies " +
			"when COALESCE wraps a boolean expression (`COALESCE(col = X, FALSE) = TRUE`) — that's just a " +
			"three-valued-logic workaround whose translation the connector still cannot perform.",
		remediation: "Replace `COALESCE(col, default) = X` with the explicit two-arm form: " +
			"`(col = X OR (X = default AND col IS NULL))`. For the boolean-expression variant " +
			"(`COALESCE(col = X, FALSE) = TRUE`), drop the wrapper entirely — the inner equality already " +
			"handles NULL correctly via SQL's three-valued logic (`col = X` is NULL when col is NULL, which " +
			"the WHERE clause treats as false). Even better, fix the column at the source so it's NOT NULL " +
			"with a default — that lets you drop the COALESCE entirely and keeps both Trino and the source " +
			"planner happy.",
	},

	{
		id:                   "json-extract",
		re:                   regexp.MustCompile(`(?i)\bJSON_EXTRACT\s*\(\s*[^,)]+`),
		applicableConnectors: []string{"mongo", "mysql", "postgresql", "postgres"},
		reason: "Trino's JSON_EXTRACT() syntax does not translate to MongoDB's dotted-path accessor, " +
			"MySQL's native JSON_EXTRACT, or Postgres's `->`/`->>` operators. The JSON column is fetched " +
			"whole and Trino navigates the path locally.",
		remediation: "For MongoDB: project the nested field directly (`SELECT col.subfield`) — the " +
			"connector pushes dotted-path accessors natively. For MySQL/Postgres: expose the JSON path as a " +
			"generated / computed column at the source so it becomes a regular indexable column from Trino's " +
			"point of view.",
	},
}

func (r UnpushableExpression) Eval(facts *queryinfo.QueryFacts) *diagnose.Finding {
	if facts == nil || len(facts.ScanPushdown) == 0 {
		return nil
	}

	hits := r.collectHits(facts.ScanPushdown)
	if len(hits) == 0 {
		return nil
	}

	// Stable sort: biggest scan first, then by pattern id for deterministic
	// ordering when two patterns match the same scan. This keeps tests
	// stable across runs and the worst offender lands at the top of the
	// evidence list.
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].scan.PhysicalInputPositions != hits[j].scan.PhysicalInputPositions {
			return hits[i].scan.PhysicalInputPositions > hits[j].scan.PhysicalInputPositions
		}
		return hits[i].pattern.id < hits[j].pattern.id
	})

	return buildUnpushableFinding(r.ID(), hits, r.maxEvidence())
}

func (r UnpushableExpression) collectHits(scans []queryinfo.ScanPushdownFact) []patternHit {
	var hits []patternHit
	for _, s := range scans {
		if s.LocalFilter == "" {
			continue
		}
		if s.PhysicalInputPositions < r.MinPhysicalRows {
			continue
		}
		hits = append(hits, matchPatternsForScan(s)...)
	}
	return hits
}

// matchPatternsForScan walks the catalogue and returns every pattern that
// matches the scan's LocalFilter and is applicable to its connector.
func matchPatternsForScan(s queryinfo.ScanPushdownFact) []patternHit {
	var out []patternHit
	for i := range unpushableCatalogue {
		p := &unpushableCatalogue[i]
		if !patternAppliesToConnector(p, s.ConnectorType) {
			continue
		}
		m := p.re.FindString(s.LocalFilter)
		if m == "" {
			continue
		}
		out = append(out, patternHit{
			scan:        s,
			pattern:     p,
			matchedExpr: m,
		})
	}
	return out
}

// patternAppliesToConnector decides whether a pattern's connector scope
// covers the scan's connector. An empty applicableConnectors slice means
// the pattern is agnostic and always applies.
//
// We use case-insensitive substring matching so that `mongodb`, `MongoDB`,
// `mongo-cdc`, etc. all match the `mongo` scope. This is intentionally
// lenient — false positives here are bounded (the pattern still has to
// match the SQL text) and the alternative (an exact enum) would require
// us to keep an exhaustive list of connector-name variants.
func patternAppliesToConnector(p *unpushablePattern, connectorType string) bool {
	if len(p.applicableConnectors) == 0 {
		return true
	}
	ct := strings.ToLower(connectorType)
	if ct == "" {
		return false
	}
	for _, want := range p.applicableConnectors {
		if strings.Contains(ct, want) {
			return true
		}
	}
	return false
}

func buildUnpushableFinding(ruleID string, hits []patternHit, maxEvidence int) *diagnose.Finding {
	top := hits[0]
	patternCount := countDistinctPatterns(hits)
	scanCount := countDistinctScans(hits)

	title := buildUnpushableTitle(top)
	details := buildUnpushableDetails(top, patternCount, scanCount)
	remediation := buildUnpushableRemediation(top, patternCount)
	evidence := buildUnpushableEvidence(hits, maxEvidence)

	return &diagnose.Finding{
		RuleID:   ruleID,
		Severity: diagnose.SeverityInfo,
		Title:    title,
		Details:  details,
		Evidence: map[string]any{
			"hits":              evidence,
			"hits_matched":      len(hits),
			"distinct_patterns": patternCount,
			"distinct_scans":    scanCount,
		},
		Remediation: remediation,
	}
}

func buildUnpushableTitle(top patternHit) string {
	connectorLabel := "the target connector"
	if top.scan.ConnectorType != "" {
		connectorLabel = top.scan.ConnectorType
	}
	return fmt.Sprintf(
		"Unpushable predicate %q on %s cannot reach %s",
		top.matchedExpr,
		scanLabel(top.scan),
		connectorLabel,
	)
}

func buildUnpushableDetails(top patternHit, distinctPatterns, distinctScans int) string {
	base := fmt.Sprintf(
		"On stage %s, scan %s has a local filter %q. The fragment %q is recognised as an "+
			"unpushable construct — %s",
		top.scan.StageID,
		scanLabel(top.scan),
		top.scan.LocalFilter,
		top.matchedExpr,
		top.pattern.reason,
	)
	if distinctPatterns > 1 || distinctScans > 1 {
		base += fmt.Sprintf(
			" %d distinct unpushable pattern(s) detected across %d scan(s) — see evidence for the full list.",
			distinctPatterns, distinctScans,
		)
	}
	base += " This finding is the diagnostic cause behind trino.local-filter-dominates / trino.poor-selectivity " +
		"when those rules fire on the same scan; fold them together in the root-cause section of the report."
	return base
}

func buildUnpushableRemediation(top patternHit, distinctPatterns int) string {
	if distinctPatterns == 1 {
		return top.pattern.remediation
	}
	// Multiple distinct patterns — give the worst-offender remediation but
	// prompt the reader to consult evidence for the others.
	return top.pattern.remediation +
		" Additional patterns were detected on other scans; consult the evidence block " +
		"for each pattern's specific rewrite."
}

func countDistinctPatterns(hits []patternHit) int {
	seen := make(map[string]struct{}, len(hits))
	for _, h := range hits {
		seen[h.pattern.id] = struct{}{}
	}
	return len(seen)
}

func countDistinctScans(hits []patternHit) int {
	seen := make(map[string]struct{}, len(hits))
	for _, h := range hits {
		seen[h.scan.StageID+":"+h.scan.Table] = struct{}{}
	}
	return len(seen)
}

func buildUnpushableEvidence(hits []patternHit, maxEvidence int) []map[string]any {
	n := len(hits)
	if n > maxEvidence {
		n = maxEvidence
	}
	out := make([]map[string]any, 0, n)
	for i := 0; i < n; i++ {
		h := hits[i]
		ev := map[string]any{
			"pattern_id":       h.pattern.id,
			"stage_id":         h.scan.StageID,
			"table":            scanLabel(h.scan),
			"matched_fragment": h.matchedExpr,
			"local_filter":     h.scan.LocalFilter,
			"reason":           h.pattern.reason,
			"remediation":      h.pattern.remediation,
		}
		if h.scan.ConnectorType != "" {
			ev["connector"] = h.scan.ConnectorType
		}
		if h.scan.PhysicalInputPositions > 0 {
			ev["physical_input_positions"] = h.scan.PhysicalInputPositions
		}
		out = append(out, ev)
	}
	return out
}
