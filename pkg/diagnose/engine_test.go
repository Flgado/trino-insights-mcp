package diagnose

import (
	"testing"

	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

type fakeRule struct {
	id      string
	finding *Finding
}

func (f *fakeRule) ID() string                            { return f.id }
func (f *fakeRule) Eval(_ *queryinfo.QueryFacts) *Finding { return f.finding }

func TestNewEngine(t *testing.T) {
	e := NewEngine()
	if e == nil {
		t.Fatal("NewEngine() returned nil")
	}
}

func TestEngine_Run_NoRules(t *testing.T) {
	e := NewEngine()
	results := e.Run(&queryinfo.QueryFacts{})
	if len(results) != 0 {
		t.Errorf("expected 0 findings, got %d", len(results))
	}
}

func TestEngine_Run_RuleFires(t *testing.T) {
	rule := &fakeRule{
		id: "test.rule",
		finding: &Finding{
			RuleID:   "test.rule",
			Severity: SeverityWarn,
			Title:    "Test warning",
		},
	}
	e := NewEngine(rule)
	results := e.Run(&queryinfo.QueryFacts{})

	if len(results) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(results))
	}
	if results[0].RuleID != "test.rule" {
		t.Errorf("RuleID = %q, want %q", results[0].RuleID, "test.rule")
	}
}

func TestEngine_Run_RuleDoesNotFire(t *testing.T) {
	rule := &fakeRule{id: "quiet.rule", finding: nil}
	e := NewEngine(rule)
	results := e.Run(&queryinfo.QueryFacts{})

	if len(results) != 0 {
		t.Errorf("expected 0 findings for non-firing rule, got %d", len(results))
	}
}

func TestEngine_Run_MultipleRules(t *testing.T) {
	rules := []Rule{
		&fakeRule{id: "rule1", finding: &Finding{RuleID: "rule1", Severity: SeverityInfo, Title: "Info"}},
		&fakeRule{id: "rule2", finding: nil},
		&fakeRule{id: "rule3", finding: &Finding{RuleID: "rule3", Severity: SeverityCritical, Title: "Critical"}},
	}
	e := NewEngine(rules...)
	results := e.Run(&queryinfo.QueryFacts{})

	if len(results) != 2 {
		t.Fatalf("expected 2 findings (rule2 skipped), got %d", len(results))
	}

	if results[0].RuleID != "rule3" {
		t.Errorf("first finding should be critical (rule3), got %q", results[0].RuleID)
	}
	if results[1].RuleID != "rule1" {
		t.Errorf("second finding should be info (rule1), got %q", results[1].RuleID)
	}
}

func TestEngine_Run_SortsBySeverity(t *testing.T) {
	rules := []Rule{
		&fakeRule{id: "info", finding: &Finding{RuleID: "info", Severity: SeverityInfo}},
		&fakeRule{id: "critical", finding: &Finding{RuleID: "critical", Severity: SeverityCritical}},
		&fakeRule{id: "warn", finding: &Finding{RuleID: "warn", Severity: SeverityWarn}},
	}
	e := NewEngine(rules...)
	results := e.Run(&queryinfo.QueryFacts{})

	if len(results) != 3 {
		t.Fatalf("expected 3 findings, got %d", len(results))
	}
	if results[0].Severity != SeverityCritical {
		t.Errorf("results[0].Severity = %q, want critical", results[0].Severity)
	}
	if results[1].Severity != SeverityWarn {
		t.Errorf("results[1].Severity = %q, want warn", results[1].Severity)
	}
	if results[2].Severity != SeverityInfo {
		t.Errorf("results[2].Severity = %q, want info", results[2].Severity)
	}
}

func TestEngine_Run_NilFacts(t *testing.T) {
	rule := &fakeRule{id: "test", finding: &Finding{RuleID: "test", Severity: SeverityInfo}}
	e := NewEngine(rule)
	results := e.Run(nil)
	if len(results) != 1 {
		t.Errorf("expected 1 finding even with nil facts, got %d", len(results))
	}
}
