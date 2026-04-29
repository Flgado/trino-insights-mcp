package queryinfo

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

type SessionFacts struct {
	User           string   `json:"user"`
	Source         string   `json:"source,omitempty"`
	Catalog        string   `json:"catalog,omitempty"`
	Schema         string   `json:"schema,omitempty"`
	ClientInfo     string   `json:"clientInfo,omitempty"`
	ClientTags     []string `json:"clientTags,omitempty"`
	ResourceGroups []string `json:"resourceGroups,omitempty"`
}

type TimeFacts struct {
	ElapsedMs        int64 `json:"elapsed_ms,omitempty"`
	QueuedMs         int64 `json:"queued_ms,omitempty"`
	PlanningMs       int64 `json:"planning_ms,omitempty"`
	ExecutionMs      int64 `json:"execution_ms,omitempty"`
	TotalCPUMs       int64 `json:"total_cpu_ms,omitempty"`
	TotalScheduledMs int64 `json:"total_scheduled_ms,omitempty"`
	TotalBlockedMs   int64 `json:"total_blocked_ms,omitempty"`
}

type MemoryFacts struct {
	PeakUserMemoryBytes  int64 `json:"peak_user_memory_bytes,omitempty"`
	PeakTotalMemoryBytes int64 `json:"peak_total_memory_bytes,omitempty"`
	PeakTaskUserMemBytes int64 `json:"peak_task_user_mem_bytes,omitempty"`
	SpilledBytes         int64 `json:"spilled_bytes,omitempty"`
}

type IOFacts struct {
	PhysicalInputBytes     int64 `json:"physical_input_bytes,omitempty"`
	PhysicalInputPositions int   `json:"physical_input_positions,omitempty"`
	ProcessedInputBytes    int64 `json:"processed_input_bytes,omitempty"`
	ProcessedInputPos      int   `json:"processed_input_positions,omitempty"`
	OutputBytes            int64 `json:"output_bytes,omitempty"`
	OutputPositions        int   `json:"output_positions,omitempty"`
}

// OperatorFact is an operator within a stage, ordered by pipeline position.
// Amplification shows the output/input row ratio: >1 means row expansion (e.g. join fan-out),
// <1 means row reduction (e.g. filter, aggregation).
type OperatorFact struct {
	OperatorType  string  `json:"operator_type"`
	CPUMs         int64   `json:"cpu_ms"`
	InputRows     int64   `json:"input_rows"`
	OutputRows    int64   `json:"output_rows"`
	Amplification float64 `json:"amplification,omitempty"`
	PeakMemBytes  int64   `json:"peak_mem_bytes,omitempty"`
	SpilledBytes  int64   `json:"spilled_bytes,omitempty"`
}

type StageFact struct {
	StageID            string         `json:"stage_id"`
	State              string         `json:"state"`
	PlanSummary        string         `json:"plan_summary,omitempty"`
	SubStageIDs        []string       `json:"sub_stage_ids,omitempty"`
	TotalCPUMs         int64          `json:"total_cpu_ms,omitempty"`
	TotalScheduledMs   int64          `json:"total_scheduled_ms,omitempty"`
	TotalBlockedMs     int64          `json:"total_blocked_ms,omitempty"`
	PhysicalInputBytes int64          `json:"physical_input_bytes,omitempty"`
	PhysicalInputPos   int            `json:"physical_input_positions,omitempty"`
	OutputBytes        int64          `json:"output_bytes,omitempty"`
	OutputPositions    int            `json:"output_positions,omitempty"`
	PeakUserMemBytes   int64          `json:"peak_user_mem_bytes,omitempty"`
	SpilledBytes       int64          `json:"spilled_bytes,omitempty"`
	PrimaryOperator    string         `json:"primary_operator,omitempty"`
	Operators          []OperatorFact `json:"operators,omitempty"`
	TaskCount          int            `json:"task_count"`
	MaxTaskCPUMs       int64          `json:"max_task_cpu_ms,omitempty"`
	MinTaskCPUMs       int64          `json:"min_task_cpu_ms,omitempty"`
	P50TaskCPUMs       int64          `json:"p50_task_cpu_ms,omitempty"`
}

