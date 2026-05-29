package queryinfo

import (
	"reflect"
	"strings"
	"testing"
)

func TestDeriveIOWait_PhysicalReadTakesPriority(t *testing.T) {
	sf := StageFact{
		PhysicalInputReadTimeMs: 512,
		TotalScheduledMs:        528,
		TotalCPUMs:              16,
		TotalBlockedMs:          0,
		PhysicalInputPos:        135736,
	}
	got, kind := deriveIOWait(sf)
	if got != 512 {
		t.Errorf("io_wait = %d, want 512 (physical_read)", got)
	}
	if kind != IOWaitKindPhysicalRead {
		t.Errorf("kind = %q, want %q", kind, IOWaitKindPhysicalRead)
	}
}

func TestDeriveIOWait_FallsBackToScheduledMinusCPU(t *testing.T) {
	// physicalInputReadTime missing (e.g. JDBC connector on older Trino) —
	// derive from scheduled - cpu for scan stages.
	sf := StageFact{
		PhysicalInputReadTimeMs: 0,
		TotalScheduledMs:        77,
		TotalCPUMs:              5,
		TotalBlockedMs:          101,
		PhysicalInputPos:        4750,
	}
	got, kind := deriveIOWait(sf)
	if got != 72 {
		t.Errorf("io_wait = %d, want 72 (scheduled_minus_cpu)", got)
	}
	if kind != IOWaitKindScheduledMinusCPU {
		t.Errorf("kind = %q, want %q", kind, IOWaitKindScheduledMinusCPU)
	}
}

func TestDeriveIOWait_DownstreamUsesBlocked(t *testing.T) {
	// Aggregation / join stage with no physical inputs — its "I/O wait" is
	// really the blocked time waiting on upstream rows.
	sf := StageFact{
		TotalScheduledMs: 3,
		TotalCPUMs:       3,
		TotalBlockedMs:   24570,
		PhysicalInputPos: 0,
	}
	got, kind := deriveIOWait(sf)
	if got != 24570 {
		t.Errorf("io_wait = %d, want 24570 (blocked)", got)
	}
	if kind != IOWaitKindBlocked {
		t.Errorf("kind = %q, want %q", kind, IOWaitKindBlocked)
	}
}

func TestDeriveIOWait_ScheduledMinusCPUClampedToZero(t *testing.T) {
	// CPU can exceed scheduled in rare cases (separate accumulators on
	// different code paths); don't return a negative I/O wait.
	sf := StageFact{
		TotalScheduledMs: 5,
		TotalCPUMs:       10,
		PhysicalInputPos: 100,
	}
	got, _ := deriveIOWait(sf)
	if got != 0 {
		t.Errorf("io_wait = %d, want 0 (clamped)", got)
	}
}

func TestBuildConnectorIORollup_SortsByIOWaitDesc(t *testing.T) {
	stages := []StageFact{
		{StageID: "q.1", IOWaitMs: 80, PhysicalInputBytes: 1000},
		{StageID: "q.2", IOWaitMs: 512, PhysicalInputBytes: 1612035},
		{StageID: "q.3", IOWaitMs: 1781, PhysicalInputBytes: 0},
		{StageID: "q.4", IOWaitMs: 1789, PhysicalInputBytes: 0},
	}
	scans := []ScanPushdownFact{
		{StageID: "q.1", ConnectorType: "postgresql", PhysicalInputPositions: 346, OutputRows: 346},
		{StageID: "q.2", ConnectorType: "iceberg", PhysicalInputPositions: 135736, OutputRows: 848},
		{StageID: "q.3", ConnectorType: "mongodb", PhysicalInputPositions: 81, OutputRows: 9},
		{StageID: "q.4", ConnectorType: "mongodb", PhysicalInputPositions: 81, OutputRows: 0},
	}
	out := buildConnectorIORollup(stages, scans)
	if len(out) != 3 {
		t.Fatalf("expected 3 connectors, got %d", len(out))
	}
	// Mongo (1781+1789=3570) > Iceberg (512) > PostgreSQL (80)
	if out[0].ConnectorType != "mongodb" || out[0].IOWaitMs != 3570 {
		t.Errorf("first = %+v, want mongodb 3570", out[0])
	}
	if out[1].ConnectorType != "iceberg" || out[1].IOWaitMs != 512 {
		t.Errorf("second = %+v, want iceberg 512", out[1])
	}
	if out[2].ConnectorType != "postgresql" || out[2].IOWaitMs != 80 {
		t.Errorf("third = %+v, want postgresql 80", out[2])
	}

	// Row totals are summed across all scans of the same connector.
	if out[0].ScanCount != 2 || out[0].RowsIn != 162 || out[0].RowsOut != 9 {
		t.Errorf("mongo aggregate wrong: %+v", out[0])
	}
	if out[1].BytesIn != 1612035 {
		t.Errorf("iceberg bytes_in = %d, want 1612035", out[1].BytesIn)
	}
}

