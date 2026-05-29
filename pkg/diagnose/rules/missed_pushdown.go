package rules

import (
	"fmt"
	"strings"

	"github.com/Flgado/trino-insights-mcp/pkg/diagnose"
	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

// importantPushdownRules lists optimizer rule name fragments that, when failed,
// indicate a missed pushdown or predicate optimisation opportunity.
var importantPushdownRules = []string{
	"PushPredicateIntoTableScan",
	"PushProjectionIntoTableScan",
	"PushAggregationIntoTableScan",
	"PushTopNIntoTableScan",
	"PushLimitIntoTableScan",
	"PushFilterIntoTableScan",
	"PushDownPredicate",
	"PruneTableScanColumns",
}

// MissedPushdown fires when important optimizer rules were invoked but never
// applied, suggesting the engine couldn't push work down to the connector.
type MissedPushdown struct {
	MinInvocations int // minimum invocations to care (default 1)
}

func (r MissedPushdown) ID() string { return "trino.missed-pushdown" }

func (r MissedPushdown) minInvocations() int {
	if r.MinInvocations <= 0 {
		return 1
	}
	return r.MinInvocations
}

func (r MissedPushdown) Eval(facts *queryinfo.QueryFacts) *diagnose.Finding {
	var missed []string
	for _, rule := range facts.OptimizerRules {
		if rule.Invocations < r.minInvocations() {
			continue
		}
		if rule.Applied > 0 {
			continue
		}
		if isImportantRule(rule.Rule) {
			missed = append(missed, rule.Rule)
		}
	}

	if len(missed) == 0 {
		return nil
	}

	title := fmt.Sprintf("%d pushdown rule(s) invoked but never applied", len(missed))
	details := fmt.Sprintf(
		"The following optimizer rules were invoked but never applied: %s. "+
			"This may mean the connector does not support the requested pushdown, "+
			"or the query shape prevents the optimisation.",
		strings.Join(missed, ", "),
	)

	return &diagnose.Finding{
		RuleID:   "trino.missed-pushdown",
		Severity: diagnose.SeverityInfo,
		Title:    title,
		Details:  details,
		Evidence: map[string]any{
			"missed_rules": missed,
		},
		Remediation: "Check if the connector supports predicate/projection/aggregation pushdown. " +
			"For connectors like memory or Kafka, pushdown is not available. " +
			"For Hive/Iceberg, ensure partition columns are used in WHERE clauses.",
	}
}

func isImportantRule(rule string) bool {
	for _, important := range importantPushdownRules {
		if strings.Contains(rule, important) {
			return true
		}
	}
	return false
}
