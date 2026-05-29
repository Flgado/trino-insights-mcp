package queryinfo

import (
	"reflect"
	"testing"
)

func TestParseTableDescriptor(t *testing.T) {
	cases := []struct {
		in                       string
		wantCat, wantSch, wantTb string
	}{
		{"app_documents:platform.user_credits", "app_documents", "platform", "user_credits"},
		{"hive:default.lineitem", "hive", "default", "lineitem"},
		{"mysql:public.orders", "mysql", "public", "orders"},
		{"app_documents:platform.user_membership MongoTableHandle{filter=...}", "app_documents", "platform", "user_membership"},
		{"redshift:analytics.agg_user_total_visits_v2", "redshift", "analytics", "agg_user_total_visits_v2"},
		{"", "", "", ""},
		{"single_table", "", "", "single_table"},
		{"schema_only.tbl", "", "schema_only", "tbl"},
	}
	for _, tc := range cases {
		cat, sch, tb := parseTableDescriptor(tc.in)
		if cat != tc.wantCat || sch != tc.wantSch || tb != tc.wantTb {
			t.Errorf("parseTableDescriptor(%q) = (%q,%q,%q), want (%q,%q,%q)",
				tc.in, cat, sch, tb, tc.wantCat, tc.wantSch, tc.wantTb)
		}
	}
}

func TestParseConstraintColumns(t *testing.T) {
	cases := []struct {
		details []string
		want    []string
	}{
		{
			details: []string{"constraint on [user_membership_id, source, branch_id]"},
			want:    []string{"user_membership_id", "source", "branch_id"},
		},
		{
			details: []string{"estimates: 1234 rows", "constraint on [a, b]"},
			want:    []string{"a", "b"},
		},
		{
			details: []string{"no constraint here"},
			want:    nil,
		},
		{
			details: nil,
			want:    nil,
		},
	}
	for _, tc := range cases {
		got := parseConstraintColumns(tc.details)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("parseConstraintColumns(%v) = %v, want %v", tc.details, got, tc.want)
		}
	}
}

func TestCollectDetails(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want []string
	}{
		{
			name: "array of strings",
			in:   []any{"constraint on [a]", " estimates: 100 ", ""},
			want: []string{"constraint on [a]", "estimates: 100"},
		},
		{
			name: "single string",
			in:   "  one line  ",
			want: []string{"one line"},
		},
		{
			name: "nil",
			in:   nil,
			want: nil,
		},
		{
			name: "empty string",
			in:   "",
			want: nil,
		},
	}
	for _, tc := range cases {
		got := collectDetails(tc.in)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: collectDetails(%v) = %v, want %v", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestExtractScanPushdowns_MongoLikePlan(t *testing.T) {
	// Plan tree mimicking the user_credits scan from the working session:
	// MongoDB scan with a constraint on [user_membership_id, source, branch_id]
	// AND a local filter for status = 'ACTIVE' that ran in Trino.
	plan := map[string]any{
		"jsonRepresentation": map[string]any{
			"name": "ScanFilterProject",
			"id":   "scan-0",
			"descriptor": map[string]any{
				"table":           "app_documents:platform.user_credits",
				"filterPredicate": "(\"status\" = 'ACTIVE')",
			},
			"details": []any{
				"constraint on [user_membership_id, source, branch_id]",
				"estimates: 7834 rows",
			},
		},
	}

	ops := []OperatorSummary{
		{StageID: 7, PipelineID: 0, OperatorID: 0, PlanNodeID: "scan-0", OperatorType: "ScanFilterProjectOperator",
			AddInputCpu: "3.00s", GetOutputCpu: "0.10s", InputPositions: 7834, OutputPositions: 0},
	}

	got := extractScanPushdowns("query.7", plan, ops, nil, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 scan pushdown fact, got %d", len(got))
	}

	assertMongoCreditsScan(t, got[0])
}

