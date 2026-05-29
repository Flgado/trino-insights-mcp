package rules

import (
	"strings"
	"testing"

	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

// slowEmptyScanFacts returns a query with one MongoDB scan that returned 0 rows
// from 0 physical input but its stage spent 1,500 ms blocked on the connector.
// Tweak the returned struct in each test to flip the conditions being checked.
func slowEmptyScanFacts() *queryinfo.QueryFacts {
	stageID := "20260419_080123_00042_abcde.5"
	return &queryinfo.QueryFacts{
		QueryID: "20260419_080123_00042_abcde",
		State:   "FINISHED",
		Stages: []queryinfo.StageFact{
			{
				StageID:          stageID,
				State:            "FINISHED",
				TotalCPUMs:       30,
				TotalScheduledMs: 1530,
				TotalBlockedMs:   1500,
				IOWaitMs:         1500,
				IOWaitKind:       "scan-roundtrip",
				TaskCount:        1,
			},
		},
		ScanPushdown: []queryinfo.ScanPushdownFact{
			{
				StageID:                 stageID,
				NodeName:                "TableScan",
				Catalog:                 "app_documents",
				Schema:                  "members",
				Table:                   "user_membership",
				ConnectorType:           "mongodb",
				PhysicalInputPositions:  0,
				OutputRows:              0,
				PushedConstraintColumns: []string{"branch_id", "user_membership_id"},
			},
		},
	}
}

func TestSlowEmptyScan_Fires_MongoEmptyResult(t *testing.T) {
	f := slowEmptyScanFacts()

	finding := SlowEmptyScan{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for slow empty Mongo scan")
	}
	if finding.RuleID != "trino.slow-empty-scan" {
		t.Errorf("got rule_id %q, want trino.slow-empty-scan", finding.RuleID)
	}
	if !strings.Contains(finding.Title, "Empty scan") {
		t.Errorf("expected 'Empty scan' in title, got %q", finding.Title)
	}
	if !strings.Contains(finding.Title, "user_membership") {
		t.Errorf("expected table in title, got %q", finding.Title)
	}
	if !strings.Contains(finding.Details, "1500 ms") {
		t.Errorf("expected wait duration in details, got %q", finding.Details)
	}
	if !strings.Contains(finding.Remediation, "explain") {
		t.Errorf("expected MongoDB explain() pointer in remediation, got %q", finding.Remediation)
	}
}

func TestSlowEmptyScan_NotFired_FastEmptyScan(t *testing.T) {
	f := slowEmptyScanFacts()
	f.Stages[0].TotalScheduledMs = 80
	f.Stages[0].TotalBlockedMs = 50
	f.Stages[0].IOWaitMs = 50

	if (SlowEmptyScan{}).Eval(f) != nil {
		t.Error("should not fire when stage wait < 500 ms")
	}
}

func TestSlowEmptyScan_NotFired_NonEmptyScan(t *testing.T) {
	f := slowEmptyScanFacts()
	f.ScanPushdown[0].PhysicalInputPositions = 1000
	f.ScanPushdown[0].OutputRows = 1000

	if (SlowEmptyScan{}).Eval(f) != nil {
		t.Error("should not fire when scan returned rows")
	}
}

func TestSlowEmptyScan_NotFired_NoScanPushdown(t *testing.T) {
	f := slowEmptyScanFacts()
	f.ScanPushdown = nil

	if (SlowEmptyScan{}).Eval(f) != nil {
		t.Error("should not fire when there are no scan_pushdown entries")
	}
}

func TestSlowEmptyScan_FallbackWait_FromScheduledMinusCPU(t *testing.T) {
	f := slowEmptyScanFacts()
	// Wipe IOWaitMs so the rule has to fall back to scheduled - cpu.
	f.Stages[0].IOWaitMs = 0
	f.Stages[0].IOWaitKind = ""
	f.Stages[0].TotalCPUMs = 40
	f.Stages[0].TotalScheduledMs = 1340

	finding := SlowEmptyScan{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding when wait is derived from scheduled - cpu")
	}
	if !strings.Contains(finding.Details, "1300 ms") {
		t.Errorf("expected fallback wait duration in details, got %q", finding.Details)
	}
}

func TestSlowEmptyScan_MultipleHits_SortedByWait(t *testing.T) {
	f := slowEmptyScanFacts()
	otherStageID := "20260419_080123_00042_abcde.9"
	f.Stages = append(f.Stages, queryinfo.StageFact{
		StageID:          otherStageID,
		State:            "FINISHED",
		TotalCPUMs:       20,
		TotalScheduledMs: 4020,
		TotalBlockedMs:   4000,
		IOWaitMs:         4000,
		IOWaitKind:       "scan-roundtrip",
		TaskCount:        1,
	})
	f.ScanPushdown = append(f.ScanPushdown, queryinfo.ScanPushdownFact{
		StageID:                otherStageID,
		NodeName:               "TableScan",
		Catalog:                "app_documents",
		Schema:                 "members",
		Table:                  "membership_history",
		ConnectorType:          "mongodb",
		PhysicalInputPositions: 0,
		OutputRows:             0,
	})

	finding := SlowEmptyScan{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for multi-hit case")
	}
	ev, ok := finding.Evidence.(map[string]any)
	if !ok {
		t.Fatalf("expected Evidence to be map[string]any, got %T", finding.Evidence)
	}
	if got, want := ev["scans_matched"], 2; got != want {
		t.Errorf("expected scans_matched=%d, got %v", want, got)
	}
	scans, ok := ev["scans"].([]map[string]any)
	if !ok || len(scans) < 2 {
		t.Fatalf("expected scans evidence with >= 2 entries, got %T %v", ev["scans"], ev["scans"])
	}
	if scans[0]["table"] != "app_documents.members.membership_history" {
		t.Errorf("expected slowest scan first; got %q", scans[0]["table"])
	}
}

func TestSlowEmptyScan_RespectsCustomMinWait(t *testing.T) {
	f := slowEmptyScanFacts()
	f.Stages[0].IOWaitMs = 200

	if (SlowEmptyScan{}).Eval(f) != nil {
		t.Error("default 500 ms floor should suppress 200 ms wait")
	}
	if (SlowEmptyScan{MinWaitMs: 100}).Eval(f) == nil {
		t.Error("custom 100 ms floor should allow 200 ms wait to fire")
	}
}
