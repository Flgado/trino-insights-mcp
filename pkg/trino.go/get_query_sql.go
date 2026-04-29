package trino

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Flgado/trino-insights-mcp/pkg/inventory"
	"github.com/Flgado/trino-insights-mcp/pkg/translations"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type getQuerySQLArgs struct {
	QueryID string `json:"query_id"`
}

type getQuerySQLResult struct {
	QueryID       string `json:"query_id"`
	SQL           string `json:"sql"`
	Truncated     bool   `json:"truncated"`
	OriginalBytes int    `json:"original_bytes"`
	ReturnedBytes int    `json:"returned_bytes"`
	Source        string `json:"source"`
}

func GetQuerySQLTool(t translations.HelperFunc) inventory.ServerTool {
	return inventory.NewServerToolWithDeps[getQuerySQLArgs, ToolDependencies](
		mcp.Tool{
			Name: "get_query_sql",
			Description: t("TOOL_GET_QUERY_SQL_DESC",
				"Return the full SQL text of a Trino query, sanitized and truncated "+
					"to the configured content-window-size (default 16 KiB, max 64 KiB). "+
					"Call this RIGHT AFTER analyze_query for any non-trivial diagnosis — "+
					"you cannot explain 'stage 1 HashJoin is skewed' without reading which join key is involved."),
			Annotations: &mcp.ToolAnnotations{
				Title:        t("TOOL_GET_QUERY_SQL_TITLE", "Get query SQL"),
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
		func(deps ToolDependencies) func(ctx context.Context, req *mcp.CallToolRequest, args getQuerySQLArgs) (*mcp.CallToolResult, error) {
			return func(ctx context.Context, _ *mcp.CallToolRequest, args getQuerySQLArgs) (*mcp.CallToolResult, error) {
				if args.QueryID == "" {
					return toolResultError("query_id is required"), nil
				}

				qi, err := deps.QueryFetcher().Fetch(ctx, args.QueryID)
				if err != nil {
					return toolResultError(fmt.Sprintf("failed to fetch query: %v", err)), nil
				}

				sql := qi.Query
				source := "Query"
				if sql == "" {
					sql = qi.QueryTextPreview
					source = "QueryTextPreview"
				}
				if sql == "" {
					return toolResultError("no SQL text available for this query"), nil
				}

				originalBytes := len(sql)
				budget := effectiveSQLBudget(deps.ContentWindowSize)
				truncated := false

				if len(sql) > budget {
					sql = sql[:budget]
					truncated = true
				}

				result := getQuerySQLResult{
					QueryID:       args.QueryID,
					SQL:           sql,
					Truncated:     truncated,
					OriginalBytes: originalBytes,
					ReturnedBytes: len(sql),
					Source:        source,
				}

				data, err := json.Marshal(result)
				if err != nil {
					return toolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
				}
				return toolResultText(string(data)), nil
			}
		},
	)
}
