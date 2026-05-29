package rules

import (
	"strings"
	"testing"

	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

// The table name in this test is taken verbatim from query
// 20260521_180602_83664_httz8 in production. Keeping it here as a fidelity
// anchor makes the rule's intent obvious and prevents accidental regressions
// in the parser when the suffix format changes.
const realSnapshotPinnedTable = "agg_user_total_visits_v2$data@3681700557314111974"

// ----- detectIcebergMetadataForm -----

func TestDetectIcebergMetadataForm_SnapshotPinned(t *testing.T) {
	hit := detectIcebergMetadataForm(realSnapshotPinnedTable)
	if hit == nil {
		t.Fatalf("expected hit for %q, got nil", realSnapshotPinnedTable)
	}
	if hit.form != "$data@" {
		t.Errorf("form = %q, want %q", hit.form, "$data@")
	}
	if hit.baseTable != "agg_user_total_visits_v2" {
		t.Errorf("baseTable = %q", hit.baseTable)
	}
	if hit.snapshotID != "3681700557314111974" {
		t.Errorf("snapshotID = %q", hit.snapshotID)
	}
}

func TestDetectIcebergMetadataForm_BareData(t *testing.T) {
	hit := detectIcebergMetadataForm("orders$data")
	if hit == nil {
		t.Fatal("expected hit")
	}
	if hit.form != "$data" {
		t.Errorf("form = %q", hit.form)
	}
	if hit.snapshotID != "" {
		t.Errorf("snapshotID should be empty, got %q", hit.snapshotID)
	}
}

func TestDetectIcebergMetadataForm_IntrospectionForms(t *testing.T) {
	cases := []struct {
		table string
		form  string
	}{
		{"orders$files", "$files"},
		{"orders$partitions", "$partitions"},
		{"orders$snapshots", "$snapshots"},
		{"orders$history", "$history"},
		{"orders$manifests", "$manifests"},
		{"orders$refs", "$refs"},
		{"orders$properties", "$properties"},
		{"orders$entries", "$entries"},
	}
	for _, tc := range cases {
		t.Run(tc.form, func(t *testing.T) {
			hit := detectIcebergMetadataForm(tc.table)
			if hit == nil {
				t.Fatalf("expected hit for %q", tc.table)
			}
			if hit.form != tc.form {
				t.Errorf("form = %q, want %q", hit.form, tc.form)
			}
			if hit.baseTable != "orders" {
				t.Errorf("baseTable = %q", hit.baseTable)
			}
		})
	}
}

func TestDetectIcebergMetadataForm_PlainTableIsNotAHit(t *testing.T) {
	if hit := detectIcebergMetadataForm("orders"); hit != nil {
		t.Errorf("plain table should not match, got %+v", hit)
	}
	if hit := detectIcebergMetadataForm("public.orders"); hit != nil {
		t.Errorf("plain qualified table should not match, got %+v", hit)
	}
}

func TestDetectIcebergMetadataForm_UnknownSuffixIsNotAHit(t *testing.T) {
	// Not all $-suffixed names are Iceberg metadata tables — some other
	// connectors emit synthetic names too. We should only fire on the known
	// vocabulary; anything else is a false-positive risk.
	if hit := detectIcebergMetadataForm("orders$nonsense"); hit != nil {
		t.Errorf("unknown suffix should not match, got %+v", hit)
	}
}

func TestDetectIcebergMetadataForm_LeadingDollarIsNotAHit(t *testing.T) {
	// A leading-$ name is a synthetic/system identifier (or malformed
	// input), not an Iceberg table reference. The detector should not
	// claim a hit because there's no base table to fall back to.
	if hit := detectIcebergMetadataForm("$data@abc"); hit != nil {
		t.Errorf("leading-$ should not match, got %+v", hit)
	}
}

// ----- IcebergMetadataTable.Eval -----

func TestIcebergMetadataTable_Fires_SnapshotPinned(t *testing.T) {
	f := baseFacts()
	f.ScanPushdown = []queryinfo.ScanPushdownFact{
		{
			StageID:                "q.11",
			NodeName:               "ScanFilterProject",
			Catalog:                "app_lakehouse",
			Schema:                 "dbt_trusted",
			Table:                  realSnapshotPinnedTable,
			ConnectorType:          "iceberg",
			LocalFilter:            `("branch_id" = 42)`,
			PhysicalInputPositions: 135736,
			OutputRows:             850,
			Selectivity:            0.0063,
		},
	}

	finding := IcebergMetadataTable{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for snapshot-pinned $data@ scan")
	}
	if finding.RuleID != "trino.iceberg-metadata-table-disables-pushdown" {
		t.Errorf("rule_id = %q", finding.RuleID)
	}
	// Title should mention both the metadata form and the base table.
	if !strings.Contains(finding.Title, "3681700557314111974") {
		t.Errorf("title should mention snapshot id, got %q", finding.Title)
	}
	if !strings.Contains(finding.Title, "agg_user_total_visits_v2") {
		t.Errorf("title should mention base table, got %q", finding.Title)
	}
	// Remediation should propose the FOR VERSION AS OF rewrite with the
	// actual snapshot id substituted in — that's the whole point of the rule.
	if !strings.Contains(finding.Remediation, "FOR VERSION AS OF 3681700557314111974") {
		t.Errorf("remediation should propose FOR VERSION AS OF with snapshot id, got %q", finding.Remediation)
	}
	if !strings.Contains(finding.Remediation, "app_lakehouse.dbt_trusted.agg_user_total_visits_v2") {
		t.Errorf("remediation should include qualified base name, got %q", finding.Remediation)
	}
}

func TestIcebergMetadataTable_Details_QuoteLocalFilter(t *testing.T) {
	f := baseFacts()
	f.ScanPushdown = []queryinfo.ScanPushdownFact{
		{
			StageID:                "q.11",
			Catalog:                "app_lakehouse",
			Schema:                 "dbt_trusted",
			Table:                  realSnapshotPinnedTable,
			LocalFilter:            `("branch_id" = 42)`,
			PhysicalInputPositions: 1000,
			OutputRows:             1,
		},
	}
	finding := IcebergMetadataTable{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding")
	}
	if !strings.Contains(finding.Details, "branch_id") {
		t.Errorf("details should quote local filter, got %q", finding.Details)
	}
	if !strings.Contains(finding.Details, "wasted") {
		t.Errorf("details should mention wasted I/O when there's a sel gap, got %q", finding.Details)
	}
}

func TestIcebergMetadataTable_Fires_IntrospectionForm(t *testing.T) {
	f := baseFacts()
	f.ScanPushdown = []queryinfo.ScanPushdownFact{
		{
			StageID:                "q.3",
			Catalog:                "iceberg",
			Schema:                 "warehouse",
			Table:                  "orders$partitions",
			ConnectorType:          "iceberg",
			PhysicalInputPositions: 1024,
			OutputRows:             1024,
		},
	}
	finding := IcebergMetadataTable{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for $partitions metadata table")
	}
	if !strings.Contains(finding.Title, "partitions") {
		t.Errorf("title should mention the metadata form, got %q", finding.Title)
	}
	// For introspection forms we should NOT propose FOR VERSION AS OF —
	// that suggestion is wrong for $partitions and friends.
	if strings.Contains(finding.Remediation, "FOR VERSION AS OF") {
		t.Errorf("introspection remediation should not propose FOR VERSION AS OF, got %q", finding.Remediation)
	}
}

func TestIcebergMetadataTable_MultipleHits_SortAndCap(t *testing.T) {
	f := baseFacts()
	f.ScanPushdown = []queryinfo.ScanPushdownFact{
		// Smallest first; the rule should reorder so the biggest is in the title.
		{StageID: "q.5", Table: "small$data@abc", PhysicalInputPositions: 100},
		{StageID: "q.4", Table: "big$data@def", PhysicalInputPositions: 1_000_000},
		{StageID: "q.6", Table: "mid$data@ghi", PhysicalInputPositions: 5000},
	}
	finding := IcebergMetadataTable{MaxScansInEvidence: 2}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding")
	}
	if !strings.Contains(finding.Title, "big") {
		t.Errorf("title should lead with biggest scan, got %q", finding.Title)
	}
	ev, ok := finding.Evidence.(map[string]any)
	if !ok {
		t.Fatalf("evidence wrong type: %T", finding.Evidence)
	}
	scans, ok := ev["scans"].([]map[string]any)
	if !ok {
		t.Fatalf("evidence.scans wrong type: %T", ev["scans"])
	}
	if len(scans) != 2 {
		t.Errorf("evidence cap should kick in: got %d scans, want 2", len(scans))
	}
	if matched := ev["scans_matched"].(int); matched != 3 {
		t.Errorf("scans_matched = %d, want 3", matched)
	}
}

func TestIcebergMetadataTable_DoesNotFire_PlainTables(t *testing.T) {
	f := baseFacts()
	f.ScanPushdown = []queryinfo.ScanPushdownFact{
		{StageID: "q.5", Table: "orders", PhysicalInputPositions: 1000},
		{StageID: "q.6", Table: "lineitem", PhysicalInputPositions: 1_000_000},
	}
	if finding := (IcebergMetadataTable{}).Eval(f); finding != nil {
		t.Errorf("should not fire on plain tables, got %+v", finding)
	}
}

func TestIcebergMetadataTable_NilFacts(t *testing.T) {
	if finding := (IcebergMetadataTable{}).Eval(nil); finding != nil {
		t.Errorf("should not fire on nil facts, got %+v", finding)
	}
}
