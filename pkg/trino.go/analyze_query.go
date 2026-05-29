package trino

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/Flgado/trino-insights-mcp/pkg/diagnose"
	"github.com/Flgado/trino-insights-mcp/pkg/diagnose/rules"
	"github.com/Flgado/trino-insights-mcp/pkg/inventory"
	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
	"github.com/Flgado/trino-insights-mcp/pkg/translations"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type analyzeQueryArgs struct {
	QueryID string `json:"query_id"`
}

func AnalyzeQueryTool(t translations.HelperFunc) inventory.ServerTool {
	return inventory.NewServerToolWithDeps[analyzeQueryArgs, ToolDependencies](
		mcp.Tool{
			Name: "analyze_query",
			Description: t("TOOL_ANALYZE_QUERY_DESC",
				"Analyze a single Trino query: fetch its metrics from the coordinator, "+
					"project them to compact facts, and run the rule engine to detect "+
					"performance issues (CPU-bound, skew, memory pressure, spill, etc.). "+
					"Returns a headline, findings with evidence, and the underlying facts "+
					"including per-scan pushdown details (which predicates the connector accepted "+
					"vs. which Trino filters locally). "+
					"Always follow up with get_query_sql for a non-trivial diagnosis."),
			Annotations: &mcp.ToolAnnotations{
				Title:        t("TOOL_ANALYZE_QUERY_TITLE", "Analyze query"),
				ReadOnlyHint: true,
			},
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query_id": {
						"type": "string",
						"description": "The Trino query ID (e.g. 20260419_080123_00042_abcde)"
					}
				},
				"required": ["query_id"]
			}`),
		},
		ToolsetMetadataPlans,
		func(deps ToolDependencies) func(ctx context.Context, req *mcp.CallToolRequest, args analyzeQueryArgs) (*mcp.CallToolResult, error) {
			engine := rules.DefaultEngine()
			return func(ctx context.Context, _ *mcp.CallToolRequest, args analyzeQueryArgs) (*mcp.CallToolResult, error) {
				if args.QueryID == "" {
					return toolResultError("query_id is required"), nil
				}

				qi, err := deps.QueryFetcher().Fetch(ctx, args.QueryID)
				if err != nil {
					return toolResultError(fmt.Sprintf("failed to fetch query: %v", err)), nil
				}

				facts := queryinfo.Project(qi)
				findings := engine.Run(facts)
				sort.Sort(findings)

				headline := buildHeadline(facts, findings)
				nextStep := ""
				if len(findings) > 0 {
					nextStep = "Call get_query_sql to read the SQL and connect the findings to specific joins, scans, or expressions."
				}

				analysis := QueryAnalysis{
					Headline: headline,
					NextStep: nextStep,
					Findings: findings,
					Facts:    facts,
				}

				data, err := json.Marshal(analysis)
				if err != nil {
					return toolResultError(fmt.Sprintf("failed to marshal analysis: %v", err)), nil
				}
				return toolResultText(string(data)), nil
			}
		},
	)
}

func buildHeadline(facts *queryinfo.QueryFacts, findings diagnose.Findings) string {
	if facts.State == "FAILED" {
		headline := fmt.Sprintf("FAILED (%s)", facts.State)
		if facts.ErrorType != "" {
			headline = fmt.Sprintf("FAILED with %s", facts.ErrorType)
		}
		if facts.ErrorCodeName != "" {
			headline += fmt.Sprintf(": %s", facts.ErrorCodeName)
		}
		return headline
	}

	if len(findings) == 0 {
		return fmt.Sprintf("Query %s — metric-clean (%s, elapsed %d ms, CPU %d ms)",
			facts.QueryID, facts.State, facts.Time.ElapsedMs, facts.Time.TotalCPUMs)
	}

	// Compose a richer headline from the top 1-2 findings
	parts := make([]string, 0, 2)
	seen := 0
	for i := range findings {
		if seen >= 2 {
			break
		}
		parts = append(parts, findings[i].Title)
		seen++
	}

	headline := parts[0]
	if len(parts) > 1 {
		headline += "; " + parts[1]
	}
	return headline
}

func toolResultText(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
}

func toolResultError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
		IsError: true,
	}
}