type TaskFacts struct {
	TotalTasks     int `json:"total_tasks"`
	CompletedTasks int `json:"completed_tasks"`
	RunningTasks   int `json:"running_tasks"`
	FailedTasks    int `json:"failed_tasks,omitempty"`
	TotalDrivers   int `json:"total_drivers"`
}

// TableFact represents a table accessed by the query, with connector context.
type TableFact struct {
	FullName      string `json:"full_name"`
	Catalog       string `json:"catalog"`
	Schema        string `json:"schema,omitempty"`
	Table         string `json:"table"`
	ConnectorType string `json:"connector_type,omitempty"`
}

// OptimizerRuleFact is a compact view of an optimizer rule that was invoked but
// never applied — a missed optimization opportunity.
type OptimizerRuleFact struct {
	Rule        string `json:"rule"`
	Invocations int    `json:"invocations"`
	Applied     int    `json:"applied"`
}

// DynamicFilterFacts summarizes the effectiveness of dynamic filters.
type DynamicFilterFacts struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
	Lazy      int `json:"lazy,omitempty"`
	Replicated int `json:"replicated,omitempty"`
}

type QueryFacts struct {
	QueryID       string `json:"query_id"`
	State         string `json:"state"`
	ErrorType     string `json:"error_type,omitempty"`
	ErrorCodeName string `json:"error_code_name,omitempty"`

	Tables         []TableFact          `json:"tables,omitempty"`
	OptimizerRules []OptimizerRuleFact  `json:"optimizer_rules,omitempty"`
	DynamicFilters *DynamicFilterFacts   `json:"dynamic_filters,omitempty"`
	Session        SessionFacts         `json:"session"`
	Time           TimeFacts            `json:"time"`
	Memory         MemoryFacts          `json:"memory"`
	IO             IOFacts              `json:"io"`
	Tasks          TaskFacts            `json:"tasks"`
	Stages         []StageFact          `json:"stages,omitempty"`
}