func TestBuildConnectorIORollup_DedupesStageContribution(t *testing.T) {
	// Same stage scanning the same connector twice (rare — fused scans).
	// Each scan increments scan_count + rows but the stage's IOWaitMs is
	// only charged once.
	stages := []StageFact{
		{StageID: "q.1", IOWaitMs: 1000, PhysicalInputBytes: 5000},
	}
	scans := []ScanPushdownFact{
		{StageID: "q.1", ConnectorType: "hive", PhysicalInputPositions: 100, OutputRows: 100},
		{StageID: "q.1", ConnectorType: "hive", PhysicalInputPositions: 200, OutputRows: 50},
	}
	out := buildConnectorIORollup(stages, scans)
	if len(out) != 1 {
		t.Fatalf("expected 1 connector, got %d", len(out))
	}
	if out[0].ScanCount != 2 {
		t.Errorf("scan_count = %d, want 2", out[0].ScanCount)
	}
	if out[0].IOWaitMs != 1000 {
		t.Errorf("io_wait_ms = %d, want 1000 (deduped)", out[0].IOWaitMs)
	}
	if out[0].RowsIn != 300 {
		t.Errorf("rows_in = %d, want 300", out[0].RowsIn)
	}
}

func TestBuildConnectorIORollup_EmptyScans(t *testing.T) {
	out := buildConnectorIORollup([]StageFact{{StageID: "q.0", IOWaitMs: 100}}, nil)
	if out != nil {
		t.Errorf("expected nil when no scans, got %+v", out)
	}
}

func TestBuildConnectorIORollup_SkipsEmptyConnectorType(t *testing.T) {
	stages := []StageFact{{StageID: "q.1", IOWaitMs: 100}}
	scans := []ScanPushdownFact{
		{StageID: "q.1", ConnectorType: "", PhysicalInputPositions: 10},
	}
	out := buildConnectorIORollup(stages, scans)
	if len(out) != 0 {
		t.Errorf("expected 0 connectors when type is empty, got %d", len(out))
	}
}

func TestBuildTopIOStages_RanksAndCaps(t *testing.T) {
	stages := []StageFact{
		{StageID: "q.0", IOWaitMs: 248, TotalBlockedMs: 248, PrimaryOperator: "ExchangeOperator", SubStageIDs: []string{"q.1"}},
		{StageID: "q.1", IOWaitMs: 72, TotalCPUMs: 5, PhysicalInputPos: 4750},
		{StageID: "q.2", IOWaitMs: 8, TotalCPUMs: 5, PhysicalInputPos: 7219},
	}
	scans := []ScanPushdownFact{
		{StageID: "q.1", ConnectorType: "mysql", Catalog: "app_reporting", Schema: "app", Table: "tasks", PhysicalInputPositions: 4750, OutputRows: 4750, LocalFilter: "(COALESCE((active = tinyint '1'), boolean 'false') = boolean 'true')"},
		{StageID: "q.2", ConnectorType: "mysql", Catalog: "app_reporting", Schema: "app", Table: "members", PhysicalInputPositions: 7219, OutputRows: 7219},
	}
	out := buildTopIOStages(stages, scans, 2)
	if len(out) != 2 {
		t.Fatalf("expected 2 top stages (capped), got %d", len(out))
	}
	if out[0].StageID != "q.0" || out[0].IOWaitMs != 248 {
		t.Errorf("first = %+v, want q.0 / 248", out[0])
	}
	if out[1].StageID != "q.1" || out[1].IOWaitMs != 72 {
		t.Errorf("second = %+v, want q.1 / 72", out[1])
	}
	// Scan stage has connector + table + scan-flavoured rationale
	if out[1].ConnectorType != "mysql" || out[1].Table != "app_reporting.app.tasks" {
		t.Errorf("scan stage metadata wrong: %+v", out[1])
	}
	if !strings.Contains(out[1].Rationale, "read 4750") || !strings.Contains(out[1].Rationale, "local filter") {
		t.Errorf("scan rationale = %q", out[1].Rationale)
	}
	// Downstream stage has substage-based rationale
	if !strings.Contains(out[0].Rationale, "blocked on") || !strings.Contains(out[0].Rationale, ".1") {
		t.Errorf("downstream rationale = %q", out[0].Rationale)
	}
}

