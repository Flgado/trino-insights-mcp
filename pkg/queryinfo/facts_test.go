package queryinfo

import (
	"testing"
)

func TestExtractTableFacts_Deduplicates(t *testing.T) {
	qi := &QueryInfo{
		Inputs: []InputRef{
			{CatalogName: "hive", Schema: "prod", Table: "lineitem"},
			{CatalogName: "hive", Schema: "prod", Table: "orders"},
		},
		ReferencedTables: []TableRef{
			{CatalogName: "hive", SchemaName: "prod", TableName: "lineitem"},
			{CatalogName: "hive", SchemaName: "prod", TableName: "customer"},
		},
	}

	tables := extractTableFacts(qi)
	if len(tables) != 3 {
		t.Fatalf("expected 3 unique tables, got %d: %v", len(tables), tables)
	}

	want := map[string]bool{
		"hive.prod.lineitem": true,
		"hive.prod.orders":   true,
		"hive.prod.customer": true,
	}
	for _, tbl := range tables {
		if !want[tbl.FullName] {
			t.Errorf("unexpected table %q", tbl.FullName)
		}
		if tbl.Catalog != "hive" {
			t.Errorf("expected catalog=hive, got %q for %s", tbl.Catalog, tbl.FullName)
		}
		if tbl.ConnectorType != "hive" {
			t.Errorf("expected connector_type=hive, got %q for %s", tbl.ConnectorType, tbl.FullName)
		}
	}
}

func TestExtractTableFacts_EmptyInputs(t *testing.T) {
	qi := &QueryInfo{}
	tables := extractTableFacts(qi)
	if len(tables) != 0 {
		t.Errorf("expected 0 tables, got %v", tables)
	}
}

func TestInferConnectorType(t *testing.T) {
	cases := []struct {
		catalog       string
		connectorInfo any
		want          string
	}{
		{"hive", nil, "hive"},
		{"Iceberg", nil, "iceberg"},
		{"postgresql", nil, "postgresql"},
		{"", nil, ""},
		{"hive", map[string]any{"connectorName": "hive_custom"}, "hive_custom"},
	}

	for _, tc := range cases {
		got := inferConnectorType(tc.catalog, tc.connectorInfo)
		if got != tc.want {
			t.Errorf("inferConnectorType(%q, %v) = %q, want %q", tc.catalog, tc.connectorInfo, got, tc.want)
		}
	}
}

func TestProjectOptimizerRules(t *testing.T) {
	rules := []OptimizerRuleSummary{
		{Rule: "PushPredicateIntoTableScan", Invocations: 5, Applied: 0},
		{Rule: "RemoveRedundantIdentityProjections", Invocations: 10, Applied: 10},
		{Rule: "Unused", Invocations: 0, Applied: 0},
	}

	facts := projectOptimizerRules(rules)
	if len(facts) != 2 {
		t.Fatalf("expected 2 rules (with invocations>0), got %d", len(facts))
	}
	if facts[0].Rule != "PushPredicateIntoTableScan" {
		t.Errorf("expected first rule PushPredicateIntoTableScan, got %s", facts[0].Rule)
	}
	if facts[0].Applied != 0 {
		t.Errorf("expected applied=0, got %d", facts[0].Applied)
	}
}

func TestProjectOptimizerRules_Nil(t *testing.T) {
	facts := projectOptimizerRules(nil)
	if facts != nil {
		t.Errorf("expected nil for nil input, got %v", facts)
	}
}

func TestProjectDynamicFilters(t *testing.T) {
	dfs := DynamicFiltersStats{
		TotalDynamicFilters:      3,
		DynamicFiltersCompleted:  2,
		LazyDynamicFilters:       1,
		ReplicatedDynamicFilters: 2,
	}

	facts := projectDynamicFilters(dfs)
	if facts == nil {
		t.Fatal("expected non-nil dynamic filter facts")
	}
	if facts.Total != 3 {
		t.Errorf("expected total=3, got %d", facts.Total)
	}
	if facts.Completed != 2 {
		t.Errorf("expected completed=2, got %d", facts.Completed)
	}
}

