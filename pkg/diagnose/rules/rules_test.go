package rules

import (
	"strings"
	"testing"

	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

// baseFacts returns a healthy FINISHED query that triggers no rules.
// Includes operators with pipeline ordering, amplification, tables, and per-task stats.
func baseFacts() *queryinfo.QueryFacts {
	return &queryinfo.QueryFacts{
		QueryID: "20260419_080123_00042_abcde",
		State:   "FINISHED",
		Session: queryinfo.SessionFacts{User: "test-user"},
		Tables: []queryinfo.TableFact{
			{FullName: "hive.production.lineitem", Catalog: "hive", Schema: "production", Table: "lineitem", ConnectorType: "hive"},
			{FullName: "hive.production.orders", Catalog: "hive", Schema: "production", Table: "orders", ConnectorType: "hive"},
		},
		Time: queryinfo.TimeFacts{
			ElapsedMs:        5000,
			QueuedMs:         100,
			PlanningMs:       200,
			ExecutionMs:      4700,
			TotalCPUMs:       3000,
			TotalScheduledMs: 10000,
			TotalBlockedMs:   500,
		},
		Memory: queryinfo.MemoryFacts{
			PeakUserMemoryBytes:  100 * 1024 * 1024,
			PeakTotalMemoryBytes: 200 * 1024 * 1024,
			PeakTaskUserMemBytes: 50 * 1024 * 1024,
			SpilledBytes:         0,
		},
		IO: queryinfo.IOFacts{
			PhysicalInputBytes:     1 * 1024 * 1024 * 1024,
			PhysicalInputPositions: 10_000_000,
			ProcessedInputBytes:    1 * 1024 * 1024 * 1024,
			ProcessedInputPos:      10_000_000,
			OutputBytes:            1024 * 1024,
			OutputPositions:        10_000,
		},
		Tasks: queryinfo.TaskFacts{
			TotalTasks:     10,
			CompletedTasks: 10,
			TotalDrivers:   20,
		},
		Stages: []queryinfo.StageFact{
			{
				StageID:         "20260419_080123_00042_abcde.0",
				State:           "FINISHED",
				TotalCPUMs:      1500,
				PlanSummary:     "Output -> ScanFilterProject[lineitem]",
				SubStageIDs:     []string{"20260419_080123_00042_abcde.1"},
				PrimaryOperator: "ScanFilterProjectOperator",
				Operators: []queryinfo.OperatorFact{
					{OperatorType: "ScanFilterProjectOperator", CPUMs: 1400, InputRows: 5_000_000, OutputRows: 5_000_000, Amplification: 1.0},
				},
				TaskCount:    4,
				MaxTaskCPUMs: 400,
				MinTaskCPUMs: 350,
				P50TaskCPUMs: 375,
			},
			{
				StageID:         "20260419_080123_00042_abcde.1",
				State:           "FINISHED",
				TotalCPUMs:      1500,
				PlanSummary:     "HashAggregation -> ScanFilterProject[orders]",
				PrimaryOperator: "HashAggregationOperator",
				Operators: []queryinfo.OperatorFact{
					{OperatorType: "ScanFilterProjectOperator", CPUMs: 200, InputRows: 5_000_000, OutputRows: 5_000_000, Amplification: 1.0},
					{OperatorType: "HashAggregationOperator", CPUMs: 1300, InputRows: 5_000_000, OutputRows: 100_000, Amplification: 0.02},
				},
				TaskCount:    4,
				MaxTaskCPUMs: 400,
				MinTaskCPUMs: 350,
				P50TaskCPUMs: 375,
			},
		},
	}
}

func TestFailed_Fires(t *testing.T) {
	f := baseFacts()
	f.State = "FAILED"
	f.ErrorType = "USER_ERROR"
	f.ErrorCodeName = "SYNTAX_ERROR"

	finding := Failed{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for FAILED query")
	}
	if finding.RuleID != "trino.failed" {
		t.Errorf("got rule_id %q, want trino.failed", finding.RuleID)
	}
	if finding.Severity != "critical" {
		t.Errorf("got severity %q, want critical", finding.Severity)
	}
}

func TestFailed_NotFired(t *testing.T) {
	f := baseFacts()
	r := Failed{}
	if r.Eval(f) != nil {
		t.Error("should not fire for FINISHED query")
	}
}

func TestCPUBound_Fires(t *testing.T) {
	f := baseFacts()
	f.Time.TotalCPUMs = 9000
	f.Time.TotalScheduledMs = 10000

	finding := CPUBound{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for high CPU ratio")
	}
	if finding.RuleID != "trino.cpu-bound" {
		t.Errorf("got rule_id %q", finding.RuleID)
	}
}

func TestCPUBound_NotFired(t *testing.T) {
	f := baseFacts()
	f.Time.TotalCPUMs = 3000
	f.Time.TotalScheduledMs = 10000

	r := CPUBound{}
	if r.Eval(f) != nil {
		t.Error("should not fire for low CPU ratio")
	}
}

func TestMemoryPressure_Fires(t *testing.T) {
	f := baseFacts()
	f.Memory.PeakTaskUserMemBytes = 2 * (1 << 30)

	finding := MemoryPressure{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for high per-task memory")
	}
	if finding.RuleID != "trino.memory-pressure" {
		t.Errorf("got rule_id %q", finding.RuleID)
	}
}

func TestMemoryPressure_NotFired(t *testing.T) {
	f := baseFacts()
	f.Memory.PeakTaskUserMemBytes = 50 * 1024 * 1024

	r := MemoryPressure{}
	if r.Eval(f) != nil {
		t.Error("should not fire for low per-task memory")
	}
}

func TestDiskSpill_Fires(t *testing.T) {
	f := baseFacts()
	f.Memory.SpilledBytes = 500 * 1024 * 1024

	finding := DiskSpill{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for disk spill")
	}
	if finding.RuleID != "trino.disk-spill" {
		t.Errorf("got rule_id %q", finding.RuleID)
	}
}

func TestDiskSpill_NotFired(t *testing.T) {
	f := baseFacts()
	r := DiskSpill{}
	if r.Eval(f) != nil {
		t.Error("should not fire when no spill")
	}
}

func TestDiskSpill_IdentifiesOperator(t *testing.T) {
	f := baseFacts()
	f.Memory.SpilledBytes = 500 * 1024 * 1024
	f.Stages[1].Operators = []queryinfo.OperatorFact{
		{OperatorType: "HashAggregationOperator", CPUMs: 1300, SpilledBytes: 500 * 1024 * 1024},
	}

	finding := DiskSpill{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for disk spill")
	}
	if !strings.Contains(finding.Title, "HashAggregationOperator") {
		t.Errorf("expected title to mention operator, got %q", finding.Title)
	}
	ev := finding.Evidence.(map[string]any)
	if ev["spill_operator"] != "HashAggregationOperator" {
		t.Errorf("expected spill_operator evidence, got %v", ev["spill_operator"])
	}
}

func TestQueuedTooLong_Fires(t *testing.T) {
	f := baseFacts()
	f.Time.QueuedMs = 4000
	f.Time.ElapsedMs = 10000

	finding := QueuedTooLong{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for long queue time")
	}
	if finding.RuleID != "trino.queued-too-long" {
		t.Errorf("got rule_id %q", finding.RuleID)
	}
}

func TestQueuedTooLong_NotFired(t *testing.T) {
	f := baseFacts()
	f.Time.QueuedMs = 100
	f.Time.ElapsedMs = 10000

	r := QueuedTooLong{}
	if r.Eval(f) != nil {
		t.Error("should not fire for short queue time")
	}
}

func TestStageSkew_TaskSkew_Fires(t *testing.T) {
	f := baseFacts()
	f.Stages = []queryinfo.StageFact{
		{
			StageID:         "20260419_080123_00042_abcde.0",
			TotalCPUMs:      5000,
			PrimaryOperator: "HashJoinOperator",
			TaskCount:       4,
			MaxTaskCPUMs:    4000,
			MinTaskCPUMs:    100,
			P50TaskCPUMs:    200,
		},
		{
			StageID:    "20260419_080123_00042_abcde.1",
			TotalCPUMs: 1000,
			TaskCount:  2,
		},
	}

	finding := StageSkew{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for per-task skew")
	}
	if finding.RuleID != "trino.stage-skew" {
		t.Errorf("got rule_id %q", finding.RuleID)
	}
	if !strings.Contains(finding.Title, "Per-task skew") {
		t.Errorf("expected per-task skew title, got %q", finding.Title)
	}
	if !strings.Contains(finding.Title, "HashJoinOperator") {
		t.Errorf("expected operator in title, got %q", finding.Title)
	}
	ev := finding.Evidence.(map[string]any)
	if ev["task_count"] != 4 {
		t.Errorf("expected task_count=4, got %v", ev["task_count"])
	}
}

func TestStageSkew_Fallback_Fires(t *testing.T) {
	f := baseFacts()
	f.Stages = []queryinfo.StageFact{
		{StageID: "0", TotalCPUMs: 1, TaskCount: 1},
		{StageID: "1", TotalCPUMs: 1, TaskCount: 1},
		{StageID: "2", TotalCPUMs: 1, TaskCount: 1},
		{StageID: "3", TotalCPUMs: 1, TaskCount: 1},
		{StageID: "4", TotalCPUMs: 1, TaskCount: 1},
		{StageID: "5", TotalCPUMs: 1, TaskCount: 1},
		{StageID: "6", TotalCPUMs: 1, TaskCount: 1},
		{StageID: "7", TotalCPUMs: 1, TaskCount: 1},
		{StageID: "8", TotalCPUMs: 1, TaskCount: 1},
		{StageID: "hot", TotalCPUMs: 100000, TaskCount: 1},
	}

	finding := StageSkew{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for stage-level skew fallback")
	}
	if !strings.Contains(finding.Title, "Stage-level skew") {
		t.Errorf("expected stage-level skew title, got %q", finding.Title)
	}
}

func TestStageSkew_NotFired(t *testing.T) {
	f := baseFacts()
	f.Stages = []queryinfo.StageFact{
		{StageID: "0", TotalCPUMs: 1000, TaskCount: 2, MaxTaskCPUMs: 550, P50TaskCPUMs: 450},
		{StageID: "1", TotalCPUMs: 1200, TaskCount: 2, MaxTaskCPUMs: 650, P50TaskCPUMs: 550},
	}

	r := StageSkew{}
	if r.Eval(f) != nil {
		t.Error("should not fire for balanced stages and tasks")
	}
}

func TestHotspotStage_Fires(t *testing.T) {
	f := baseFacts()
	f.Stages = []queryinfo.StageFact{
		{StageID: "20260419_080123_00042_abcde.0", TotalCPUMs: 200},
		{StageID: "20260419_080123_00042_abcde.1", TotalCPUMs: 9800, PrimaryOperator: "OrderByOperator"},
	}

	finding := HotspotStage{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for hotspot stage")
	}
	if finding.RuleID != "trino.hotspot-stage" {
		t.Errorf("got rule_id %q", finding.RuleID)
	}
	if !strings.Contains(finding.Title, "OrderByOperator") {
		t.Errorf("expected operator in title, got %q", finding.Title)
	}
	ev := finding.Evidence.(map[string]any)
	if ev["primary_operator"] != "OrderByOperator" {
		t.Errorf("expected primary_operator in evidence, got %v", ev["primary_operator"])
	}
}

func TestHotspotStage_NotFired(t *testing.T) {
	f := baseFacts()
	r := HotspotStage{}
	if r.Eval(f) != nil {
		t.Error("should not fire for balanced stages")
	}
}

func TestScanTooLarge_Fires_Rows(t *testing.T) {
	f := baseFacts()
	f.IO.PhysicalInputPositions = 2_000_000_000

	finding := ScanTooLarge{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for large scan by rows")
	}
	if finding.RuleID != "trino.scan-too-large" {
		t.Errorf("got rule_id %q", finding.RuleID)
	}
}

func TestScanTooLarge_Fires_Bytes(t *testing.T) {
	f := baseFacts()
	f.IO.PhysicalInputBytes = 200 * (1 << 30)

	finding := ScanTooLarge{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for large scan by bytes")
	}
}

func TestScanTooLarge_NotFired(t *testing.T) {
	f := baseFacts()
	r := ScanTooLarge{}
	if r.Eval(f) != nil {
		t.Error("should not fire for normal scan")
	}
}

func TestPoorSelectivity_Fires(t *testing.T) {
	f := baseFacts()
	f.IO.ProcessedInputPos = 100_000_000
	f.IO.OutputPositions = 5

	finding := PoorSelectivity{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for poor selectivity")
	}
	if finding.RuleID != "trino.poor-selectivity" {
		t.Errorf("got rule_id %q", finding.RuleID)
	}
}

func TestPoorSelectivity_NotFired(t *testing.T) {
	f := baseFacts()
	r := PoorSelectivity{}
	if r.Eval(f) != nil {
		t.Error("should not fire for decent selectivity")
	}
}

func TestUnderParallelised_Fires(t *testing.T) {
	f := baseFacts()
	f.Time.ElapsedMs = 60000
	f.Tasks.TotalDrivers = 2

	finding := UnderParallelised{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for under-parallelised query")
	}
	if finding.RuleID != "trino.under-parallelised" {
		t.Errorf("got rule_id %q", finding.RuleID)
	}
}

func TestUnderParallelised_NotFired(t *testing.T) {
	f := baseFacts()
	r := UnderParallelised{}
	if r.Eval(f) != nil {
		t.Error("should not fire for well-parallelised query")
	}
}

func TestLongBlocked_Fires(t *testing.T) {
	f := baseFacts()
	f.Time.TotalBlockedMs = 6000
	f.Time.TotalScheduledMs = 10000

	finding := LongBlocked{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for long-blocked query")
	}
	if finding.RuleID != "trino.long-blocked" {
		t.Errorf("got rule_id %q", finding.RuleID)
	}
}

func TestLongBlocked_NotFired(t *testing.T) {
	f := baseFacts()
	r := LongBlocked{}
	if r.Eval(f) != nil {
		t.Error("should not fire for lightly blocked query")
	}
}

func TestLongBlocked_NotFired_BelowAbsoluteFloor(t *testing.T) {
	// 600 ms blocked out of 1,000 ms scheduled = 60% (above 40% ratio)
	// but the absolute wait is only 600 ms — below the 2,000 ms floor.
	// This protects against false positives on sub-second queries.
	f := baseFacts()
	f.Time.TotalBlockedMs = 600
	f.Time.TotalScheduledMs = 1000

	if (LongBlocked{}).Eval(f) != nil {
		t.Error("should not fire when blocked < 2000 ms even with high ratio")
	}
}

func TestLongBlocked_RespectsCustomAbsoluteFloor(t *testing.T) {
	f := baseFacts()
	f.Time.TotalBlockedMs = 1500
	f.Time.TotalScheduledMs = 2500

	if (LongBlocked{}).Eval(f) != nil {
		t.Error("default 2000 ms floor should suppress 1500 ms blocked")
	}
	if (LongBlocked{MinBlockedMs: 500}).Eval(f) == nil {
		t.Error("custom 500 ms floor should allow 1500 ms blocked to fire")
	}
}

func TestPoorSelectivity_NotFired_OnSummaryAggregation(t *testing.T) {
	// COUNT(*) shape: aggregation operator emits 1 row from millions of input.
	// Selectivity is by design < 0.0001 but the rule MUST NOT flag it.
	f := baseFacts()
	f.IO.ProcessedInputPos = 100_000_000
	f.IO.OutputPositions = 1
	f.Stages = []queryinfo.StageFact{
		{
			StageID:         "20260419_080123_00042_abcde.0",
			TotalCPUMs:      2000,
			PrimaryOperator: "AggregationOperator",
			Operators: []queryinfo.OperatorFact{
				{OperatorType: "ScanFilterProjectOperator", InputRows: 100_000_000, OutputRows: 100_000_000},
				{OperatorType: "AggregationOperator", InputRows: 100_000_000, OutputRows: 1, Amplification: 0},
			},
		},
	}

	if (PoorSelectivity{}).Eval(f) != nil {
		t.Error("should not fire when an aggregation operator produces <= 100 rows (summary query)")
	}
}

func TestPoorSelectivity_NotFired_OnSmallGroupBy(t *testing.T) {
	f := baseFacts()
	f.IO.ProcessedInputPos = 100_000_000
	f.IO.OutputPositions = 50
	f.Stages = []queryinfo.StageFact{
		{
			StageID:         "20260419_080123_00042_abcde.0",
			TotalCPUMs:      2000,
			PrimaryOperator: "HashAggregationOperator",
			Operators: []queryinfo.OperatorFact{
				{OperatorType: "HashAggregationOperator", InputRows: 100_000_000, OutputRows: 50, Amplification: 0},
			},
		},
	}

	if (PoorSelectivity{}).Eval(f) != nil {
		t.Error("should not fire on GROUP BY producing <= 100 groups")
	}
}

func TestPoorSelectivity_Fires_LargeGroupByIsStillSuspicious(t *testing.T) {
	// 100M input -> 50K output via aggregation is 0.0005 selectivity (above
	// the 0.0001 threshold). Below threshold the rule needn't fire at all,
	// but we want to confirm the aggregation suppression doesn't accidentally
	// suppress legitimate cases. So drop the threshold below 0.0005.
	f := baseFacts()
	f.IO.ProcessedInputPos = 100_000_000
	f.IO.OutputPositions = 1000
	f.Stages = []queryinfo.StageFact{
		{
			StageID:         "20260419_080123_00042_abcde.0",
			TotalCPUMs:      2000,
			PrimaryOperator: "HashAggregationOperator",
			Operators: []queryinfo.OperatorFact{
				{OperatorType: "HashAggregationOperator", InputRows: 100_000_000, OutputRows: 1000, Amplification: 0},
			},
		},
	}

	// Output 1000 is well above the AggregationOutputFloor of 100, so we DO
	// want the rule to fire here — a 1000-group aggregation that reads 100M
	// rows is still worth flagging as poor selectivity.
	if (PoorSelectivity{}).Eval(f) == nil {
		t.Error("should fire when aggregation output is large (> 100 rows) even on aggregation queries")
	}
}

func TestRowExplosion_Fires(t *testing.T) {
	f := baseFacts()
	f.Stages = []queryinfo.StageFact{
		{
			StageID:         "20260419_080123_00042_abcde.0",
			TotalCPUMs:      3000,
			PrimaryOperator: "LookupJoinOperator",
			Operators: []queryinfo.OperatorFact{
				{OperatorType: "ScanFilterProjectOperator", CPUMs: 500, InputRows: 1_000_000, OutputRows: 1_000_000, Amplification: 1.0},
				{OperatorType: "LookupJoinOperator", CPUMs: 2500, InputRows: 1_000_000, OutputRows: 15_000_000, Amplification: 15.0},
			},
		},
		{
			StageID:    "20260419_080123_00042_abcde.1",
			TotalCPUMs: 500,
		},
	}

	finding := RowExplosion{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for row explosion")
	}
	if finding.RuleID != "trino.row-explosion" {
		t.Errorf("got rule_id %q", finding.RuleID)
	}
	if !strings.Contains(finding.Title, "LookupJoinOperator") {
		t.Errorf("expected operator in title, got %q", finding.Title)
	}
	ev := finding.Evidence.(map[string]any)
	if ev["amplification"] != 15.0 {
		t.Errorf("expected amplification=15.0, got %v", ev["amplification"])
	}
}

func TestRowExplosion_NotFired(t *testing.T) {
	f := baseFacts()
	r := RowExplosion{}
	if r.Eval(f) != nil {
		t.Error("should not fire for normal amplification")
	}
}

func TestRowExplosion_IgnoresSmallOperators(t *testing.T) {
	f := baseFacts()
	f.Stages = []queryinfo.StageFact{
		{
			StageID: "20260419_080123_00042_abcde.0",
			Operators: []queryinfo.OperatorFact{
				{OperatorType: "LookupJoinOperator", InputRows: 100, OutputRows: 5000, Amplification: 50.0},
			},
		},
		{StageID: "20260419_080123_00042_abcde.1"},
	}

	r := RowExplosion{}
	if r.Eval(f) != nil {
		t.Error("should not fire for operators with < 10000 input rows")
	}
}

func TestMissedPushdown_Fires(t *testing.T) {
	f := baseFacts()
	f.OptimizerRules = []queryinfo.OptimizerRuleFact{
		{Rule: "PushPredicateIntoTableScan", Invocations: 5, Applied: 0},
		{Rule: "RemoveRedundantIdentityProjections", Invocations: 10, Applied: 10},
	}

	finding := MissedPushdown{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for missed pushdown")
	}
	if finding.RuleID != "trino.missed-pushdown" {
		t.Errorf("got rule_id %q", finding.RuleID)
	}
	if !strings.Contains(finding.Details, "PushPredicateIntoTableScan") {
		t.Errorf("expected rule name in details, got %q", finding.Details)
	}
}

func TestMissedPushdown_NotFired_AllApplied(t *testing.T) {
	f := baseFacts()
	f.OptimizerRules = []queryinfo.OptimizerRuleFact{
		{Rule: "PushPredicateIntoTableScan", Invocations: 5, Applied: 3},
	}

	r := MissedPushdown{}
	if r.Eval(f) != nil {
		t.Error("should not fire when rules were applied")
	}
}

func TestMissedPushdown_NotFired_NoRules(t *testing.T) {
	f := baseFacts()
	r := MissedPushdown{}
	if r.Eval(f) != nil {
		t.Error("should not fire with no optimizer rules")
	}
}

func TestMissedPushdown_IgnoresNonImportantRules(t *testing.T) {
	f := baseFacts()
	f.OptimizerRules = []queryinfo.OptimizerRuleFact{
		{Rule: "RemoveRedundantIdentityProjections", Invocations: 5, Applied: 0},
	}

	r := MissedPushdown{}
	if r.Eval(f) != nil {
		t.Error("should not fire for non-important rules")
	}
}

func TestDefaultEngine_HealthyQuery(t *testing.T) {
	f := baseFacts()
	findings := DefaultEngine().Run(f)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for healthy query, got %d: %+v", len(findings), findings)
	}
}

func TestDefaultEngine_MultipleFindings(t *testing.T) {
	f := baseFacts()
	f.State = "FAILED"
	f.ErrorType = "INTERNAL_ERROR"
	f.ErrorCodeName = "GENERIC_INTERNAL_ERROR"
	f.Memory.SpilledBytes = 1 << 30
	f.Time.TotalCPUMs = 9500
	f.Time.TotalScheduledMs = 10000

	findings := DefaultEngine().Run(f)
	if len(findings) < 3 {
		t.Errorf("expected at least 3 findings, got %d: %+v", len(findings), findings)
	}

	if findings[0].Severity != "critical" {
		t.Errorf("first finding should be critical, got %q", findings[0].Severity)
	}
}