func TestExtractScanPushdowns_PrefersStageTablesConnector(t *testing.T) {
	// Custom catalog ("app_reporting") that the old code would
	// echo as the connector type. With the stage tables map, the connector
	// must be mysql.
	plan := map[string]any{
		"jsonRepresentation": map[string]any{
			"name": "ScanFilterProject",
			"id":   "node-7",
			"descriptor": map[string]any{
				"table": "app_reporting:platform.user_membership",
			},
		},
	}
	connectorByPlanNode := map[string]string{
		"node-7": "mysql",
	}

	got := extractScanPushdowns("q.1", plan, nil, connectorByPlanNode, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 scan, got %d", len(got))
	}
	if got[0].ConnectorType != "mysql" {
		t.Errorf("ConnectorType = %q, want mysql (from stage tables map)", got[0].ConnectorType)
	}
}

func TestExtractScanPushdowns_FallsBackToFQNForFusedScan(t *testing.T) {
	// Reproduces the ScanFilterProject id-mismatch bug: Trino keys
	// outputStage[].tables by the underlying TableScan id ("19") but the
	// plan tree only ever exposes the fused ScanFilterProject id ("37"). The
	// FQN-based fallback must kick in so the connector resolves to "mysql"
	// rather than being mistakenly echoed as the catalog name.
	plan := map[string]any{
		"jsonRepresentation": map[string]any{
			"name": "ScanFilterProject",
			"id":   "37",
			"descriptor": map[string]any{
				"table": "app_reporting:platform.user_membership",
			},
		},
	}
	connectorByFQN := map[string]string{
		"app_reporting.platform.user_membership": "mysql",
	}

	got := extractScanPushdowns("q.4", plan, nil, nil, connectorByFQN)
	if len(got) != 1 {
		t.Fatalf("expected 1 scan, got %d", len(got))
	}
	if got[0].ConnectorType != "mysql" {
		t.Errorf("ConnectorType = %q, want mysql (resolved via FQN fallback for fused scan)", got[0].ConnectorType)
	}
}

func TestExtractScanPushdowns_FallsBackToCatalogWhenNoStageMap(t *testing.T) {
	plan := map[string]any{
		"jsonRepresentation": map[string]any{
			"name": "TableScan",
			"id":   "scan-2",
			"descriptor": map[string]any{
				"table": "Hive:default.lineitem",
			},
		},
	}
	got := extractScanPushdowns("q.0", plan, nil, nil, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 scan, got %d", len(got))
	}
	if got[0].ConnectorType != "hive" {
		t.Errorf("ConnectorType = %q, want hive (lowercased fallback)", got[0].ConnectorType)
	}
}

func assertMongoCreditsScan(t *testing.T, s ScanPushdownFact) {
	t.Helper()
	if s.Catalog != "app_documents" || s.Schema != "platform" || s.Table != "user_credits" {
		t.Errorf("wrong table identity: %+v", s)
	}
	if s.NodeName != "ScanFilterProject" {
		t.Errorf("NodeName = %q, want ScanFilterProject", s.NodeName)
	}
	if s.PlanNodeID != "scan-0" {
		t.Errorf("PlanNodeID = %q, want scan-0", s.PlanNodeID)
	}
	if s.LocalFilter != `("status" = 'ACTIVE')` {
		t.Errorf("LocalFilter = %q", s.LocalFilter)
	}
	wantCols := []string{"user_membership_id", "source", "branch_id"}
	if !reflect.DeepEqual(s.PushedConstraintColumns, wantCols) {
		t.Errorf("PushedConstraintColumns = %v, want %v", s.PushedConstraintColumns, wantCols)
	}
	if s.PhysicalInputPositions != 7834 || s.OutputRows != 0 {
		t.Errorf("rows wrong: in=%d out=%d", s.PhysicalInputPositions, s.OutputRows)
	}
	if s.Selectivity != 0 {
		t.Errorf("Selectivity = %v, want 0", s.Selectivity)
	}
	if !containsString(s.PushedDetails, "constraint on [user_membership_id, source, branch_id]") {
		t.Errorf("expected constraint line in PushedDetails, got %v", s.PushedDetails)
	}
	if containsString(s.PushedDetails, "estimates: 7834 rows") {
		t.Errorf("estimates line should have been filtered out, got %v", s.PushedDetails)
	}
}

