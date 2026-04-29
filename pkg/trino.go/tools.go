package trino

import (
	"github.com/Flgado/trino-insights-mcp/pkg/inventory"
	"github.com/Flgado/trino-insights-mcp/pkg/translations"
)

var ToolsetMetadataPlans = inventory.ToolsetMetadata{
	ID:          "plans",
	Description: "Per-query diagnostics: fetch QueryInfo, project to slim QueryFacts, run the rule engine, return findings + remediation.",
	Default:     true,
	InstructionsFunc: func(_ *inventory.Inventory) string {
		return plansToolsetInstructions
	},
}

func AllTools(t translations.HelperFunc) []inventory.ServerTool {
	return []inventory.ServerTool{
		AnalyzeQueryTool(t),
		GetQuerySQLTool(t),
	}
}
