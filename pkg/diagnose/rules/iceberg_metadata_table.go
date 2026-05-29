package rules

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Flgado/trino-insights-mcp/pkg/diagnose"
	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

// IcebergMetadataTable fires when a scan's table reference uses Iceberg's
// `$<metadata>` table syntax (e.g. `tbl$data@<snapshot-id>`, `tbl$files`,
// `tbl$partitions`). These syntactic forms route the scan through the
// connector's metadata-table code path, which historically does NOT
// support predicate pushdown / partition pruning the way the standard
// table scan does.
//
// The headline case this catches: dbt-iceberg integrations that emit
// `FROM <tbl>$data@<snapshot-id>` to pin a specific snapshot. The same
// intent can be expressed with the standard `FOR VERSION AS OF <id>`
// clause, which preserves pushdown. We've seen real federated queries
// where this single difference caused a 135 K-row scan to be reduced
// to ~850 useful rows in Trino-local filters — a 99.4 % wasted-I/O
// pattern that the local-filter-dominates rule also flags but can't
// explain the cause of.
//
// Detection is purely syntactic on the parsed `Table` field of each
// ScanPushdownFact. We do not require ConnectorType == "iceberg"
// because the `$` syntax is sufficiently unique in real Trino payloads
// that false positives are vanishingly rare; and the catalog vs.
// connector-type lookup is best-effort for fused ScanFilterProject
// nodes (the catalog name often masks the connector type).
type IcebergMetadataTable struct {
	// MaxScansInEvidence caps how many affected scans land in evidence.
	// Default 5.
	MaxScansInEvidence int
}

func (r IcebergMetadataTable) ID() string {
	return "trino.iceberg-metadata-table-disables-pushdown"
}

func (r IcebergMetadataTable) maxEvidence() int {
	if r.MaxScansInEvidence <= 0 {
		return 5
	}
	return r.MaxScansInEvidence
}

// metadataHit is one scan flagged by this rule, with the detected
// metadata-table form and the base table name parsed out for reporting.
type metadataHit struct {
	scan       queryinfo.ScanPushdownFact
	form       string // e.g. "$data@", "$files", "$partitions"
	baseTable  string // table name with the $ suffix stripped
	snapshotID string // populated only for "$data@<id>"
}

func (r IcebergMetadataTable) Eval(facts *queryinfo.QueryFacts) *diagnose.Finding {
	if facts == nil || len(facts.ScanPushdown) == 0 {
		return nil
	}

	hits := r.collectHits(facts.ScanPushdown)
	if len(hits) == 0 {
		return nil
	}

	// Sort by physical input bytes/rows so the worst offender is first in
	// the title (and in evidence after we cap).
	sort.Slice(hits, func(i, j int) bool {
		return hits[i].scan.PhysicalInputPositions > hits[j].scan.PhysicalInputPositions
	})

	return buildIcebergMetadataFinding(r.ID(), hits, r.maxEvidence())
}

func (r IcebergMetadataTable) collectHits(scans []queryinfo.ScanPushdownFact) []metadataHit {
	var hits []metadataHit
	for _, s := range scans {
		if s.Table == "" {
			continue
		}
		hit := detectIcebergMetadataForm(s.Table)
		if hit == nil {
			continue
		}
		hit.scan = s
		hits = append(hits, *hit)
	}
	return hits
}

// detectIcebergMetadataForm parses a Trino-Iceberg table reference and, if it
// contains a metadata-table suffix, returns the form ("$data@", "$files",
// etc.) and the base table name. Returns nil for plain table names.
//
// The forms recognised here come from the Iceberg connector documentation
// and from real Trino payloads observed in the wild:
//
//	tbl$data            — current snapshot's data files
//	tbl$data@<id>       — specific snapshot's data files (time-travel via metadata path)
//	tbl$files           — file metadata
//	tbl$partitions      — partition rollup
//	tbl$snapshots       — snapshot history
//	tbl$history         — change history
//	tbl$manifests       — manifest file listing
//	tbl$refs            — branches / tags (Iceberg V2)
//	tbl$properties      — table properties
//	tbl$entries         — manifest entries
func detectIcebergMetadataForm(table string) *metadataHit {
	dollar := strings.IndexByte(table, '$')
	if dollar <= 0 {
		// Either no $ (plain table) or $ at position 0 (synthetic name —
		// not an Iceberg reference). Either way, not a hit.
		return nil
	}
	base := table[:dollar]
	suffix := table[dollar:]

	form, snapshotID, ok := classifyMetadataSuffix(suffix)
	if !ok {
		return nil
	}
	return &metadataHit{
		form:       form,
		baseTable:  base,
		snapshotID: snapshotID,
	}
}

// classifyMetadataSuffix matches an Iceberg metadata-table suffix against the
// known forms. The "$data@<id>" form is special-cased so we can extract the
// snapshot id for the remediation message; all other forms are exact matches
// (no @<id> tail).
func classifyMetadataSuffix(suffix string) (form, snapshotID string, ok bool) {
	const dataPrefix = "$data@"
	if strings.HasPrefix(suffix, dataPrefix) {
		return dataPrefix, suffix[len(dataPrefix):], true
	}
	if suffix == "$data" {
		return "$data", "", true
	}
	for _, exact := range []string{
		"$files", "$partitions", "$snapshots",
		"$history", "$manifests", "$refs",
		"$properties", "$entries",
	} {
		if suffix == exact {
			return exact, "", true
		}
	}
	return "", "", false
}

