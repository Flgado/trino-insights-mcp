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

	tables := extractTableFacts(qi, nil)
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
	tables := extractTableFacts(qi, nil)
	if len(tables) != 0 {
		t.Errorf("expected 0 tables, got %v", tables)
	}
}

func TestExtractTableFacts_PrefersStageTablesConnector(t *testing.T) {
	// Custom catalog name that does NOT match a connector type — e.g. a MySQL
	// backend exposed as "app_reporting". Without the stage
	// tables map the old code would echo the catalog name as the connector.
	qi := &QueryInfo{
		Inputs: []InputRef{
			{CatalogName: "app_reporting", Schema: "platform", Table: "user_membership"},
		},
	}

	connectors := map[string]string{
		"app_reporting.platform.user_membership": "mysql",
	}

	tables := extractTableFacts(qi, connectors)
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if tables[0].ConnectorType != "mysql" {
		t.Errorf("expected connector_type=mysql from stage tables lookup, got %q", tables[0].ConnectorType)
	}
}

func TestExtractTableFacts_FallsBackToCatalogNameWhenMapEmpty(t *testing.T) {
	qi := &QueryInfo{
		Inputs: []InputRef{
			{CatalogName: "hive", Schema: "prod", Table: "lineitem"},
		},
	}

	tables := extractTableFacts(qi, nil)
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if tables[0].ConnectorType != "hive" {
		t.Errorf("expected fallback connector_type=hive, got %q", tables[0].ConnectorType)
	}
}

func TestCollectStageConnectors(t *testing.T) {
	stages := []StageInfo{
		{
			StageID: "q.1",
			Tables: map[string]any{
				"12": map[string]any{
					"connectorName": "MySql",
					"tableName":     "app_reporting.platform.user_membership",
				},
				"15": map[string]any{
					"connectorName": "iceberg",
				},
				"99": map[string]any{
					// connectorName missing — should be skipped
					"tableName": "x.y.z",
				},
			},
		},
		{
			StageID: "q.2",
			Tables:  nil, // no tables map — should be tolerated
		},
		{
			StageID: "q.3",
			Tables: map[string]any{
				"21": map[string]any{
					"connectorName": "redshift",
				},
			},
		},
	}

	gotByPlanNode, gotByFQN := collectStageConnectors(stages)
	wantByPlanNode := map[string]string{
		"12": "mysql",
		"15": "iceberg",
		"21": "redshift",
	}

	if len(gotByPlanNode) != len(wantByPlanNode) {
		t.Fatalf("expected %d planNode entries, got %d: %v", len(wantByPlanNode), len(gotByPlanNode), gotByPlanNode)
	}
	for k, v := range wantByPlanNode {
		if gotByPlanNode[k] != v {
			t.Errorf("planNode[%q] = %q, want %q", k, gotByPlanNode[k], v)
		}
	}

	// FQN map is built from the same entries but only those that carry a
	// tableName. The "15" entry has no tableName, the "99" entry has no
	// connectorName — both must be absent.
	if got := gotByFQN["app_reporting.platform.user_membership"]; got != "mysql" {
		t.Errorf("FQN lookup for user_membership = %q, want mysql", got)
	}
	if _, ok := gotByFQN["x.y.z"]; ok {
		t.Errorf("entry without connectorName must not appear in FQN map")
	}
	if len(gotByFQN) != 1 {
		t.Errorf("expected 1 FQN entry (only the one with both connectorName and tableName), got %d: %v", len(gotByFQN), gotByFQN)
	}
}

func TestConnectorsByFQN(t *testing.T) {
	scans := []ScanPushdownFact{
		{Catalog: "hive", Schema: "prod", Table: "lineitem", ConnectorType: "hive"},
		// Duplicate scan of same table — first non-empty wins.
		{Catalog: "hive", Schema: "prod", Table: "lineitem", ConnectorType: "hive"},
		{Catalog: "app_reporting", Schema: "platform", Table: "user_membership", ConnectorType: "mysql"},
		// Scan with no connector should not pollute the map.
		{Catalog: "x", Schema: "y", Table: "z", ConnectorType: ""},
	}

	got := connectorsByFQN(scans)
	if got["hive.prod.lineitem"] != "hive" {
		t.Errorf("hive.prod.lineitem = %q, want hive", got["hive.prod.lineitem"])
	}
	if got["app_reporting.platform.user_membership"] != "mysql" {
		t.Errorf("aggregatedb mapping wrong, got %q", got["app_reporting.platform.user_membership"])
	}
	if _, present := got["x.y.z"]; present {
		t.Errorf("empty-connector scan should not be mapped")
	}
}

