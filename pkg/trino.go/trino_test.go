package trino

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Flgado/trino-insights-mcp/pkg/diagnose"
	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
	"github.com/Flgado/trino-insights-mcp/pkg/rest"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- Fake Fetcher ---

type fakeFetcher struct {
	qi  *queryinfo.QueryInfo
	err error
}

func (f *fakeFetcher) Fetch(_ context.Context, _ string) (*queryinfo.QueryInfo, error) {
	return f.qi, f.err
}

// --- deps.go ---

func TestToolDependencies_QueryFetcher_WithFetcher(t *testing.T) {
	f := &fakeFetcher{}
	deps := ToolDependencies{Fetcher: f}
	if deps.QueryFetcher() != f {
		t.Error("should return the provided Fetcher")
	}
}

func TestToolDependencies_QueryFetcher_FallbackToREST(t *testing.T) {
	client, _ := rest.NewClient(rest.Options{BaseURL: "http://trino:8080"})
	deps := ToolDependencies{REST: client}
	fetcher := deps.QueryFetcher()
	if _, ok := fetcher.(*queryinfo.RestFetcher); !ok {
		t.Errorf("expected RestFetcher, got %T", fetcher)
	}
}

// --- plans.go ---

func TestEffectiveSQLBudget(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{"zero", 0, MinSQLBudgetBytes},
		{"negative", -100, MinSQLBudgetBytes},
		{"below min", 100, MinSQLBudgetBytes},
		{"at min", MinSQLBudgetBytes, MinSQLBudgetBytes},
		{"normal", 16 * 1024, 16 * 1024},
		{"at max", MaxQuerySQLBytes, MaxQuerySQLBytes},
		{"above max", MaxQuerySQLBytes + 1, MaxQuerySQLBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := effectiveSQLBudget(tt.input)
			if got != tt.want {
				t.Errorf("effectiveSQLBudget(%d) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestConstants(t *testing.T) {
	if MaxQuerySQLBytes != 64*1024 {
		t.Errorf("MaxQuerySQLBytes = %d, want %d", MaxQuerySQLBytes, 64*1024)
	}
	if MinSQLBudgetBytes != 2*1024 {
		t.Errorf("MinSQLBudgetBytes = %d, want %d", MinSQLBudgetBytes, 2*1024)
	}
}

// --- tools.go ---

func TestAllTools_ReturnsTwoTools(t *testing.T) {
	tFunc := func(key, fallback string) string { return fallback }
	tools := AllTools(tFunc)
	if len(tools) != 2 {
		t.Fatalf("AllTools() returned %d tools, want 2", len(tools))
	}

	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Tool.Name] = true
	}
	if !names["analyze_query"] {
		t.Error("missing analyze_query tool")
	}
	if !names["get_query_sql"] {
		t.Error("missing get_query_sql tool")
	}
}

func TestToolsetMetadataPlans(t *testing.T) {
	if ToolsetMetadataPlans.ID != "plans" {
		t.Errorf("ID = %q, want %q", ToolsetMetadataPlans.ID, "plans")
	}
	if !ToolsetMetadataPlans.Default {
		t.Error("plans toolset should be default")
	}
	if ToolsetMetadataPlans.InstructionsFunc == nil {
		t.Fatal("InstructionsFunc should not be nil")
	}
	instr := ToolsetMetadataPlans.InstructionsFunc(nil)
	if instr == "" {
		t.Error("instructions should not be empty")
	}
}

// --- analyze_query.go ---

func callTool(st mcp.ToolHandler, argsJSON json.RawMessage) (*mcp.CallToolResult, error) {
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Arguments: argsJSON,
		},
	}
	return st(context.Background(), req)
}

