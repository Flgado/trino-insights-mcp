package rules

import (
	"fmt"

	"github.com/Flgado/trino-insights-mcp/pkg/diagnose"
	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

type Failed struct{}

func (Failed) ID() string { return "trino.failed" }

func (Failed) Eval(facts *queryinfo.QueryFacts) *diagnose.Finding {
	if facts.State != "FAILED" {
		return nil
	}

	detail := fmt.Sprintf("Query %s FAILED", facts.QueryID)
	if facts.ErrorType != "" {
		detail += fmt.Sprintf(" with error type %s", facts.ErrorType)
	}
	if facts.ErrorCodeName != "" {
		detail += fmt.Sprintf(" (code: %s)", facts.ErrorCodeName)
	}

	return &diagnose.Finding{
		RuleID:   "trino.failed",
		Severity: diagnose.SeverityCritical,
		Title:    "Query failed",
		Details:  detail,
		Evidence: map[string]any{
			"error_type":      facts.ErrorType,
			"error_code_name": facts.ErrorCodeName,
		},
		Remediation: "Check the error code and message. Common causes: insufficient resources (NO_NODES_AVAILABLE), permissions (PERMISSION_DENIED), syntax errors, or data issues.",
	}
}