func TestProject_UsesStageTablesForConnectorType(t *testing.T) {
	// End-to-end: catalog name is custom (looks nothing like a connector type),
	// but outputStage.subStages[].tables[planNodeId].connectorName tells us it
	// is a MySQL backend. Project() must report connector_type=mysql in both
	// facts.Tables and facts.ScanPushdown.
	qi := &QueryInfo{
		QueryID: "qx",
		State:   "FINISHED",
		Inputs: []InputRef{
			{CatalogName: "app_reporting", Schema: "platform", Table: "user_membership"},
		},
		Stages: &StagesWrapper{
			Stages: []StageInfo{
				{
					StageID: "qx.1",
					State:   "FINISHED",
					Tables: map[string]any{
						"node-7": map[string]any{
							"connectorName": "mysql",
							"tableName":     "app_reporting.platform.user_membership",
						},
					},
					Plan: map[string]any{
						"jsonRepresentation": map[string]any{
							"name": "ScanFilterProject",
							"id":   "node-7",
							"descriptor": map[string]any{
								"table": "app_reporting:platform.user_membership",
							},
						},
					},
					StageStats: StageStats{
						OperatorSummaries: []OperatorSummary{
							{StageID: 1, PipelineID: 0, OperatorID: 0, PlanNodeID: "node-7",
								OperatorType: "ScanFilterProjectOperator", InputPositions: 1000, OutputPositions: 1000},
						},
					},
				},
			},
		},
	}

	facts := Project(qi)

	if len(facts.Tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(facts.Tables))
	}
	if facts.Tables[0].ConnectorType != "mysql" {
		t.Errorf("Tables[0].ConnectorType = %q, want mysql (from stage tables map)", facts.Tables[0].ConnectorType)
	}

	if len(facts.ScanPushdown) != 1 {
		t.Fatalf("expected 1 scan pushdown, got %d", len(facts.ScanPushdown))
	}
	if facts.ScanPushdown[0].ConnectorType != "mysql" {
		t.Errorf("ScanPushdown[0].ConnectorType = %q, want mysql", facts.ScanPushdown[0].ConnectorType)
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

func TestProject_StageLevel_OperatorSummaries(t *testing.T) {
	qi := &QueryInfo{
		QueryID: "test_query_456",
		State:   "FINISHED",
		QueryStats: QueryStats{
			// Query-level OperatorSummaries is empty — operators live at the stage level only
			OperatorSummaries: nil,
		},
		Stages: &StagesWrapper{
			Stages: []StageInfo{
				{
					StageID: "test_query_456.0",
					State:   "FINISHED",
					StageStats: StageStats{
						TotalCpuTime: "5.00s",
						OperatorSummaries: []OperatorSummary{
							{StageID: 0, PipelineID: 0, OperatorID: 0, OperatorType: "ScanFilterProjectOperator", AddInputCpu: "3.00s", GetOutputCpu: "0.50s", InputPositions: 300000, OutputPositions: 300000},
							{StageID: 0, PipelineID: 0, OperatorID: 1, OperatorType: "LookupJoinOperator", AddInputCpu: "1.00s", GetOutputCpu: "0.20s", InputPositions: 300000, OutputPositions: 37},
							{StageID: 0, PipelineID: 0, OperatorID: 2, OperatorType: "TaskOutputOperator", AddInputCpu: "0.01s", GetOutputCpu: "0.00s", InputPositions: 37, OutputPositions: 37},
						},
					},
				},
				{
					StageID: "test_query_456.1",
					State:   "FINISHED",
					StageStats: StageStats{
						TotalCpuTime: "0.10s",
						OperatorSummaries: []OperatorSummary{
							{StageID: 1, PipelineID: 0, OperatorID: 0, OperatorType: "HashBuilderOperator", AddInputCpu: "0.05s", GetOutputCpu: "0.00s", InputPositions: 100, OutputPositions: 0},
						},
					},
				},
			},
		},
	}

	facts := Project(qi)

	if len(facts.Stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(facts.Stages))
	}

	// Stage 0: should have operators from stage-level summaries (minus infrastructure)
	s0 := facts.Stages[0]
	if len(s0.Operators) != 2 {
		t.Fatalf("stage 0: expected 2 non-infrastructure operators, got %d: %+v", len(s0.Operators), s0.Operators)
	}
	if s0.Operators[0].OperatorType != "ScanFilterProjectOperator" {
		t.Errorf("stage 0 op[0]: expected ScanFilterProjectOperator, got %s", s0.Operators[0].OperatorType)
	}
	if s0.Operators[0].InputRows != 300000 {
		t.Errorf("stage 0 op[0]: expected input_rows=300000, got %d", s0.Operators[0].InputRows)
	}
	if s0.Operators[1].OperatorType != "LookupJoinOperator" {
		t.Errorf("stage 0 op[1]: expected LookupJoinOperator, got %s", s0.Operators[1].OperatorType)
	}
	if s0.Operators[1].OutputRows != 37 {
		t.Errorf("stage 0 op[1]: expected output_rows=37, got %d", s0.Operators[1].OutputRows)
	}
	if s0.PrimaryOperator != "ScanFilterProjectOperator" {
		t.Errorf("stage 0: expected primary=ScanFilterProjectOperator, got %s", s0.PrimaryOperator)
	}

	// Stage 1: single operator
	s1 := facts.Stages[1]
	if len(s1.Operators) != 1 {
		t.Fatalf("stage 1: expected 1 operator, got %d", len(s1.Operators))
	}
	if s1.Operators[0].OperatorType != "HashBuilderOperator" {
		t.Errorf("stage 1 op[0]: expected HashBuilderOperator, got %s", s1.Operators[0].OperatorType)
	}
}

