package rules

import (
	"strings"
	"testing"

	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

// ----- LocalFilterDominates -----

func TestLocalFilterDominates_Fires(t *testing.T) {
	f := baseFacts()
	f.ScanPushdown = []queryinfo.ScanPushdownFact{
		{
			StageID:                "q.7",
			NodeName:               "ScanFilterProject",
			Catalog:                "mongo",
			Schema:                 "platform",
			Table:                  "user_credits",
			ConnectorType:          "mongo",
			LocalFilter:            `("status" = 'ACTIVE')`,
			PhysicalInputPositions: 7834,
			OutputRows:             0,
			Selectivity:            0,
		},
	}

	finding := LocalFilterDominates{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for local filter dominating")
	}
	if finding.RuleID != "trino.local-filter-dominates" {
		t.Errorf("rule_id = %q", finding.RuleID)
	}
	if !strings.Contains(finding.Title, "user_credits") {
		t.Errorf("title should mention table, got %q", finding.Title)
	}
	if !strings.Contains(finding.Details, `status`) || !strings.Contains(finding.Details, `ACTIVE`) {
		t.Errorf("details should reference the local filter columns/literals, got %q", finding.Details)
	}
}

func TestLocalFilterDominates_NoLocalFilter(t *testing.T) {
	f := baseFacts()
	f.ScanPushdown = []queryinfo.ScanPushdownFact{
		{
			StageID:                "q.7",
			Catalog:                "mongo",
			Table:                  "user_credits",
			PhysicalInputPositions: 7834,
			OutputRows:             0,
		},
	}
	r := LocalFilterDominates{}
	if r.Eval(f) != nil {
		t.Error("should not fire when LocalFilter is empty (pushdown was complete)")
	}
}

func TestLocalFilterDominates_TinyScan(t *testing.T) {
	f := baseFacts()
	f.ScanPushdown = []queryinfo.ScanPushdownFact{
		{
			StageID:                "q.7",
			Catalog:                "mongo",
			Table:                  "user_credits",
			LocalFilter:            "x = 1",
			PhysicalInputPositions: 10,
			OutputRows:             0,
		},
	}
	r := LocalFilterDominates{}
	if r.Eval(f) != nil {
		t.Error("should not fire for tiny scans (noise)")
	}
}

func TestLocalFilterDominates_GoodSelectivity(t *testing.T) {
	f := baseFacts()
	f.ScanPushdown = []queryinfo.ScanPushdownFact{
		{
			StageID:                "q.7",
			Catalog:                "mongo",
			Table:                  "user_credits",
			LocalFilter:            "x = 1",
			PhysicalInputPositions: 10000,
			OutputRows:             9000,
			Selectivity:            0.9,
		},
	}
	r := LocalFilterDominates{}
	if r.Eval(f) != nil {
		t.Error("should not fire when local filter doesn't reject much")
	}
}

func TestLocalFilterDominates_EmptyScanPushdown(t *testing.T) {
	f := baseFacts()
	r := LocalFilterDominates{}
	if r.Eval(f) != nil {
		t.Error("should not fire when no scan pushdown facts present")
	}
}

// ----- DuplicateFederatedScans -----

func TestDuplicateFederatedScans_Fires(t *testing.T) {
	f := baseFacts()
	f.ScanPushdown = []queryinfo.ScanPushdownFact{
		{StageID: "q.4", Catalog: "mysql", Schema: "platform", Table: "user_membership", ConnectorType: "mysql", PhysicalInputPositions: 8999, OutputRows: 58},
		{StageID: "q.6", Catalog: "mysql", Schema: "platform", Table: "user_membership", ConnectorType: "mysql", PhysicalInputPositions: 412, OutputRows: 5},
		{StageID: "q.7", Catalog: "mysql", Schema: "platform", Table: "user_membership", ConnectorType: "mysql", PhysicalInputPositions: 47, OutputRows: 0},
		{StageID: "q.8", Catalog: "mysql", Schema: "platform", Table: "user_membership", ConnectorType: "mysql", PhysicalInputPositions: 69, OutputRows: 69},
	}
	finding := DuplicateFederatedScans{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for 4 duplicate scans")
	}
	if !strings.Contains(finding.Title, "user_membership") {
		t.Errorf("title should mention table, got %q", finding.Title)
	}
	if !strings.Contains(finding.Title, "4 times") {
		t.Errorf("title should mention scan count, got %q", finding.Title)
	}
	if !strings.Contains(finding.Details, "q.4") || !strings.Contains(finding.Details, "q.8") {
		t.Errorf("details should list stage IDs, got %q", finding.Details)
	}
}

func TestDuplicateFederatedScans_SingleScan(t *testing.T) {
	f := baseFacts()
	f.ScanPushdown = []queryinfo.ScanPushdownFact{
		{StageID: "q.4", Catalog: "mysql", Schema: "platform", Table: "user_membership", ConnectorType: "mysql"},
	}
	r := DuplicateFederatedScans{}
	if r.Eval(f) != nil {
		t.Error("should not fire for a single scan")
	}
}

func TestDuplicateFederatedScans_LocalConnectorIgnored(t *testing.T) {
	f := baseFacts()
	// memory / tpch / tpcds connectors are cheap; duplicate scans there don't matter.
	f.ScanPushdown = []queryinfo.ScanPushdownFact{
		{StageID: "q.1", Catalog: "memory", Schema: "default", Table: "t", ConnectorType: "memory"},
		{StageID: "q.2", Catalog: "memory", Schema: "default", Table: "t", ConnectorType: "memory"},
		{StageID: "q.3", Catalog: "memory", Schema: "default", Table: "t", ConnectorType: "memory"},
	}
	r := DuplicateFederatedScans{}
	if r.Eval(f) != nil {
		t.Error("should not fire for local connectors like memory")
	}
}

func TestDuplicateFederatedScans_DifferentTables(t *testing.T) {
	f := baseFacts()
	f.ScanPushdown = []queryinfo.ScanPushdownFact{
		{StageID: "q.1", Catalog: "mysql", Schema: "platform", Table: "user_membership", ConnectorType: "mysql"},
		{StageID: "q.2", Catalog: "mysql", Schema: "platform", Table: "user_credits", ConnectorType: "mysql"},
	}
	r := DuplicateFederatedScans{}
	if r.Eval(f) != nil {
		t.Error("should not fire when scans target different tables")
	}
}

// ----- DivergentScanRowcounts -----

func TestDivergentScanRowcounts_Fires(t *testing.T) {
	f := baseFacts()
	// The famous user_membership pattern: 8999 / 412 / 47 / 69
	// max/min = 8999/47 = ~191× — well above the 10× threshold.
	f.ScanPushdown = []queryinfo.ScanPushdownFact{
		{StageID: "q.4", Catalog: "mysql", Schema: "platform", Table: "user_membership", ConnectorType: "mysql", PhysicalInputPositions: 8999, OutputRows: 58},
		{StageID: "q.6", Catalog: "mysql", Schema: "platform", Table: "user_membership", ConnectorType: "mysql", PhysicalInputPositions: 412, OutputRows: 5},
		{StageID: "q.7", Catalog: "mysql", Schema: "platform", Table: "user_membership", ConnectorType: "mysql", PhysicalInputPositions: 47, OutputRows: 0},
		{StageID: "q.8", Catalog: "mysql", Schema: "platform", Table: "user_membership", ConnectorType: "mysql", PhysicalInputPositions: 69, OutputRows: 69},
	}
	finding := DivergentScanRowcounts{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for divergent row counts")
	}
	if !strings.Contains(finding.Title, "user_membership") {
		t.Errorf("title should mention table, got %q", finding.Title)
	}
	if !strings.Contains(finding.Title, "spread") {
		t.Errorf("title should mention row-count spread, got %q", finding.Title)
	}
}

func TestDivergentScanRowcounts_SimilarCounts(t *testing.T) {
	f := baseFacts()
	f.ScanPushdown = []queryinfo.ScanPushdownFact{
		{StageID: "q.1", Catalog: "mysql", Table: "t", PhysicalInputPositions: 1000},
		{StageID: "q.2", Catalog: "mysql", Table: "t", PhysicalInputPositions: 1100},
	}
	r := DivergentScanRowcounts{}
	if r.Eval(f) != nil {
		t.Error("should not fire for similar row counts (< 10× spread)")
	}
}

func TestDivergentScanRowcounts_SingleScan(t *testing.T) {
	f := baseFacts()
	f.ScanPushdown = []queryinfo.ScanPushdownFact{
		{StageID: "q.1", Catalog: "mysql", Table: "t", PhysicalInputPositions: 1000},
	}
	r := DivergentScanRowcounts{}
	if r.Eval(f) != nil {
		t.Error("should not fire for a single scan")
	}
}

func TestDivergentScanRowcounts_AllSmall(t *testing.T) {
	f := baseFacts()
	f.ScanPushdown = []queryinfo.ScanPushdownFact{
		{StageID: "q.1", Catalog: "mysql", Table: "t", PhysicalInputPositions: 5},
		{StageID: "q.2", Catalog: "mysql", Table: "t", PhysicalInputPositions: 50},
	}
	r := DivergentScanRowcounts{}
	if r.Eval(f) != nil {
		t.Error("should not fire when all scans are tiny (noise)")
	}
}

// ----- Engine integration: rules don't break baseFacts -----

func TestDefaultEngine_NewRulesQuiet_OnHealthyQuery(t *testing.T) {
	// baseFacts() has no ScanPushdown — none of the new rules should fire.
	// This guards against accidentally regressing the healthy-baseline test.
	f := baseFacts()
	findings := DefaultEngine().Run(f)
	for _, fnd := range findings {
		switch fnd.RuleID {
		case "trino.local-filter-dominates",
			"trino.duplicate-federated-scans",
			"trino.divergent-scan-rowcounts":
			t.Errorf("new rule %q fired on baseline healthy query, got %+v", fnd.RuleID, fnd)
		}
	}
}