func TestAnalyzeQueryTool_EmptyQueryID(t *testing.T) {
	tFunc := func(key, fallback string) string { return fallback }
	tool := AnalyzeQueryTool(tFunc)
	handler := tool.HandlerFunc(ToolDependencies{Fetcher: &fakeFetcher{}})

	result, err := callTool(handler, json.RawMessage(`{"query_id":""}`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error result for empty query_id")
	}
}

func TestAnalyzeQueryTool_FetchError(t *testing.T) {
	tFunc := func(key, fallback string) string { return fallback }
	tool := AnalyzeQueryTool(tFunc)
	deps := ToolDependencies{Fetcher: &fakeFetcher{err: fmt.Errorf("connection refused")}}
	handler := tool.HandlerFunc(deps)

	result, err := callTool(handler, json.RawMessage(`{"query_id":"abc"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error result for fetch failure")
	}
}

func TestAnalyzeQueryTool_Success(t *testing.T) {
	qi := &queryinfo.QueryInfo{
		QueryID: "q123",
		State:   "FINISHED",
		Query:   "SELECT 1",
	}

	tFunc := func(key, fallback string) string { return fallback }
	tool := AnalyzeQueryTool(tFunc)
	deps := ToolDependencies{Fetcher: &fakeFetcher{qi: qi}}
	handler := tool.HandlerFunc(deps)

	result, err := callTool(handler, json.RawMessage(`{"query_id":"q123"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(*mcp.TextContent).Text
	var analysis QueryAnalysis
	if err := json.Unmarshal([]byte(text), &analysis); err != nil {
		t.Fatalf("failed to unmarshal analysis: %v", err)
	}
	if analysis.Headline == "" {
		t.Error("headline should not be empty")
	}
	if analysis.Facts == nil {
		t.Error("facts should not be nil")
	}
}

func TestAnalyzeQueryTool_FailedQuery(t *testing.T) {
	qi := &queryinfo.QueryInfo{
		QueryID:   "q_fail",
		State:     "FAILED",
		ErrorType: "USER_ERROR",
		ErrorCode: &queryinfo.ErrorCode{Name: "SYNTAX_ERROR", Type: "USER_ERROR"},
	}

	tFunc := func(key, fallback string) string { return fallback }
	tool := AnalyzeQueryTool(tFunc)
	deps := ToolDependencies{Fetcher: &fakeFetcher{qi: qi}}
	handler := tool.HandlerFunc(deps)

	result, err := callTool(handler, json.RawMessage(`{"query_id":"q_fail"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatal("failed queries should still return successfully")
	}

	text := result.Content[0].(*mcp.TextContent).Text
	var analysis QueryAnalysis
	json.Unmarshal([]byte(text), &analysis)
	if analysis.Headline == "" {
		t.Error("headline should contain failure info")
	}
}

// --- buildHeadline ---

func TestBuildHeadline_Failed(t *testing.T) {
	facts := &queryinfo.QueryFacts{
		State:         "FAILED",
		ErrorType:     "INTERNAL_ERROR",
		ErrorCodeName: "GENERIC_INTERNAL_ERROR",
	}
	headline := buildHeadline(facts, nil)
	if headline != "FAILED with INTERNAL_ERROR: GENERIC_INTERNAL_ERROR" {
		t.Errorf("headline = %q", headline)
	}
}

func TestBuildHeadline_FailedNoErrorType(t *testing.T) {
	facts := &queryinfo.QueryFacts{State: "FAILED"}
	headline := buildHeadline(facts, nil)
	if headline != "FAILED (FAILED)" {
		t.Errorf("headline = %q", headline)
	}
}

func TestBuildHeadline_MetricClean(t *testing.T) {
	facts := &queryinfo.QueryFacts{
		QueryID: "q1",
		State:   "FINISHED",
		Time:    queryinfo.TimeFacts{ElapsedMs: 100, TotalCPUMs: 50},
	}
	headline := buildHeadline(facts, nil)
	if headline == "" {
		t.Error("metric-clean headline should not be empty")
	}
}

func TestBuildHeadline_WithFindings(t *testing.T) {
	facts := &queryinfo.QueryFacts{
		QueryID: "q1",
		State:   "FINISHED",
	}
	findings := diagnose.Findings{
		{Title: "CPU bound", Severity: diagnose.SeverityCritical},
		{Title: "Stage skew", Severity: diagnose.SeverityWarn},
	}
	headline := buildHeadline(facts, findings)
	if headline != "CPU bound; Stage skew" {
		t.Errorf("headline = %q", headline)
	}
}

func TestBuildHeadline_SingleFinding(t *testing.T) {
	facts := &queryinfo.QueryFacts{State: "FINISHED"}
	findings := diagnose.Findings{
		{Title: "Memory pressure", Severity: diagnose.SeverityWarn},
	}
	headline := buildHeadline(facts, findings)
	if headline != "Memory pressure" {
		t.Errorf("headline = %q, want %q", headline, "Memory pressure")
	}
}

// --- get_query_sql.go ---

func TestGetQuerySQLTool_EmptyQueryID(t *testing.T) {
	tFunc := func(key, fallback string) string { return fallback }
	tool := GetQuerySQLTool(tFunc)
	deps := ToolDependencies{Fetcher: &fakeFetcher{}}
	handler := tool.HandlerFunc(deps)

	result, err := callTool(handler, json.RawMessage(`{"query_id":""}`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for empty query_id")
	}
}

func TestGetQuerySQLTool_FetchError(t *testing.T) {
	tFunc := func(key, fallback string) string { return fallback }
	tool := GetQuerySQLTool(tFunc)
	deps := ToolDependencies{Fetcher: &fakeFetcher{err: fmt.Errorf("timeout")}}
	handler := tool.HandlerFunc(deps)

	result, err := callTool(handler, json.RawMessage(`{"query_id":"q1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for fetch failure")
	}
}

func TestGetQuerySQLTool_NoSQL(t *testing.T) {
	qi := &queryinfo.QueryInfo{QueryID: "q1", State: "FINISHED"}
	tFunc := func(key, fallback string) string { return fallback }
	tool := GetQuerySQLTool(tFunc)
	deps := ToolDependencies{
		Fetcher:           &fakeFetcher{qi: qi},
		ContentWindowSize: 16 * 1024,
	}
	handler := tool.HandlerFunc(deps)

	result, err := callTool(handler, json.RawMessage(`{"query_id":"q1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error when no SQL text available")
	}
}

func TestGetQuerySQLTool_Success(t *testing.T) {
	qi := &queryinfo.QueryInfo{
		QueryID: "q1",
		State:   "FINISHED",
		Query:   "SELECT * FROM orders WHERE id = 42",
	}
	tFunc := func(key, fallback string) string { return fallback }
	tool := GetQuerySQLTool(tFunc)
	deps := ToolDependencies{
		Fetcher:           &fakeFetcher{qi: qi},
		ContentWindowSize: 16 * 1024,
	}
	handler := tool.HandlerFunc(deps)

	result, err := callTool(handler, json.RawMessage(`{"query_id":"q1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatal("unexpected error")
	}

	text := result.Content[0].(*mcp.TextContent).Text
	var res getQuerySQLResult
	if err := json.Unmarshal([]byte(text), &res); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if res.SQL != qi.Query {
		t.Errorf("SQL = %q, want %q", res.SQL, qi.Query)
	}
	if res.Truncated {
		t.Error("should not be truncated")
	}
	if res.Source != "Query" {
		t.Errorf("Source = %q, want %q", res.Source, "Query")
	}
}

func TestGetQuerySQLTool_FallsBackToPreview(t *testing.T) {
	qi := &queryinfo.QueryInfo{
		QueryID:          "q1",
		State:            "FINISHED",
		QueryTextPreview: "SELECT 1",
	}
	tFunc := func(key, fallback string) string { return fallback }
	tool := GetQuerySQLTool(tFunc)
	deps := ToolDependencies{
		Fetcher:           &fakeFetcher{qi: qi},
		ContentWindowSize: 16 * 1024,
	}
	handler := tool.HandlerFunc(deps)

	result, _ := callTool(handler, json.RawMessage(`{"query_id":"q1"}`))
	text := result.Content[0].(*mcp.TextContent).Text
	var res getQuerySQLResult
	json.Unmarshal([]byte(text), &res)
	if res.Source != "QueryTextPreview" {
		t.Errorf("Source = %q, want %q", res.Source, "QueryTextPreview")
	}
}

func TestGetQuerySQLTool_WithPreparedQuery(t *testing.T) {
	qi := &queryinfo.QueryInfo{
		QueryID:       "q1",
		State:         "FINISHED",
		Query:         "EXECUTE stmt USING 'abc', 42",
		PreparedQuery: "SELECT * FROM t WHERE id = ? AND val = ?",
	}
	tFunc := func(key, fallback string) string { return fallback }
	tool := GetQuerySQLTool(tFunc)
	deps := ToolDependencies{
		Fetcher:           &fakeFetcher{qi: qi},
		ContentWindowSize: 16 * 1024,
	}
	handler := tool.HandlerFunc(deps)

	result, _ := callTool(handler, json.RawMessage(`{"query_id":"q1"}`))
	text := result.Content[0].(*mcp.TextContent).Text
	var res getQuerySQLResult
	json.Unmarshal([]byte(text), &res)
	if res.PreparedQuery == "" {
		t.Error("PreparedQuery should be populated")
	}
	if res.Source != "PreparedQuery" {
		t.Errorf("Source = %q, want %q", res.Source, "PreparedQuery")
	}
}

func TestGetQuerySQLTool_Truncation(t *testing.T) {
	longSQL := make([]byte, MaxQuerySQLBytes+100)
	for i := range longSQL {
		longSQL[i] = 'A'
	}
	qi := &queryinfo.QueryInfo{
		QueryID: "q1",
		State:   "FINISHED",
		Query:   string(longSQL),
	}
	tFunc := func(key, fallback string) string { return fallback }
	tool := GetQuerySQLTool(tFunc)
	deps := ToolDependencies{
		Fetcher:           &fakeFetcher{qi: qi},
		ContentWindowSize: MaxQuerySQLBytes,
	}
	handler := tool.HandlerFunc(deps)

	result, _ := callTool(handler, json.RawMessage(`{"query_id":"q1"}`))
	text := result.Content[0].(*mcp.TextContent).Text
	var res getQuerySQLResult
	json.Unmarshal([]byte(text), &res)
	if !res.Truncated {
		t.Error("should be truncated")
	}
	if res.ReturnedBytes > MaxQuerySQLBytes {
		t.Errorf("ReturnedBytes = %d, should not exceed %d", res.ReturnedBytes, MaxQuerySQLBytes)
	}
}

// --- toolResultText / toolResultError ---

func TestToolResultText(t *testing.T) {
	r := toolResultText("hello")
	if r.IsError {
		t.Error("should not be an error")
	}
	if len(r.Content) != 1 {
		t.Fatalf("expected 1 content, got %d", len(r.Content))
	}
	tc := r.Content[0].(*mcp.TextContent)
	if tc.Text != "hello" {
		t.Errorf("Text = %q", tc.Text)
	}
}

func TestToolResultError(t *testing.T) {
	r := toolResultError("boom")
	if !r.IsError {
		t.Error("should be an error")
	}
	tc := r.Content[0].(*mcp.TextContent)
	if tc.Text != "boom" {
		t.Errorf("Text = %q", tc.Text)
	}
}