func TestProjectDynamicFilters_Zero(t *testing.T) {
	dfs := DynamicFiltersStats{}
	if projectDynamicFilters(dfs) != nil {
		t.Error("expected nil for zero dynamic filters")
	}
}

func TestComputeAmplification(t *testing.T) {
	cases := []struct {
		in, out int64
		want    float64
	}{
		{1000, 10000, 10.0},
		{1000, 1000, 1.0},
		{1000, 100, 0.1},
		{0, 100, 0},
		{1000, 0, 0},
	}

	for _, tc := range cases {
		got := computeAmplification(tc.in, tc.out)
		if got != tc.want {
			t.Errorf("computeAmplification(%d, %d) = %v, want %v", tc.in, tc.out, got, tc.want)
		}
	}
}

func TestProjectOperators_PipelineOrder(t *testing.T) {
	ops := []OperatorSummary{
		{StageID: 0, PipelineID: 0, OperatorID: 2, OperatorType: "HashAggregationOperator", AddInputCpu: "1.00s", GetOutputCpu: "0.50s", InputPositions: 5000000, OutputPositions: 100000},
		{StageID: 0, PipelineID: 0, OperatorID: 0, OperatorType: "ScanFilterProjectOperator", AddInputCpu: "2.00s", GetOutputCpu: "0.00s", InputPositions: 5000000, OutputPositions: 5000000},
		{StageID: 0, PipelineID: 0, OperatorID: 1, OperatorType: "FilterAndProjectOperator", AddInputCpu: "0.10s", GetOutputCpu: "0.00s", InputPositions: 5000000, OutputPositions: 3000000},
	}

	facts, primary := projectOperators(ops)
	if len(facts) != 3 {
		t.Fatalf("expected 3 operators, got %d", len(facts))
	}

	// Should be sorted by operatorID: Scan(0), Filter(1), Hash(2)
	if facts[0].OperatorType != "ScanFilterProjectOperator" {
		t.Errorf("expected first operator ScanFilterProjectOperator, got %s", facts[0].OperatorType)
	}
	if facts[1].OperatorType != "FilterAndProjectOperator" {
		t.Errorf("expected second operator FilterAndProjectOperator, got %s", facts[1].OperatorType)
	}
	if facts[2].OperatorType != "HashAggregationOperator" {
		t.Errorf("expected third operator HashAggregationOperator, got %s", facts[2].OperatorType)
	}

	if primary != "ScanFilterProjectOperator" {
		t.Errorf("expected primary=ScanFilterProjectOperator (highest CPU), got %s", primary)
	}

	// Check amplification
	if facts[2].Amplification != 0.02 {
		t.Errorf("expected HashAgg amplification=0.02, got %v", facts[2].Amplification)
	}
}

func TestProjectOperators_FiltersInfrastructure(t *testing.T) {
	ops := []OperatorSummary{
		{StageID: 0, PipelineID: 0, OperatorID: 0, OperatorType: "ScanFilterProjectOperator", AddInputCpu: "1.00s", InputPositions: 100, OutputPositions: 100},
		{StageID: 0, PipelineID: 0, OperatorID: 1, OperatorType: "TaskOutputOperator", AddInputCpu: "0.01s", InputPositions: 100, OutputPositions: 100},
		{StageID: 0, PipelineID: 0, OperatorID: 2, OperatorType: "LocalExchangeSinkOperator", AddInputCpu: "0.01s", InputPositions: 100, OutputPositions: 100},
	}

	facts, _ := projectOperators(ops)
	if len(facts) != 1 {
		t.Fatalf("expected 1 non-infrastructure operator, got %d", len(facts))
	}
	if facts[0].OperatorType != "ScanFilterProjectOperator" {
		t.Errorf("expected ScanFilterProjectOperator, got %s", facts[0].OperatorType)
	}
}

