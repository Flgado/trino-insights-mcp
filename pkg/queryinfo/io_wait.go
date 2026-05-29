package queryinfo

import (
	"fmt"
	"sort"
	"strings"
)

// TopStagesLimit caps how many entries TopIOStages / TopCPUStages contain.
const TopStagesLimit = 5

// ConnectorIOFact is a per-connector roll-up of the I/O wait, scan count,
// and row totals across every scan in the query. It answers the question
// "which back-end spent the most time answering Trino?" without making the
// agent sort and aggregate facts.scan_pushdown[] itself.
//
// Sorted descending by IOWaitMs in the final QueryFacts payload.
type ConnectorIOFact struct {
	ConnectorType string `json:"connector_type"`
	ScanCount     int    `json:"scan_count"`
	IOWaitMs      int64  `json:"io_wait_ms,omitempty"`
	RowsIn        int64  `json:"rows_in,omitempty"`
	RowsOut       int64  `json:"rows_out,omitempty"`
	BytesIn       int64  `json:"bytes_in,omitempty"`
}

// TopStageFact is a compact, pre-ranked summary of one stage for the
// QueryFacts.TopIOStages and TopCPUStages lists. The Rationale is a
// human-readable one-liner generated from the underlying scan_pushdown +
// stage facts — it is meant to be quotable straight into a report.
type TopStageFact struct {
	StageID       string `json:"stage_id"`
	ConnectorType string `json:"connector_type,omitempty"`
	Table         string `json:"table,omitempty"`
	IOWaitMs      int64  `json:"io_wait_ms,omitempty"`
	CPUMs         int64  `json:"cpu_ms,omitempty"`
	Rationale     string `json:"rationale,omitempty"`
}

// IOWaitKind labels the source of a StageFact.IOWaitMs value so the agent
// renders it correctly ("Mongo round-trip" vs. "blocked on upstream").
const (
	IOWaitKindPhysicalRead      = "physical_read"       // from physicalInputReadTime — the real connector read time
	IOWaitKindScheduledMinusCPU = "scheduled_minus_cpu" // proxy for scan stages when physicalInputReadTime is missing
	IOWaitKindBlocked           = "blocked"             // downstream stage parked on upstream rows
)

// deriveIOWait computes the I/O wait time for a stage and labels its source.
// Priority order:
//
//  1. physical_input_read_time_ms — the connector-side read time as measured
//     by Trino. Always preferred for scan stages.
//  2. scheduled_ms − cpu_ms — proxy for connector wait when (1) is missing
//     but the stage has physical inputs (some old Trino versions don't emit
//     physicalInputReadTime).
//  3. blocked_ms — downstream / exchange / join stages parked waiting on
//     upstream rows. This is "I/O wait propagated from below," not direct
//     connector wait.
func deriveIOWait(sf StageFact) (int64, string) {
	if sf.PhysicalInputReadTimeMs > 0 {
		return sf.PhysicalInputReadTimeMs, IOWaitKindPhysicalRead
	}
	if sf.PhysicalInputPos > 0 {
		wait := sf.TotalScheduledMs - sf.TotalCPUMs
		if wait < 0 {
			wait = 0
		}
		return wait, IOWaitKindScheduledMinusCPU
	}
	return sf.TotalBlockedMs, IOWaitKindBlocked
}

// buildConnectorIORollup aggregates per-stage I/O wait and per-scan rows /
// bytes into a per-connector summary, sorted descending by IOWaitMs.
//
// A stage's IOWaitMs is attributed to a connector at most once even when the
// stage contains multiple scans on the same connector (rare — usually a fused
// ScanFilterProject + a TableScan in a build-side branch). Multi-connector
// stages are unattested in real Trino plans, so we don't try to apportion.
func buildConnectorIORollup(stages []StageFact, scans []ScanPushdownFact) []ConnectorIOFact {
	if len(scans) == 0 {
		return nil
	}

	stageByID := make(map[string]StageFact, len(stages))
	for _, s := range stages {
		stageByID[s.StageID] = s
	}

	agg := make(map[string]*ConnectorIOFact)
	stageCharged := make(map[string]bool) // dedupe by stage_id|connector_type

	for _, scan := range scans {
		if scan.ConnectorType == "" {
			continue
		}
		rec := agg[scan.ConnectorType]
		if rec == nil {
			rec = &ConnectorIOFact{ConnectorType: scan.ConnectorType}
			agg[scan.ConnectorType] = rec
		}
		rec.ScanCount++
		rec.RowsIn += scan.PhysicalInputPositions
		rec.RowsOut += scan.OutputRows

		key := scan.StageID + "|" + scan.ConnectorType
		if stageCharged[key] {
			continue
		}
		stageCharged[key] = true
		if sf, ok := stageByID[scan.StageID]; ok {
			rec.IOWaitMs += sf.IOWaitMs
			rec.BytesIn += sf.PhysicalInputBytes
		}
	}

	out := make([]ConnectorIOFact, 0, len(agg))
	for _, rec := range agg {
		out = append(out, *rec)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IOWaitMs != out[j].IOWaitMs {
			return out[i].IOWaitMs > out[j].IOWaitMs
		}
		return out[i].ConnectorType < out[j].ConnectorType
	})
	return out
}