// Project converts a full QueryInfo into an agent-friendly QueryFacts.
func Project(qi *QueryInfo) *QueryFacts {
	if qi == nil {
		return nil
	}

	qs := qi.QueryStats

	facts := &QueryFacts{
		QueryID:        qi.QueryID,
		State:          qi.State,
		ErrorType:      qi.ErrorType,
		ErrorCodeName:  errorCodeName(qi.ErrorCode),
		Tables:         extractTableFacts(qi),
		OptimizerRules: projectOptimizerRules(qs.OptimizerRulesSummaries),
		DynamicFilters: projectDynamicFilters(qs.DynamicFiltersStats),

		Session: projectSession(qi),
		Time: TimeFacts{
			ElapsedMs:        ParseDurationMs(qs.ElapsedTime),
			QueuedMs:         ParseDurationMs(qs.QueuedTime),
			PlanningMs:       ParseDurationMs(qs.PlanningTime),
			ExecutionMs:      ParseDurationMs(qs.ExecutionTime),
			TotalCPUMs:       ParseDurationMs(qs.TotalCpuTime),
			TotalScheduledMs: ParseDurationMs(qs.TotalScheduledTime),
			TotalBlockedMs:   ParseDurationMs(qs.TotalBlockedTime),
		},
		Memory: MemoryFacts{
			PeakUserMemoryBytes:  ParseSizeBytes(qs.PeakUserMemoryReservation),
			PeakTotalMemoryBytes: ParseSizeBytes(qs.PeakTotalMemoryReservation),
			PeakTaskUserMemBytes: ParseSizeBytes(qs.PeakTaskUserMemory),
			SpilledBytes:         ParseSizeBytes(qs.SpilledDataSize),
		},
		IO: IOFacts{
			PhysicalInputBytes:     ParseSizeBytes(qs.PhysicalInputDataSize),
			PhysicalInputPositions: qs.PhysicalInputPositions,
			ProcessedInputBytes:    ParseSizeBytes(qs.ProcessedInputDataSize),
			ProcessedInputPos:      qs.ProcessedInputPositions,
			OutputBytes:            ParseSizeBytes(qs.OutputDataSize),
			OutputPositions:        qs.OutputPositions,
		},
		Tasks: TaskFacts{
			TotalTasks:     qs.TotalTasks,
			CompletedTasks: qs.CompletedTasks,
			RunningTasks:   qs.RunningTasks,
			FailedTasks:    qs.FailedTasks,
			TotalDrivers:   qs.TotalDrivers,
		},
	}

	opsByStage := groupOperatorsByStage(qs.OperatorSummaries)

	if qi.Stages != nil {
		for _, si := range qi.Stages.Stages {
			ss := si.StageStats
			sf := StageFact{
				StageID:            si.StageID,
				State:              si.State,
				SubStageIDs:        si.SubStages,
				PlanSummary:        extractPlanSummary(si.Plan),
				TotalCPUMs:         ParseDurationMs(ss.TotalCpuTime),
				TotalScheduledMs:   ParseDurationMs(ss.TotalScheduledTime),
				TotalBlockedMs:     ParseDurationMs(ss.TotalBlockedTime),
				PhysicalInputBytes: ParseSizeBytes(ss.PhysicalInputDataSize),
				PhysicalInputPos:   ss.PhysicalInputPositions,
				OutputBytes:        ParseSizeBytes(ss.OutputDataSize),
				OutputPositions:    ss.OutputPositions,
				PeakUserMemBytes:   ParseSizeBytes(ss.PeakUserMemoryReservation),
				SpilledBytes:       ParseSizeBytes(ss.SpilledDataSize),
			}

			stageNum := extractStageNum(si.StageID)
			sf.Operators, sf.PrimaryOperator = projectOperators(opsByStage[stageNum])
			sf.TaskCount, sf.MaxTaskCPUMs, sf.MinTaskCPUMs, sf.P50TaskCPUMs = projectTaskStats(si.Tasks)

			facts.Stages = append(facts.Stages, sf)
		}
	}

	return facts
}

// infrastructureOps are operators that exist for data plumbing, not user logic.
var infrastructureOps = map[string]bool{
	"TaskOutputOperator":        true,
	"LocalExchangeSinkOperator": true,
	"ExchangeOperator":          true,
	"OutputSpoolingOperator":    true,
	"LocalMergeSourceOperator":  true,
}

func groupOperatorsByStage(ops []OperatorSummary) map[int][]OperatorSummary {
	m := make(map[int][]OperatorSummary)
	for _, op := range ops {
		m[op.StageID] = append(m[op.StageID], op)
	}
	return m
}

func projectOperators(ops []OperatorSummary) ([]OperatorFact, string) {
	if len(ops) == 0 {
		return nil, ""
	}

	// Sort by pipeline position (pipelineId, operatorId) to preserve data flow order
	sorted := make([]OperatorSummary, len(ops))
	copy(sorted, ops)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].PipelineID != sorted[j].PipelineID {
			return sorted[i].PipelineID < sorted[j].PipelineID
		}
		return sorted[i].OperatorID < sorted[j].OperatorID
	})

	var facts []OperatorFact
	for _, op := range sorted {
		if infrastructureOps[op.OperatorType] {
			continue
		}
		cpuMs := ParseDurationMs(op.AddInputCpu) + ParseDurationMs(op.GetOutputCpu)
		amp := computeAmplification(op.InputPositions, op.OutputPositions)
		facts = append(facts, OperatorFact{
			OperatorType:  op.OperatorType,
			CPUMs:         cpuMs,
			InputRows:     op.InputPositions,
			OutputRows:    op.OutputPositions,
			Amplification: amp,
			PeakMemBytes:  ParseSizeBytes(op.PeakUserMemoryReservation),
			SpilledBytes:  ParseSizeBytes(op.SpilledDataSize),
		})
	}

	primary := ""
	var maxCPU int64
	for _, f := range facts {
		if f.CPUMs > maxCPU {
			maxCPU = f.CPUMs
			primary = f.OperatorType
		}
	}

	if primary == "" && len(ops) > 0 {
		primary = ops[0].OperatorType
	}

	return facts, primary
}