func containsString(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func TestExtractScanPushdowns_TableScanWithoutFilter(t *testing.T) {
	plan := map[string]any{
		"jsonRepresentation": map[string]any{
			"name": "TableScan",
			"id":   "scan-1",
			"descriptor": map[string]any{
				"table": "hive:default.lineitem",
			},
		},
	}

	got := extractScanPushdowns("q.1", plan, nil, nil, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 scan, got %d", len(got))
	}
	s := got[0]
	if s.LocalFilter != "" {
		t.Errorf("LocalFilter should be empty, got %q", s.LocalFilter)
	}
	if s.Catalog != "hive" || s.Schema != "default" || s.Table != "lineitem" {
		t.Errorf("wrong identity: %+v", s)
	}
}

func TestExtractScanPushdowns_NestedChildren(t *testing.T) {
	// Verifies the walker descends through non-scan parent nodes.
	plan := map[string]any{
		"jsonRepresentation": map[string]any{
			"name": "InnerJoin",
			"children": []any{
				map[string]any{
					"name": "ScanFilterProject",
					"id":   "scan-a",
					"descriptor": map[string]any{
						"table": "mysql:platform.user_membership",
					},
				},
				map[string]any{
					"name": "RemoteSource",
					"children": []any{
						map[string]any{
							"name": "TableScan",
							"id":   "scan-b",
							"descriptor": map[string]any{
								"table": "mongo:platform.user_credits",
							},
						},
					},
				},
			},
		},
	}

	got := extractScanPushdowns("q.0", plan, nil, nil, nil)
	if len(got) != 2 {
		t.Fatalf("expected 2 scans, got %d: %+v", len(got), got)
	}
	tables := map[string]bool{}
	for _, s := range got {
		tables[s.Table] = true
	}
	if !tables["user_membership"] || !tables["user_credits"] {
		t.Errorf("missing expected tables, got %v", tables)
	}
}

func TestExtractScanPushdowns_NilPlan(t *testing.T) {
	if got := extractScanPushdowns("q.0", nil, nil, nil, nil); got != nil {
		t.Errorf("expected nil for nil plan, got %v", got)
	}
}

func TestExtractScanPushdowns_JSONStringPlan(t *testing.T) {
	// Some Trino responses encode jsonRepresentation as a string, not a map.
	plan := map[string]any{
		"jsonRepresentation": `{"name":"TableScan","id":"x","descriptor":{"table":"hive:default.t"}}`,
	}
	got := extractScanPushdowns("q.0", plan, nil, nil, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 scan from string jsonRepresentation, got %d", len(got))
	}
	if got[0].Table != "t" {
		t.Errorf("table = %q", got[0].Table)
	}
}

func TestProject_PopulatesScanPushdown(t *testing.T) {
	// End-to-end: Project() should populate facts.ScanPushdown and stage.ScanPushdown.
	qi := &QueryInfo{
		QueryID: "q",
		State:   "FINISHED",
		Stages: &StagesWrapper{
			Stages: []StageInfo{
				{
					StageID: "q.1",
					State:   "FINISHED",
					Plan: map[string]any{
						"jsonRepresentation": map[string]any{
							"name": "ScanFilterProject",
							"id":   "p-1",
							"descriptor": map[string]any{
								"table":           "mongo:platform.user_credits",
								"filterPredicate": "(\"status\" = 'ACTIVE')",
							},
							"details": []any{"constraint on [branch_id]"},
						},
					},
					StageStats: StageStats{
						OperatorSummaries: []OperatorSummary{
							{StageID: 1, PipelineID: 0, OperatorID: 0, PlanNodeID: "p-1",
								OperatorType:   "ScanFilterProjectOperator",
								InputPositions: 7834, OutputPositions: 0},
						},
					},
				},
			},
		},
	}

	facts := Project(qi)
	if len(facts.ScanPushdown) != 1 {
		t.Fatalf("expected 1 scan pushdown at top level, got %d", len(facts.ScanPushdown))
	}
	if len(facts.Stages) != 1 || len(facts.Stages[0].ScanPushdown) != 1 {
		t.Fatalf("expected stage-level scan pushdown, got %+v", facts.Stages)
	}
	s := facts.ScanPushdown[0]
	if s.StageID != "q.1" || s.Catalog != "mongo" || s.Table != "user_credits" {
		t.Errorf("wrong scan: %+v", s)
	}
	if s.PhysicalInputPositions != 7834 || s.OutputRows != 0 {
		t.Errorf("operator metrics not joined: %+v", s)
	}
	if s.LocalFilter == "" {
		t.Errorf("LocalFilter should be populated")
	}
}
