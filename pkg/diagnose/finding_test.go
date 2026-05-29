package diagnose

import (
	"sort"
	"testing"
)

func TestSeverity_Order(t *testing.T) {
	tests := []struct {
		sev  Severity
		want int
	}{
		{SeverityCritical, 3},
		{SeverityWarn, 2},
		{SeverityInfo, 1},
		{Severity("unknown"), 0},
		{Severity(""), 0},
	}
	for _, tt := range tests {
		t.Run(string(tt.sev), func(t *testing.T) {
			if got := tt.sev.Order(); got != tt.want {
				t.Errorf("Order() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSeverity_Ordering(t *testing.T) {
	if SeverityCritical.Order() <= SeverityWarn.Order() {
		t.Error("critical should outrank warn")
	}
	if SeverityWarn.Order() <= SeverityInfo.Order() {
		t.Error("warn should outrank info")
	}
}

func TestFindings_Sort(t *testing.T) {
	fs := Findings{
		{RuleID: "info-rule", Severity: SeverityInfo},
		{RuleID: "critical-rule", Severity: SeverityCritical},
		{RuleID: "warn-rule", Severity: SeverityWarn},
	}
	sort.Sort(fs)

	if fs[0].RuleID != "critical-rule" {
		t.Errorf("first should be critical, got %q", fs[0].RuleID)
	}
	if fs[1].RuleID != "warn-rule" {
		t.Errorf("second should be warn, got %q", fs[1].RuleID)
	}
	if fs[2].RuleID != "info-rule" {
		t.Errorf("third should be info, got %q", fs[2].RuleID)
	}
}

func TestFindings_Sort_SameSeverity(t *testing.T) {
	fs := Findings{
		{RuleID: "a", Severity: SeverityWarn},
		{RuleID: "b", Severity: SeverityWarn},
	}
	sort.Sort(fs)
	if fs.Len() != 2 {
		t.Errorf("Len() = %d, want 2", fs.Len())
	}
}

func TestFindings_Sort_Empty(t *testing.T) {
	var fs Findings
	sort.Sort(fs)
	if fs.Len() != 0 {
		t.Errorf("Len() = %d, want 0", fs.Len())
	}
}

func TestFindings_Worst_Critical(t *testing.T) {
	fs := Findings{
		{RuleID: "info", Severity: SeverityInfo},
		{RuleID: "critical", Severity: SeverityCritical},
		{RuleID: "warn", Severity: SeverityWarn},
	}
	w := fs.Worst()
	if w == nil {
		t.Fatal("Worst() returned nil")
	}
	if w.RuleID != "critical" {
		t.Errorf("Worst().RuleID = %q, want %q", w.RuleID, "critical")
	}
}

func TestFindings_Worst_SingleElement(t *testing.T) {
	fs := Findings{
		{RuleID: "only", Severity: SeverityInfo},
	}
	w := fs.Worst()
	if w.RuleID != "only" {
		t.Errorf("Worst().RuleID = %q, want %q", w.RuleID, "only")
	}
}

func TestFindings_Worst_Empty(t *testing.T) {
	var fs Findings
	if w := fs.Worst(); w != nil {
		t.Errorf("Worst() on empty should be nil, got %v", w)
	}
}

func TestFindings_Worst_AllSameSeverity(t *testing.T) {
	fs := Findings{
		{RuleID: "a", Severity: SeverityWarn},
		{RuleID: "b", Severity: SeverityWarn},
		{RuleID: "c", Severity: SeverityWarn},
	}
	w := fs.Worst()
	if w == nil {
		t.Fatal("Worst() returned nil")
	}
	if w.Severity != SeverityWarn {
		t.Errorf("Worst().Severity = %q, want %q", w.Severity, SeverityWarn)
	}
}

func TestFindings_Len(t *testing.T) {
	fs := Findings{{}, {}, {}}
	if fs.Len() != 3 {
		t.Errorf("Len() = %d, want 3", fs.Len())
	}
}

func TestFindings_Swap(t *testing.T) {
	fs := Findings{
		{RuleID: "first"},
		{RuleID: "second"},
	}
	fs.Swap(0, 1)
	if fs[0].RuleID != "second" || fs[1].RuleID != "first" {
		t.Error("Swap did not work")
	}
}

func TestFinding_Fields(t *testing.T) {
	f := Finding{
		RuleID:      "trino.test",
		Severity:    SeverityCritical,
		Title:       "Test finding",
		Details:     "Some details",
		Evidence:    map[string]int{"count": 42},
		Remediation: "Fix it",
	}

	if f.RuleID != "trino.test" {
		t.Errorf("RuleID = %q", f.RuleID)
	}
	if f.Severity != SeverityCritical {
		t.Errorf("Severity = %q", f.Severity)
	}
	if f.Title != "Test finding" {
		t.Errorf("Title = %q", f.Title)
	}
	if f.Remediation != "Fix it" {
		t.Errorf("Remediation = %q", f.Remediation)
	}
}