func computeAmplification(in, out int64) float64 {
	if in <= 0 {
		return 0
	}
	amp := float64(out) / float64(in)
	return math.Round(amp*100) / 100
}

func projectTaskStats(tasks []TaskInfo) (count int, maxCPU, minCPU, p50CPU int64) {
	count = len(tasks)
	if count == 0 {
		return
	}

	cpus := make([]int64, 0, count)
	for _, t := range tasks {
		cpus = append(cpus, ParseDurationMs(t.Stats.TotalCpuTime))
	}
	sort.Slice(cpus, func(i, j int) bool { return cpus[i] < cpus[j] })

	minCPU = cpus[0]
	maxCPU = cpus[count-1]
	p50CPU = cpus[count/2]
	return
}

// extractTableFacts builds a deduplicated list of TableFact from
// QueryInfo.inputs[] and QueryInfo.referencedTables[].
// Connector type is inferred from the catalog name (hive -> hive, iceberg -> iceberg, etc.)
// or from connectorInfo when available.
func extractTableFacts(qi *QueryInfo) []TableFact {
	seen := make(map[string]bool)
	var tables []TableFact

	for _, inp := range qi.Inputs {
		fqn := formatTableName(inp.CatalogName, inp.Schema, inp.Table)
		if fqn == "" || seen[fqn] {
			continue
		}
		seen[fqn] = true
		tables = append(tables, TableFact{
			FullName:      fqn,
			Catalog:       inp.CatalogName,
			Schema:        inp.Schema,
			Table:         inp.Table,
			ConnectorType: inferConnectorType(inp.CatalogName, inp.ConnectorInfo),
		})
	}

	for _, ref := range qi.ReferencedTables {
		fqn := formatTableName(ref.CatalogName, ref.SchemaName, ref.TableName)
		if fqn == "" || seen[fqn] {
			continue
		}
		seen[fqn] = true
		tables = append(tables, TableFact{
			FullName:      fqn,
			Catalog:       ref.CatalogName,
			Schema:        ref.SchemaName,
			Table:         ref.TableName,
			ConnectorType: inferConnectorType(ref.CatalogName, nil),
		})
	}

	return tables
}

// inferConnectorType derives the connector type from the catalog name or connectorInfo.
// Common convention: catalog names like "hive", "iceberg", "postgresql", "mysql", "memory".
func inferConnectorType(catalog string, connectorInfo any) string {
	if m, ok := connectorInfo.(map[string]any); ok {
		if ct, ok := m["connectorName"].(string); ok && ct != "" {
			return ct
		}
	}
	if catalog == "" {
		return ""
	}
	return strings.ToLower(catalog)
}

func formatTableName(catalog, schema, table string) string {
	if table == "" {
		return ""
	}
	parts := make([]string, 0, 3)
	if catalog != "" {
		parts = append(parts, catalog)
	}
	if schema != "" {
		parts = append(parts, schema)
	}
	parts = append(parts, table)
	return strings.Join(parts, ".")
}

// projectOptimizerRules converts the raw optimizer rule summaries into a compact
// list of facts. All rules are included so the agent can see both applied and missed.
func projectOptimizerRules(rules []OptimizerRuleSummary) []OptimizerRuleFact {
	if len(rules) == 0 {
		return nil
	}
	var facts []OptimizerRuleFact
	for _, r := range rules {
		if r.Invocations == 0 {
			continue
		}
		facts = append(facts, OptimizerRuleFact{
			Rule:        r.Rule,
			Invocations: r.Invocations,
			Applied:     r.Applied,
		})
	}
	return facts
}

