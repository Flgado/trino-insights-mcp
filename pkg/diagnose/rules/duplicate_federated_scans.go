package rules

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Flgado/trino-insights-mcp/pkg/diagnose"
	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

// remoteConnectorPrefixes are connector types whose duplicate scans matter
// because each scan is a separate round-trip to an external system.
// Local connectors (memory, tpch, tpcds, system, blackhole) are excluded —
// duplicate scans there are cheap and not worth a finding.
var remoteConnectorPrefixes = []string{
	"mongo",
	"mysql",
	"postgresql",
	"redshift",
	"sqlserver",
	"oracle",
	"clickhouse",
	"cassandra",
	"bigquery",
	"snowflake",
	"hive",
	"iceberg",
	"delta",
	"elasticsearch",
	"kafka",
}

// DuplicateFederatedScans fires when the same physical table is scanned 2+ times
// in the same query against a remote/federated connector. This is the structural
// signature of CTE inlining producing N parallel round-trips (the user_membership
// 4×scan pattern from the working session).
type DuplicateFederatedScans struct {
	MinScans int // default 2
}

func (r DuplicateFederatedScans) ID() string { return "trino.duplicate-federated-scans" }

func (r DuplicateFederatedScans) minScans() int {
	if r.MinScans <= 0 {
		return 2
	}
	return r.MinScans
}

type scanGroupKey struct {
	catalog string
	schema  string
	table   string
}

func (k scanGroupKey) String() string {
	return fmt.Sprintf("%s.%s.%s", k.catalog, k.schema, k.table)
}

type duplicateGroup struct {
	key   scanGroupKey
	scans []queryinfo.ScanPushdownFact
}

func (r DuplicateFederatedScans) Eval(facts *queryinfo.QueryFacts) *diagnose.Finding {
	if facts == nil || len(facts.ScanPushdown) == 0 {
		return nil
	}

	groups := groupScansByTable(facts.ScanPushdown, false)
	dups := r.collectDuplicates(groups)
	if len(dups) == 0 {
		return nil
	}

	sort.Slice(dups, func(i, j int) bool {
		return len(dups[i].scans) > len(dups[j].scans)
	})

	return buildDuplicateFinding(r.ID(), dups)
}

func (r DuplicateFederatedScans) collectDuplicates(groups map[scanGroupKey][]queryinfo.ScanPushdownFact) []duplicateGroup {
	var dups []duplicateGroup
	for k, g := range groups {
		if len(g) < r.minScans() {
			continue
		}
		if !isRemoteConnector(g[0].ConnectorType) {
			continue
		}
		dups = append(dups, duplicateGroup{key: k, scans: g})
	}
	return dups
}

func buildDuplicateFinding(ruleID string, dups []duplicateGroup) *diagnose.Finding {
	worst := dups[0]
	stageIDs := make([]string, 0, len(worst.scans))
	for _, s := range worst.scans {
		stageIDs = append(stageIDs, s.StageID)
	}
	sort.Strings(stageIDs)

	title := fmt.Sprintf("%s scanned %d times in this query", worst.key.String(), len(worst.scans))
	details := fmt.Sprintf(
		"Table %s is scanned %d times (stages: %s). Each scan is an independent round-trip "+
			"to the federated source — they run in parallel but compete for the same connection pool "+
			"and prevent the source from caching/index reuse. This is usually caused by CTE inlining: "+
			"Trino inlines CTEs by default and specialises each consumer with its own predicate, "+
			"turning one logical CTE reference into N physical scans.",
		worst.key.String(), len(worst.scans), strings.Join(stageIDs, ", "),
	)

	evidence := make([]map[string]any, 0, len(dups))
	for _, g := range dups {
		evidence = append(evidence, map[string]any{
			"table":      g.key.String(),
			"scan_count": len(g.scans),
			"scans":      scansToEvidence(g.scans),
		})
	}

	return &diagnose.Finding{
		RuleID:   ruleID,
		Severity: diagnose.SeverityWarn,
		Title:    title,
		Details:  details,
		Evidence: map[string]any{
			"duplicate_groups": evidence,
		},
		Remediation: "Either (a) restructure the SQL to reference each CTE once and select-distribute downstream, " +
			"(b) enable Trino CTE materialisation via session property cte_materialization_strategy (Trino 451+), " +
			"or (c) push the duplicate-scan UNION into the source database itself (e.g. write a view in MySQL/Mongo) " +
			"so the source can plan one query instead of N.",
	}
}

func isRemoteConnector(connectorType string) bool {
	if connectorType == "" {
		return false
	}
	ct := strings.ToLower(connectorType)
	for _, prefix := range remoteConnectorPrefixes {
		if strings.HasPrefix(ct, prefix) {
			return true
		}
	}
	return false
}

// groupScansByTable buckets ScanPushdownFact entries by (catalog, schema, table).
// When requireRows is true, scans with PhysicalInputPositions == 0 are excluded
// (used by rules that need row counts to be meaningful).
func groupScansByTable(scans []queryinfo.ScanPushdownFact, requireRows bool) map[scanGroupKey][]queryinfo.ScanPushdownFact {
	groups := make(map[scanGroupKey][]queryinfo.ScanPushdownFact)
	for _, s := range scans {
		if s.Table == "" {
			continue
		}
		if requireRows && s.PhysicalInputPositions == 0 {
			continue
		}
		key := scanGroupKey{catalog: s.Catalog, schema: s.Schema, table: s.Table}
		groups[key] = append(groups[key], s)
	}
	return groups
}
