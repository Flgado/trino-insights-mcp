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
	PreparedQuery string `json:"prepared_query,omitempty"`
	// PreparedQueryWithLiterals is the prepared_query template with each ?
	// placeholder substituted by the corresponding USING literal from `sql`.
	// Populated only when the query uses a prepared statement and the
	// substitution succeeds (correct ? count, no unterminated strings).
	// Agents should prefer this field when reconstructing the executed SQL
	// for a report — it removes positional-argument mistakes.
	PreparedQueryWithLiterals string `json:"prepared_query_with_literals,omitempty"`
	Truncated                 bool   `json:"truncated"`
	OriginalBytes             int    `json:"original_bytes"`
	ReturnedBytes             int    `json:"returned_bytes"`
	Source                    string `json:"source"`
}

func GetQuerySQLTool(t translations.HelperFunc) inventory.ServerTool {
	return inventory.NewServerToolWithDeps[getQuerySQLArgs, ToolDependencies](
		mcp.Tool{
			Name: "get_query_sql",
			Description: t("TOOL_GET_QUERY_SQL_DESC",
				"Return the full SQL text of a Trino query, sanitized and truncated "+
					"to the configured content-window-size (default 16 KiB, max 64 KiB). "+
					"When the query uses a prepared statement (EXECUTE ... USING), the response "+
					"includes both the executed SQL (with parameter values) in 'sql' and the "+
					"parameterized template (with ? placeholders) in 'prepared_query'. "+
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

				budget := effectiveSQLBudget(deps.ContentWindowSize)
				originalBytes := len(sql)
				truncated := false

				preparedQuery := qi.PreparedQuery
				if preparedQuery != "" {
					source = "PreparedQuery"
					originalBytes += len(preparedQuery)
				}

				if len(sql)+len(preparedQuery) > budget {
					truncated = true
					if preparedQuery != "" {
						pqBudget := budget - len(sql)
						if pqBudget < MinSQLBudgetBytes {
							pqBudget = MinSQLBudgetBytes
						}
						if len(preparedQuery) > pqBudget {
							preparedQuery = preparedQuery[:pqBudget]
						}
						remaining := budget - len(preparedQuery)
						if remaining > 0 && len(sql) > remaining {
							sql = sql[:remaining]
						}
					} else {
						sql = sql[:budget]
					}
				}

				// Best-effort literal substitution. Computed against the
				// (possibly truncated) sql + preparedQuery — if either was
				// truncated, the substitution may end up incomplete; in that
				// case we leave the field empty rather than ship a broken SQL.
				var preparedWithLiterals string
				if preparedQuery != "" && !truncated {
					preparedWithLiterals = SubstitutePreparedLiterals(sql, preparedQuery)
				}

				result := getQuerySQLResult{
					QueryID:                   args.QueryID,
					SQL:                       sql,
					PreparedQuery:             preparedQuery,
					PreparedQueryWithLiterals: preparedWithLiterals,
					Truncated:                 truncated,
					OriginalBytes:             originalBytes,
					ReturnedBytes:             len(sql) + len(preparedQuery),
					Source:                    source,
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