func buildIcebergMetadataFinding(ruleID string, hits []metadataHit, maxEvidence int) *diagnose.Finding {
	top := hits[0]
	title := buildIcebergMetadataTitle(top)
	details := buildIcebergMetadataDetails(top, len(hits))
	remediation := buildIcebergMetadataRemediation(top)
	evidence := buildIcebergMetadataEvidence(hits, maxEvidence)

	return &diagnose.Finding{
		RuleID:   ruleID,
		Severity: diagnose.SeverityWarn,
		Title:    title,
		Details:  details,
		Evidence: map[string]any{
			"scans":         evidence,
			"scans_matched": len(hits),
			"form":          top.form,
		},
		Remediation: remediation,
	}
}

func buildIcebergMetadataTitle(top metadataHit) string {
	form := strings.TrimPrefix(top.form, "$")
	form = strings.TrimSuffix(form, "@")
	if top.snapshotID != "" {
		return fmt.Sprintf(
			"Iceberg snapshot-pinned scan (%s@%s) disables pushdown on %s",
			form, top.snapshotID, qualifiedBaseName(top),
		)
	}
	return fmt.Sprintf(
		"Iceberg %s metadata-table scan disables pushdown on %s",
		form, qualifiedBaseName(top),
	)
}

func buildIcebergMetadataDetails(top metadataHit, matched int) string {
	base := fmt.Sprintf(
		"Stage %s scans %q via the Iceberg connector's %s metadata-table code path. "+
			"This code path does not support predicate pushdown or partition pruning — "+
			"all rows produced by the metadata view are returned to Trino, and any WHERE "+
			"clause is evaluated locally.",
		top.scan.StageID, top.scan.Table, top.form,
	)
	if top.scan.LocalFilter != "" {
		base += fmt.Sprintf(
			" Concretely, the local filter %q was evaluated in Trino instead of being pushed down.",
			top.scan.LocalFilter,
		)
	}
	if top.scan.PhysicalInputPositions > 0 && top.scan.OutputRows > 0 &&
		top.scan.OutputRows < top.scan.PhysicalInputPositions {
		base += fmt.Sprintf(
			" The scan read %d rows but emitted %d after the local filter — %.1f%% of the I/O was wasted.",
			top.scan.PhysicalInputPositions,
			top.scan.OutputRows,
			100*(1-float64(top.scan.OutputRows)/float64(top.scan.PhysicalInputPositions)),
		)
	}
	if matched > 1 {
		base += fmt.Sprintf(" %d further scan(s) in this query use the same syntax (see evidence).", matched-1)
	}
	return base
}

func buildIcebergMetadataRemediation(top metadataHit) string {
	if top.form == "$data@" || top.form == "$data" {
		base := "Replace the `<tbl>$data@<snapshot-id>` syntax with the standard SQL " +
			"`FROM <tbl> FOR VERSION AS OF <snapshot-id>` (or `FOR TIMESTAMP AS OF <ts>`). " +
			"That form preserves predicate pushdown and partition pruning. If you didn't " +
			"mean to pin a specific snapshot, drop the `$data@<id>` suffix entirely and read " +
			"the current snapshot."
		if top.snapshotID != "" {
			base += fmt.Sprintf(
				" For this scan that's: `FROM %s FOR VERSION AS OF %s`.",
				qualifiedBaseName(top), top.snapshotID,
			)
		}
		return base
	}
	// Other metadata-table forms ($files, $partitions, etc.) are usually
	// queried intentionally for introspection. Inform but don't prescribe.
	return fmt.Sprintf(
		"`%s` is an Iceberg introspection table. Predicate pushdown is intentionally limited on "+
			"these views — all rows are returned and any WHERE clause is filtered locally. If "+
			"you're filtering heavily and performance matters, query the underlying table directly "+
			"(or join against the metadata view with a small driving filter).",
		top.form,
	)
}

func buildIcebergMetadataEvidence(hits []metadataHit, maxEvidence int) []map[string]any {
	n := len(hits)
	if n > maxEvidence {
		n = maxEvidence
	}
	out := make([]map[string]any, 0, n)
	for i := 0; i < n; i++ {
		h := hits[i]
		ev := map[string]any{
			"stage_id":   h.scan.StageID,
			"table":      h.scan.Table,
			"base_table": qualifiedBaseName(h),
			"form":       h.form,
		}
		if h.snapshotID != "" {
			ev["snapshot_id"] = h.snapshotID
		}
		if h.scan.LocalFilter != "" {
			ev["local_filter"] = h.scan.LocalFilter
		}
		if h.scan.PhysicalInputPositions > 0 {
			ev["physical_input_positions"] = h.scan.PhysicalInputPositions
		}
		if h.scan.OutputRows > 0 {
			ev["output_rows"] = h.scan.OutputRows
		}
		if h.scan.Selectivity > 0 {
			ev["selectivity"] = h.scan.Selectivity
		}
		out = append(out, ev)
	}
	return out
}

// qualifiedBaseName returns the catalog.schema.<base-table> form of the
// flagged scan, stripping the $<form> suffix from the table name. Useful
// for both reporting and the FOR VERSION AS OF rewrite suggestion.
func qualifiedBaseName(h metadataHit) string {
	if h.scan.Catalog == "" && h.scan.Schema == "" {
		return h.baseTable
	}
	parts := make([]string, 0, 3)
	if h.scan.Catalog != "" {
		parts = append(parts, h.scan.Catalog)
	}
	if h.scan.Schema != "" {
		parts = append(parts, h.scan.Schema)
	}
	parts = append(parts, h.baseTable)
	return strings.Join(parts, ".")
}