func TestExtractPlanSummary(t *testing.T) {
	plan := map[string]any{
		"jsonRepresentation": map[string]any{
			"name": "Output",
			"children": []any{
				map[string]any{
					"name": "OrderBy",
					"children": []any{
						map[string]any{
							"name": "ScanFilterProject",
							"descriptor": map[string]any{
								"table": "lineitem",
							},
						},
					},
				},
			},
		},
	}

	got := extractPlanSummary(plan)
	want := "Output -> OrderBy -> ScanFilterProject[lineitem]"
	if got != want {
		t.Errorf("extractPlanSummary() = %q, want %q", got, want)
	}
}

func TestExtractPlanSummary_NilPlan(t *testing.T) {
	if got := extractPlanSummary(nil); got != "" {
		t.Errorf("expected empty for nil plan, got %q", got)
	}
}

func TestProject_PopulatesNewFields(t *testing.T) {
	qi := &QueryInfo{
		QueryID: "test_query_123",
		State:   "FINISHED",
		Inputs: []InputRef{
			{CatalogName: "hive", Schema: "prod", Table: "lineitem"},
		},
		QueryStats: QueryStats{
			OptimizerRulesSummaries: []OptimizerRuleSummary{
				{Rule: "PushPredicateIntoTableScan", Invocations: 3, Applied: 1},
			},
			DynamicFiltersStats: DynamicFiltersStats{
				TotalDynamicFilters:     2,
				DynamicFiltersCompleted: 1,
				LazyDynamicFilters:      1,
			},
		},
		Stages: &StagesWrapper{
			Stages: []StageInfo{
				{
					StageID:   "test_query_123.0",
					State:     "FINISHED",
					SubStages: []string{"test_query_123.1"},
					Plan: map[string]any{
						"jsonRepresentation": map[string]any{
							"name": "Output",
							"children": []any{
								map[string]any{"name": "Scan"},
							},
						},
					},
				},
				{
					StageID: "test_query_123.1",
					State:   "FINISHED",
				},
			},
		},
	}

	facts := Project(qi)

	if len(facts.Tables) != 1 || facts.Tables[0].FullName != "hive.prod.lineitem" {
		t.Errorf("expected tables=[hive.prod.lineitem], got %v", facts.Tables)
	}
	if facts.Tables[0].ConnectorType != "hive" {
		t.Errorf("expected connector_type=hive, got %q", facts.Tables[0].ConnectorType)
	}

	if len(facts.OptimizerRules) != 1 {
		t.Fatalf("expected 1 optimizer rule, got %d", len(facts.OptimizerRules))
	}
	if facts.OptimizerRules[0].Rule != "PushPredicateIntoTableScan" {
		t.Errorf("expected rule PushPredicateIntoTableScan, got %s", facts.OptimizerRules[0].Rule)
	}

	if facts.DynamicFilters == nil {
		t.Fatal("expected non-nil dynamic filters")
	}
	if facts.DynamicFilters.Total != 2 {
		t.Errorf("expected total=2, got %d", facts.DynamicFilters.Total)
	}

	if len(facts.Stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(facts.Stages))
	}

	s0 := facts.Stages[0]
	if len(s0.SubStageIDs) != 1 || s0.SubStageIDs[0] != "test_query_123.1" {
		t.Errorf("expected sub_stage_ids=[test_query_123.1], got %v", s0.SubStageIDs)
	}
	if s0.PlanSummary != "Output -> Scan" {
		t.Errorf("expected plan_summary='Output -> Scan', got %q", s0.PlanSummary)
	}
}

func TestExtractStageNum(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"20260425_140250_00084_83n6z.1", 1},
		{"20260425_140250_00084_83n6z.0", 0},
		{"20260425_140250_00084_83n6z.12", 12},
		{"no_dot", -1},
		{"trailing.abc", -1},
	}

	for _, tc := range cases {
		got := extractStageNum(tc.input)
		if got != tc.want {
			t.Errorf("extractStageNum(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}
