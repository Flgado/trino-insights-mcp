package diagnose

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarn     Severity = "warn"
	SeverityCritical Severity = "critical"
)

func (s Severity) Order() int {
	switch s {
	case SeverityCritical:
		return 3
	case SeverityWarn:
		return 2
	case SeverityInfo:
		return 1
	default:
		return 0
	}
}

type Finding struct {
	RuleID      string   `json:"rule_id"`
	Severity    Severity `json:"severity"`
	Title       string   `json:"title"`
	Details     string   `json:"details"`
	Evidence    any      `json:"evidence,omitempty"`
	Remediation string   `json:"remediation,omitempty"`
}

type Findings []Finding

func (f Findings) Len() int      { return len(f) }
func (f Findings) Swap(i, j int) { f[i], f[j] = f[j], f[i] }
func (f Findings) Less(i, j int) bool {
	return f[i].Severity.Order() > f[j].Severity.Order()
}

func (f Findings) Worst() *Finding {
	if len(f) == 0 {
		return nil
	}
	best := &f[0]
	for i := 1; i < len(f); i++ {
		if f[i].Severity.Order() > best.Severity.Order() {
			best = &f[i]
		}
	}
	return best
}
