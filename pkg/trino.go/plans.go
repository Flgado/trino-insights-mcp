package trino

import (
	"github.com/Flgado/trino-insights-mcp/pkg/diagnose"
	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

const MaxQuerySQLBytes = 64 * 1024
const MinSQLBudgetBytes = 2 * 1024

func effectiveSQLBudget(contentWindowSize int) int {
	if contentWindowSize <= 0 {
		return MinSQLBudgetBytes
	}

	if contentWindowSize > MaxQuerySQLBytes {
		return MaxQuerySQLBytes
	}

	if contentWindowSize < MinSQLBudgetBytes {
		return MinSQLBudgetBytes
	}

	return contentWindowSize
}

type QueryAnalysis struct {
	Headline string                `json:"headline"`
	NextStep string                `json:"next_step,omitempty"`
	Findings diagnose.Findings     `json:"findings"`
	Facts    *queryinfo.QueryFacts `json:"facts"`
}
