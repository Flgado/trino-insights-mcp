package queryinfo

import (
	"encoding/json"
	"strings"
)

// ScanPushdownFact describes a single TableScan / ScanFilterProject node in
// the query plan, extracting the connector-side pushed predicates and the
// Trino-side local filter (the part the connector refused to push).
//
// This is the structural data needed to answer "what was pushed down vs. what
// is being filtered in Trino" without re-running the query or invoking EXPLAIN.
// All fields are populated best-effort from QueryInfo.outputStage[].plan.jsonRepresentation
// which already ships in the single REST payload the MCP fetches.
type ScanPushdownFact struct {
	StageID    string `json:"stage_id"`
	PlanNodeID string `json:"plan_node_id,omitempty"`
	NodeName   string `json:"node_name"`

	// Table identity, parsed from descriptor.table (e.g. "catalog:schema.table")
	Catalog       string `json:"catalog,omitempty"`
	Schema        string `json:"schema,omitempty"`
	Table         string `json:"table,omitempty"`
	ConnectorType string `json:"connector_type,omitempty"`

	// PushedConstraintColumns are the columns the connector accepted as a
	// constraint — parsed from "constraint on [c1, c2, ...]" details line.
	// This tells you WHICH columns the connector took, not the literal values.
	PushedConstraintColumns []string `json:"pushed_constraint_columns,omitempty"`

	// PushedDetails are the raw details strings that survive after dropping the
	// generic "estimates" / "stats" lines — useful for spotting connector-specific
	// info like "limit = 100", "topN = ...", or the literal predicate when the
	// connector emits one.
	PushedDetails []string `json:"pushed_details,omitempty"`

	// LocalFilter is descriptor.filterPredicate: the Trino-side filter that
	// the connector refused to push. When this is non-empty AND PhysicalInputPositions
	// is much larger than OutputRows, the scan is over-fetching.
	LocalFilter string `json:"local_filter,omitempty"`

	// Linked operator metrics (when the matching scan operator can be found by planNodeId).
	PhysicalInputPositions int64   `json:"physical_input_positions,omitempty"`
	OutputRows             int64   `json:"output_rows,omitempty"`
	Selectivity            float64 `json:"selectivity,omitempty"`
}

// scanNodeNames is the set of plan-node names we treat as a table scan.
// Trino fuses scans with predicates/projections so we have to handle all variants.
var scanNodeNames = map[string]bool{
	"TableScan":         true,
	"ScanFilter":        true,
	"ScanProject":       true,
	"ScanFilterProject": true,
}

// extractScanPushdowns walks the plan tree for the given stage and returns one
// ScanPushdownFact per scan-like plan node it finds. Operator rows are joined by
// planNodeId when available.
//
// connectorByPlanNode is the primary planNodeID → connectorName lookup built
// from QueryInfo.outputStage[].tables. connectorByFQN is the parallel
// "catalog.schema.table" → connectorName fallback, used when the scan node we
// found is a fused ScanFilterProject whose planNodeID doesn't match the entry
// in `tables` (Trino keys those by the original TableScan id, not the fused
// node's id). Only when both miss do we fall back to the catalog name.
func extractScanPushdowns(stageID string, plan any, ops []OperatorSummary, connectorByPlanNode, connectorByFQN map[string]string) []ScanPushdownFact {
	if plan == nil {
		return nil
	}

	m, ok := plan.(map[string]any)
	if !ok {
		return nil
	}

	root, ok := planJSONRoot(m["jsonRepresentation"])
	if !ok {
		return nil
	}

	opsByPlanNode := make(map[string]OperatorSummary, len(ops))
	for _, op := range ops {
		if op.PlanNodeID == "" {
			continue
		}
		// Keep the entry with the largest input — that's the scan operator
		// (vs. a small helper like HashBuilder sharing the same planNodeId).
		if existing, ok := opsByPlanNode[op.PlanNodeID]; ok && existing.InputPositions >= op.InputPositions {
			continue
		}
		opsByPlanNode[op.PlanNodeID] = op
	}

	var out []ScanPushdownFact
	walkScanNodes(root, stageID, opsByPlanNode, connectorByPlanNode, connectorByFQN, &out, 0)
	return out
}

// planJSONRoot normalises the jsonRepresentation value (sometimes a string,
// sometimes already a map) into a map.
func planJSONRoot(jsonRep any) (map[string]any, bool) {
	switch v := jsonRep.(type) {
	case map[string]any:
		return v, true
	case string:
		var root map[string]any
		if err := json.Unmarshal([]byte(v), &root); err != nil {
			return nil, false
		}
		return root, true
	default:
		return nil, false
	}
}

func walkScanNodes(node map[string]any, stageID string, opsByPlanNode map[string]OperatorSummary, connectorByPlanNode, connectorByFQN map[string]string, out *[]ScanPushdownFact, depth int) {
	if depth > 20 || node == nil {
		return
	}

	name, _ := node["name"].(string)
	if scanNodeNames[name] {
		if fact := buildScanPushdownFact(stageID, name, node, opsByPlanNode, connectorByPlanNode, connectorByFQN); fact != nil {
			*out = append(*out, *fact)
		}
	}

	children, _ := node["children"].([]any)
	for _, child := range children {
		if childMap, ok := child.(map[string]any); ok {
			walkScanNodes(childMap, stageID, opsByPlanNode, connectorByPlanNode, connectorByFQN, out, depth+1)
		}
	}
}