// buildTopIOStages returns the top-N stages by IOWaitMs in descending order.
// Each entry has a generated Rationale meant to be quotable straight into a
// diagnostic report. Stages with zero IOWaitMs are excluded.
func buildTopIOStages(stages []StageFact, scans []ScanPushdownFact, n int) []TopStageFact {
	scanByStage := primaryScanByStage(scans)

	type entry struct {
		idx    int
		iowait int64
	}
	entries := make([]entry, 0, len(stages))
	for i, sf := range stages {
		if sf.IOWaitMs == 0 {
			continue
		}
		entries = append(entries, entry{i, sf.IOWaitMs})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].iowait > entries[j].iowait })
	if len(entries) > n {
		entries = entries[:n]
	}

	out := make([]TopStageFact, 0, len(entries))
	for _, e := range entries {
		sf := stages[e.idx]
		out = append(out, buildTopStageFact(sf, scanByStage, true))
	}
	return out
}

// buildTopCPUStages returns the top-N stages by TotalCPUMs in descending
// order. Stages with zero CPUMs are excluded.
func buildTopCPUStages(stages []StageFact, scans []ScanPushdownFact, n int) []TopStageFact {
	scanByStage := primaryScanByStage(scans)

	type entry struct {
		idx int
		cpu int64
	}
	entries := make([]entry, 0, len(stages))
	for i, sf := range stages {
		if sf.TotalCPUMs == 0 {
			continue
		}
		entries = append(entries, entry{i, sf.TotalCPUMs})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].cpu > entries[j].cpu })
	if len(entries) > n {
		entries = entries[:n]
	}

	out := make([]TopStageFact, 0, len(entries))
	for _, e := range entries {
		sf := stages[e.idx]
		out = append(out, buildTopStageFact(sf, scanByStage, false))
	}
	return out
}

// primaryScanByStage picks the most-illuminating scan per stage_id when there
// are multiple. "Most illuminating" = the scan that read the most rows, since
// that one carries the bulk of the I/O for the stage.
func primaryScanByStage(scans []ScanPushdownFact) map[string]ScanPushdownFact {
	out := make(map[string]ScanPushdownFact, len(scans))
	for _, s := range scans {
		if existing, ok := out[s.StageID]; ok && existing.PhysicalInputPositions >= s.PhysicalInputPositions {
			continue
		}
		out[s.StageID] = s
	}
	return out
}

func buildTopStageFact(sf StageFact, scanByStage map[string]ScanPushdownFact, ioFocused bool) TopStageFact {
	ts := TopStageFact{
		StageID:  sf.StageID,
		IOWaitMs: sf.IOWaitMs,
		CPUMs:    sf.TotalCPUMs,
	}
	scan, hasScan := scanByStage[sf.StageID]
	if hasScan {
		ts.ConnectorType = scan.ConnectorType
		ts.Table = formatTableName(scan.Catalog, scan.Schema, scan.Table)
		ts.Rationale = generateScanRationale(sf, scan, ioFocused)
		return ts
	}
	ts.Rationale = generateDownstreamRationale(sf, ioFocused)
	return ts
}

// generateScanRationale produces a one-liner describing what a scan stage did,
// suitable for direct quotation in a report. Keeps things short: 60-120
// characters typical.
func generateScanRationale(sf StageFact, scan ScanPushdownFact, ioFocused bool) string {
	parts := make([]string, 0, 4)

	if scan.PhysicalInputPositions > 0 {
		if scan.OutputRows > 0 && scan.OutputRows != scan.PhysicalInputPositions {
			parts = append(parts, fmt.Sprintf("read %d rows, emitted %d after local filter", scan.PhysicalInputPositions, scan.OutputRows))
		} else {
			parts = append(parts, fmt.Sprintf("read %d rows", scan.PhysicalInputPositions))
		}
	}
	if sf.PhysicalInputBytes > 0 {
		parts = append(parts, fmt.Sprintf("%d bytes", sf.PhysicalInputBytes))
	}
	if scan.LocalFilter != "" {
		parts = append(parts, "local filter: "+truncate(scan.LocalFilter, 80))
	}
	if !ioFocused && sf.TotalCPUMs > 0 {
		parts = append(parts, fmt.Sprintf("cpu %d ms", sf.TotalCPUMs))
	}

	if len(parts) == 0 {
		// Fall back to something always-true so the entry is still informative.
		return fmt.Sprintf("scan stage on %s", scan.ConnectorType)
	}
	return strings.Join(parts, "; ")
}

// generateDownstreamRationale handles join / aggregation / exchange stages
// whose I/O wait is really "blocked waiting on upstream rows." Names the
// upstream stages by their short suffix so the rationale reads naturally
// (e.g. "blocked on .5/.10/.11" rather than the full prefixed IDs).
func generateDownstreamRationale(sf StageFact, ioFocused bool) string {
	if !ioFocused && sf.TotalCPUMs > 0 {
		return fmt.Sprintf("%s; cpu %d ms", downstreamWaitDescription(sf), sf.TotalCPUMs)
	}
	return downstreamWaitDescription(sf)
}

func downstreamWaitDescription(sf StageFact) string {
	if len(sf.SubStageIDs) == 0 {
		op := sf.PrimaryOperator
		if op == "" {
			op = "downstream stage"
		}
		return op + " with no upstream"
	}
	shorts := make([]string, 0, len(sf.SubStageIDs))
	for _, id := range sf.SubStageIDs {
		shorts = append(shorts, "."+StageShortID(id))
	}
	op := sf.PrimaryOperator
	if op == "" {
		op = "stage"
	}
	return fmt.Sprintf("%s blocked on %s", op, strings.Join(shorts, "/"))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}
