package diagnose

import (
	"sort"

	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

// Rule evaluates a single diagnostic check against QueryFacts.
// Returns nil if the rule does not fire.
type Rule interface {
	ID() string
	Eval(facts *queryinfo.QueryFacts) *Finding
}

// Engine holds an ordered set of rules and runs them all against a query.
type Engine struct {
	rules []Rule
}

func NewEngine(rules ...Rule) *Engine {
	return &Engine{rules: rules}
}

// Run evaluates every rule and returns findings sorted by severity (critical first).
func (e *Engine) Run(facts *queryinfo.QueryFacts) Findings {
	var results Findings
	for _, r := range e.rules {
		if f := r.Eval(facts); f != nil {
			results = append(results, *f)
		}
	}
	sort.Sort(results)
	return results
}