func buildScanPushdownFact(stageID, nodeName string, node map[string]any, opsByPlanNode map[string]OperatorSummary, connectorByPlanNode, connectorByFQN map[string]string) *ScanPushdownFact {
	fact := &ScanPushdownFact{
		StageID:  stageID,
		NodeName: nodeName,
	}
	if id, ok := node["id"].(string); ok {
		fact.PlanNodeID = id
	}

	applyDescriptor(fact, node["descriptor"])
	fact.ConnectorType = resolveConnectorType(fact.PlanNodeID, fact.Catalog, fact.Schema, fact.Table, connectorByPlanNode, connectorByFQN)
	applyDetails(fact, node["details"])
	applyOperatorMetrics(fact, opsByPlanNode)

	if fact.Catalog == "" && fact.LocalFilter == "" && len(fact.PushedDetails) == 0 && fact.PhysicalInputPositions == 0 {
		return nil
	}
	return fact
}

func applyDescriptor(fact *ScanPushdownFact, raw any) {
	desc, ok := raw.(map[string]any)
	if !ok {
		return
	}
	if tbl, ok := desc["table"].(string); ok {
		fact.Catalog, fact.Schema, fact.Table = parseTableDescriptor(tbl)
	}
	if fp, ok := desc["filterPredicate"].(string); ok {
		fact.LocalFilter = strings.TrimSpace(fp)
	}
}

// resolveConnectorType picks the connector type for a scan in three steps:
//
//  1. planNodeID lookup against outputStage[].tables[planNodeId].connectorName.
//     This is the cheapest and most precise source.
//  2. catalog.schema.table FQN lookup against the same `tables` map. Required
//     because Trino fuses TableScan + Filter + Project into ScanFilterProject
//     and the fused node's id never appears in `tables` (the entry stays keyed
//     by the underlying TableScan id). Without this fallback every fused scan
//     on a non-stock connector would be misidentified as the catalog name.
//  3. Lowercased catalog name. Last-resort guess for queries from very old
//     Trino versions that don't ship a `tables` map at all.
func resolveConnectorType(planNodeID, catalog, schema, table string, connectorByPlanNode, connectorByFQN map[string]string) string {
	if c, ok := connectorByPlanNode[planNodeID]; ok && c != "" {
		return c
	}
	if fqn := strings.ToLower(formatTableName(catalog, schema, table)); fqn != "" {
		if c, ok := connectorByFQN[fqn]; ok && c != "" {
			return c
		}
	}
	if catalog == "" {
		return ""
	}
	return strings.ToLower(catalog)
}

func applyDetails(fact *ScanPushdownFact, raw any) {
	details := collectDetails(raw)
	if len(details) == 0 {
		return
	}
	fact.PushedConstraintColumns = parseConstraintColumns(details)
	fact.PushedDetails = filterPushedDetails(details)
}

func applyOperatorMetrics(fact *ScanPushdownFact, opsByPlanNode map[string]OperatorSummary) {
	if fact.PlanNodeID == "" {
		return
	}
	op, ok := opsByPlanNode[fact.PlanNodeID]
	if !ok {
		return
	}
	fact.PhysicalInputPositions = op.InputPositions
	fact.OutputRows = op.OutputPositions
	if op.InputPositions > 0 {
		fact.Selectivity = roundDecimals(float64(op.OutputPositions)/float64(op.InputPositions), 6)
	}
}

// parseTableDescriptor splits values like "app_documents:platform.user_credits"
// into catalog="app_documents", schema="platform", table="user_credits".
// Tolerates surrounding whitespace and trailing handle strings ("catalog:schema.table FooHandle{...}").
func parseTableDescriptor(raw string) (catalog, schema, table string) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return
	}

	if sp := strings.IndexAny(s, " \t"); sp >= 0 {
		s = s[:sp]
	}

	colon := strings.IndexByte(s, ':')
	if colon < 0 {
		// No catalog prefix; treat the whole string as schema.table or table.
		schema = schemaAndTable(s, &table)
		return
	}
	catalog = s[:colon]
	rest := s[colon+1:]
	schema = schemaAndTable(rest, &table)
	return
}

func schemaAndTable(s string, table *string) string {
	dot := strings.IndexByte(s, '.')
	if dot < 0 {
		*table = s
		return ""
	}
	*table = s[dot+1:]
	return s[:dot]
}

// collectDetails normalises the "details" field which can be:
//   - []any of strings
//   - a single string
//   - nil
func collectDetails(raw any) []string {
	switch v := raw.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []string{strings.TrimSpace(v)}
	default:
		return nil
	}
}

// parseConstraintColumns looks for "constraint on [c1, c2, ...]" details lines
// and returns the bracketed column list.
func parseConstraintColumns(details []string) []string {
	for _, line := range details {
		if !strings.Contains(line, "constraint on [") {
			continue
		}
		open := strings.Index(line, "[")
		closeIdx := strings.LastIndex(line, "]")
		if open < 0 || closeIdx <= open {
			continue
		}
		raw := line[open+1 : closeIdx]
		var cols []string
		for _, part := range strings.Split(raw, ",") {
			c := strings.TrimSpace(part)
			if c != "" {
				cols = append(cols, c)
			}
		}
		return cols
	}
	return nil
}

// filterPushedDetails drops noise lines so the agent only sees pushed-down info.
func filterPushedDetails(details []string) []string {
	var out []string
	for _, line := range details {
		lower := strings.ToLower(line)
		// Skip estimate / stats output that the connector did not produce.
		if strings.HasPrefix(lower, "estimates:") || strings.HasPrefix(lower, "stats:") {
			continue
		}
		out = append(out, line)
	}
	return out
}

func roundDecimals(v float64, decimals int) float64 {
	if v == 0 {
		return 0
	}
	mult := 1.0
	for i := 0; i < decimals; i++ {
		mult *= 10
	}
	return float64(int64(v*mult+0.5)) / mult
}
