package rules

import (
	"fmt"

	"github.com/Flgado/trino-insights-mcp/pkg/diagnose"
	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

// ScanTooLarge fires when physical input exceeds thresholds:
// >= 1 billion rows OR >= 100 GiB scanned.
type ScanTooLarge struct {
	MaxRows  int   // default 1_000_000_000
	MaxBytes int64 // default 100 GiB
}

func (r ScanTooLarge) ID() string { return "trino.scan-too-large" }

func (r ScanTooLarge) maxRows() int {
	if r.MaxRows <= 0 {
		return 1_000_000_000
	}
	return r.MaxRows
}

func (r ScanTooLarge) maxBytes() int64 {
	if r.MaxBytes <= 0 {
		return 100 * (1 << 30) // 100 GiB
	}
	return r.MaxBytes
}

func (r ScanTooLarge) Eval(facts *queryinfo.QueryFacts) *diagnose.Finding {
	rows := facts.IO.PhysicalInputPositions
	bytes := facts.IO.PhysicalInputBytes

	rowsOver := rows >= r.maxRows()
	bytesOver := bytes >= r.maxBytes()

	if !rowsOver && !bytesOver {
		return nil
	}

	bytesGiB := float64(bytes) / (1024 * 1024 * 1024)

	return &diagnose.Finding{
		RuleID:   "trino.scan-too-large",
		Severity: diagnose.SeverityWarn,
		Title:    "Very large scan",
		Details:  fmt.Sprintf("Scanned %d rows (%.1f GiB). Consider partition pruning, predicate pushdown, or materializing intermediate results.", rows, bytesGiB),
		Evidence: map[string]any{
			"physical_input_rows":  rows,
			"physical_input_bytes": bytes,
			"physical_input_gib":   bytesGiB,
		},
		Remediation: "Add WHERE clauses that align with partition columns, use TABLESAMPLE for exploration, or pre-aggregate into a summary table.",
	}
}