func TestProject_QueryLevel_OperatorsPreferred(t *testing.T) {
	// When query-level OperatorSummaries exist, they should be used (no fallback)
	qi := &QueryInfo{
		QueryID: "test_query_789",
		State:   "FINISHED",
		QueryStats: QueryStats{
			OperatorSummaries: []OperatorSummary{
				{StageID: 0, PipelineID: 0, OperatorID: 0, OperatorType: "ScanFilterProjectOperator", AddInputCpu: "2.00s", GetOutputCpu: "0.00s", InputPositions: 1000, OutputPositions: 1000},
			},
		},
		Stages: &StagesWrapper{
			Stages: []StageInfo{
				{
					StageID: "test_query_789.0",
					State:   "FINISHED",
					StageStats: StageStats{
						OperatorSummaries: []OperatorSummary{
							{StageID: 0, PipelineID: 0, OperatorID: 0, OperatorType: "ShouldNotAppear", AddInputCpu: "0.01s", InputPositions: 1, OutputPositions: 1},
						},
					},
				},
			},
		},
	}

	facts := Project(qi)
	if len(facts.Stages) != 1 {
		t.Fatalf("expected 1 stage, got %d", len(facts.Stages))
	}
	s0 := facts.Stages[0]
	if len(s0.Operators) != 1 {
		t.Fatalf("expected 1 operator, got %d", len(s0.Operators))
	}
	// Query-level should win since it produced results
	if s0.Operators[0].OperatorType != "ScanFilterProjectOperator" {
		t.Errorf("expected query-level ScanFilterProjectOperator, got %s", s0.Operators[0].OperatorType)
	}
}