func TestBuildTopIOStages_ExcludesZeroes(t *testing.T) {
	stages := []StageFact{
		{StageID: "q.0", IOWaitMs: 100},
		{StageID: "q.1", IOWaitMs: 0},
		{StageID: "q.2", IOWaitMs: 50},
	}
	out := buildTopIOStages(stages, nil, 5)
	ids := make([]string, len(out))
	for i, ts := range out {
		ids[i] = ts.StageID
	}
	if !reflect.DeepEqual(ids, []string{"q.0", "q.2"}) {
		t.Errorf("ids = %v, want [q.0 q.2]", ids)
	}
}

func TestBuildTopCPUStages_RanksByCPU(t *testing.T) {
	stages := []StageFact{
		{StageID: "q.0", TotalCPUMs: 0},
		{StageID: "q.1", TotalCPUMs: 16, IOWaitMs: 512, PhysicalInputPos: 100},
		{StageID: "q.2", TotalCPUMs: 9, IOWaitMs: 1781, PhysicalInputPos: 100},
	}
	out := buildTopCPUStages(stages, nil, 5)
	if len(out) != 2 {
		t.Fatalf("expected 2 entries (q.0 excluded), got %d", len(out))
	}
	if out[0].StageID != "q.1" || out[0].CPUMs != 16 {
		t.Errorf("first = %+v, want q.1 / 16", out[0])
	}
	if out[1].StageID != "q.2" || out[1].CPUMs != 9 {
		t.Errorf("second = %+v, want q.2 / 9", out[1])
	}
}

func TestGenerateScanRationale_LocalFilterTruncated(t *testing.T) {
	long := strings.Repeat("a", 200)
	sf := StageFact{}
	scan := ScanPushdownFact{
		PhysicalInputPositions: 100,
		OutputRows:             100,
		LocalFilter:            long,
		ConnectorType:          "mysql",
	}
	r := generateScanRationale(sf, scan, true)
	if !strings.Contains(r, "…") {
		t.Errorf("expected truncation indicator, got %q", r)
	}
	if len(r) > 120 { // ~80 chars filter + boilerplate
		t.Errorf("rationale too long (%d chars): %q", len(r), r)
	}
}

func TestGenerateDownstreamRationale_NoSubstages(t *testing.T) {
	sf := StageFact{PrimaryOperator: "AggregationOperator"}
	r := generateDownstreamRationale(sf, true)
	if !strings.Contains(r, "no upstream") {
		t.Errorf("expected 'no upstream' fallback, got %q", r)
	}
}

func TestProjectEndToEnd_PopulatesNewFacts(t *testing.T) {
	// Sanity check that Project() actually populates ConnectorIO + TopIOStages
	// + StageFact.IOWaitMs end-to-end when given a realistic QueryInfo shape.
	// We use the nested outputStage path so the flattener exercises too.
	qi := &QueryInfo{
		QueryID: "q1",
		State:   "FINISHED",
		QueryStats: QueryStats{
			TotalCPUTime:           "65ms",
			TotalScheduledTime:     "4.42s",
			TotalBlockedTime:       "95.4s",
			PhysicalInputReadTime:  "4.35s",
			PhysicalInputDataSize:  "1.5MB",
			PhysicalInputPositions: 135736,
		},
		Inputs: []InputRef{
			{CatalogName: "app_lakehouse", Schema: "dbt_trusted", Table: "agg_user_total_visits_v2"},
		},
	}
	facts := Project(qi)
	if facts == nil {
		t.Fatal("facts should not be nil")
	}
	if facts.IO.PhysicalInputReadTimeMs != 4350 {
		t.Errorf("query-level PhysicalInputReadTimeMs = %d, want 4350", facts.IO.PhysicalInputReadTimeMs)
	}
}