// projectDynamicFilters summarises dynamic filter stats. Returns nil when no filters exist.
func projectDynamicFilters(dfs DynamicFiltersStats) *DynamicFilterFacts {
	if dfs.TotalDynamicFilters == 0 {
		return nil
	}
	return &DynamicFilterFacts{
		Total:      dfs.TotalDynamicFilters,
		Completed:  dfs.DynamicFiltersCompleted,
		Lazy:       dfs.LazyDynamicFilters,
		Replicated: dfs.ReplicatedDynamicFilters,
	}
}

// extractPlanSummary builds a compact one-line plan description from the stage's plan.
// Trino's plan object can have a jsonRepresentation with a node tree. We walk it
// to produce e.g. "Output -> OrderBy -> ScanFilterProject[lineitem]".
func extractPlanSummary(plan any) string {
	if plan == nil {
		return ""
	}

	m, ok := plan.(map[string]any)
	if !ok {
		return ""
	}

	jsonRep, ok := m["jsonRepresentation"]
	if !ok {
		return ""
	}

	var root map[string]any
	switch v := jsonRep.(type) {
	case map[string]any:
		root = v
	case string:
		if err := json.Unmarshal([]byte(v), &root); err != nil {
			return ""
		}
	default:
		return ""
	}

	var parts []string
	walkPlanNode(root, &parts, 0)
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " -> ")
}

// walkPlanNode recursively extracts operator names from the plan tree.
// Limits depth to keep the summary compact.
func walkPlanNode(node map[string]any, parts *[]string, depth int) {
	if depth > 10 || node == nil {
		return
	}

	name, _ := node["name"].(string)
	if name == "" {
		return
	}

	// Enrich with table name if present in the descriptor
	desc, _ := node["descriptor"].(map[string]any)
	tableName := ""
	if desc != nil {
		if tbl, ok := desc["table"].(string); ok {
			tableName = tbl
		}
	}

	label := name
	if tableName != "" {
		label = fmt.Sprintf("%s[%s]", name, tableName)
	}
	*parts = append(*parts, label)

	children, _ := node["children"].([]any)
	for _, child := range children {
		if childMap, ok := child.(map[string]any); ok {
			walkPlanNode(childMap, parts, depth+1)
		}
	}
}

// extractStageNum parses the trailing stage number from a full stage ID like
// "20260425_140250_00084_83n6z.1" -> 1
func extractStageNum(stageID string) int {
	idx := strings.LastIndexByte(stageID, '.')
	if idx < 0 {
		return -1
	}
	n := 0
	for _, c := range stageID[idx+1:] {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// StageShortID returns just the stage number suffix for display, e.g. "1" from "...84_83n6z.1".
func StageShortID(stageID string) string {
	idx := strings.LastIndexByte(stageID, '.')
	if idx >= 0 {
		return stageID[idx+1:]
	}
	return stageID
}

// FormatStageLabel returns "stage N (OperatorType)" or just "stage N" if no operator.
func FormatStageLabel(sf StageFact) string {
	short := StageShortID(sf.StageID)
	if sf.PrimaryOperator != "" {
		return fmt.Sprintf("stage %s (%s)", short, sf.PrimaryOperator)
	}
	return fmt.Sprintf("stage %s", short)
}

func projectSession(qi *QueryInfo) SessionFacts {
	sf := SessionFacts{
		User:   qi.SessionUser,
		Source: qi.SessionSource,
	}
	if qi.Session != nil {
		sf.User = qi.Session.User
		sf.Source = qi.Session.Source
		sf.ClientTags = qi.Session.ClientTags
	}
	if sf.User == "" && qi.SessionUser != "" {
		sf.User = qi.SessionUser
	}
	if qi.ResourceGroupID != nil {
		sf.ResourceGroups = qi.ResourceGroupID
	}
	return sf
}

func errorCodeName(ec *ErrorCode) string {
	if ec == nil {
		return ""
	}
	return ec.Name
}