func TestProject_OutputStage_Nested(t *testing.T) {
	// Simulates the /v1/query/{id} response where stages come via outputStage (nested tree)
	// instead of the flat stages wrapper.
	qi := &QueryInfo{
		QueryID: "20260504_150328_11566_mpxwv",
		State:   "FINISHED",
		QueryStats: QueryStats{
			TotalCpuTime:       "50.00ms",
			TotalScheduledTime: "100.00ms",
		},
		OutputStage: &NestedStageInfo{
			StageID: "20260504_150328_11566_mpxwv.0",
			State:   "FINISHED",
			StageStats: StageStats{
				TotalCpuTime:    "5.00ms",
				OutputDataSize:  "100B",
				OutputPositions: 37,
				OperatorSummaries: []OperatorSummary{
					{StageID: 0, PipelineID: 0, OperatorID: 0, OperatorType: "OutputSpoolingOperator", AddInputCpu: "0.50ms", GetOutputCpu: "0.00ms", InputPositions: 37, OutputPositions: 37},
				},
			},
			Plan: map[string]any{
				"jsonRepresentation": map[string]any{
					"name": "Output",
				},
			},
			SubStages: []NestedStageInfo{
				{
					StageID: "20260504_150328_11566_mpxwv.1",
					State:   "FINISHED",
					StageStats: StageStats{
						TotalCpuTime:           "30.00ms",
						PhysicalInputDataSize:  "1kB",
						PhysicalInputPositions: 37,
						OperatorSummaries: []OperatorSummary{
							{StageID: 1, PipelineID: 0, OperatorID: 0, OperatorType: "ScanFilterProjectOperator", AddInputCpu: "2.90ms", GetOutputCpu: "0.00ms", InputPositions: 37, OutputPositions: 37},
							{StageID: 1, PipelineID: 0, OperatorID: 1, OperatorType: "LookupJoinOperator", AddInputCpu: "0.08ms", GetOutputCpu: "0.00ms", InputPositions: 37, OutputPositions: 37},
						},
					},
					SubStages: []NestedStageInfo{
						{
							StageID: "20260504_150328_11566_mpxwv.3",
							State:   "FINISHED",
							StageStats: StageStats{
								TotalCpuTime:           "19.00ms",
								PhysicalInputDataSize:  "10MB",
								PhysicalInputPositions: 300335,
								OperatorSummaries: []OperatorSummary{
									{StageID: 3, PipelineID: 0, OperatorID: 0, OperatorType: "ScanFilterProjectOperator", AddInputCpu: "15.00ms", GetOutputCpu: "0.00ms", InputPositions: 300335, OutputPositions: 300335},
									{StageID: 3, PipelineID: 1, OperatorID: 0, OperatorType: "HashBuilderOperator", AddInputCpu: "4.00ms", GetOutputCpu: "0.00ms", InputPositions: 300335, OutputPositions: 300335, PeakUserMemoryReservation: "23MB"},
								},
							},
						},
					},
				},
			},
		},
	}

	facts := Project(qi)

	if len(facts.Stages) != 3 {
		t.Fatalf("expected 3 stages from nested outputStage, got %d", len(facts.Stages))
	}

	// Stage 0 (root) should have sub_stage_ids pointing to stage 1
	s0 := facts.Stages[0]
	if s0.StageID != "20260504_150328_11566_mpxwv.0" {
		t.Errorf("stage[0] ID = %q, want ...0", s0.StageID)
	}
	if len(s0.SubStageIDs) != 1 || s0.SubStageIDs[0] != "20260504_150328_11566_mpxwv.1" {
		t.Errorf("stage[0] sub_stage_ids = %v, want [....1]", s0.SubStageIDs)
	}
	if s0.PlanSummary != "Output" {
		t.Errorf("stage[0] plan_summary = %q, want 'Output'", s0.PlanSummary)
	}

	// Stage 1 should have operators from stage-level summaries
	s1 := facts.Stages[1]
	if s1.StageID != "20260504_150328_11566_mpxwv.1" {
		t.Errorf("stage[1] ID = %q, want ...1", s1.StageID)
	}
	if len(s1.Operators) < 2 {
		t.Fatalf("stage[1] expected >=2 operators, got %d", len(s1.Operators))
	}
	if s1.Operators[0].OperatorType != "ScanFilterProjectOperator" {
		t.Errorf("stage[1] op[0] = %q, want ScanFilterProjectOperator", s1.Operators[0].OperatorType)
	}
	if s1.Operators[0].InputRows != 37 {
		t.Errorf("stage[1] op[0] input_rows = %d, want 37", s1.Operators[0].InputRows)
	}

	// Stage 3 (deeply nested) should have the HashBuilderOperator
	s3 := facts.Stages[2]
	if s3.StageID != "20260504_150328_11566_mpxwv.3" {
		t.Errorf("stage[2] ID = %q, want ...3", s3.StageID)
	}
	if s3.PhysicalInputPos != 300335 {
		t.Errorf("stage[2] physical_input_positions = %d, want 300335", s3.PhysicalInputPos)
	}
	foundHash := false
	for _, op := range s3.Operators {
		if op.OperatorType == "HashBuilderOperator" {
			foundHash = true
			if op.InputRows != 300335 {
				t.Errorf("HashBuilderOperator input_rows = %d, want 300335", op.InputRows)
			}
		}
	}
	if !foundHash {
		t.Error("stage[2] should contain HashBuilderOperator but it wasn't found")
	}
}

func TestProject_FlatStages_PreferredOverOutputStage(t *testing.T) {
	// When both Stages (flat) and OutputStage (nested) are present, flat should win
	qi := &QueryInfo{
		QueryID: "test_both",
		State:   "FINISHED",
		Stages: &StagesWrapper{
			Stages: []StageInfo{
				{StageID: "test_both.0", State: "FINISHED"},
			},
		},
		OutputStage: &NestedStageInfo{
			StageID: "test_both.0",
			State:   "FINISHED",
			SubStages: []NestedStageInfo{
				{StageID: "test_both.1", State: "FINISHED"},
			},
		},
	}

	facts := Project(qi)
	// Flat has 1 stage, nested would give 2 — we should get 1
	if len(facts.Stages) != 1 {
		t.Fatalf("expected flat stages (1) to be preferred, got %d stages", len(facts.Stages))
	}
}

func TestFlattenOutputStage_Nil(t *testing.T) {
	result := flattenOutputStage(nil)
	if result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
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
