package queryinfo

// ErrorCode is the structured error from Trino (present when a query fails).
type ErrorCode struct {
	Code  int    `json:"code"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	Fatal bool   `json:"fatal"`
}

// CoordinatorVersion is the nested "version" object (e.g. { "version": "480" }).
type CoordinatorVersion struct {
	Version string `json:"version"`
}

// CatalogSchemaPath is one path element in session.path.path.
type CatalogSchemaPath struct {
	CatalogName string `json:"catalogName"`
	SchemaName  string `json:"schemaName"`
}

// SessionPath is session.path (search path, catalog.schema pairs + raw string).
type SessionPath struct {
	Path    []CatalogSchemaPath `json:"path"`
	RawPath string              `json:"rawPath"`
}

// QuerySession is the full "session" object on GET /v1/query/{id} and /ui/api/query/{id}.
type QuerySession struct {
	QueryID                  string         `json:"queryId"`
	QuerySpan                map[string]any `json:"querySpan"`
	TransactionID            string         `json:"transactionId"`
	ClientTransactionSupport bool           `json:"clientTransactionSupport"`
	User                     string         `json:"user"`
	OriginalUser             string         `json:"originalUser"`
	SetOriginalRoles         []any          `json:"setOriginalRoles"`
	Groups                   []any          `json:"groups"`
	OriginalUserGroups       []any          `json:"originalUserGroups"`
	Principal                string         `json:"principal"`
	EnabledRoles             []any          `json:"enabledRoles"`
	Source                   string         `json:"source"`
	Path                     SessionPath    `json:"path"`
	TimeZoneKey              int            `json:"timeZoneKey"`
	Locale                   string         `json:"locale"`
	RemoteUserAddress        string         `json:"remoteUserAddress"`
	UserAgent                string         `json:"userAgent"`
	ClientTags               []string       `json:"clientTags"`
	ClientCapabilities       []string       `json:"clientCapabilities"`
	ResourceEstimates        map[string]any `json:"resourceEstimates"`
	Start                    string         `json:"start"`
	SystemProperties         map[string]any `json:"systemProperties"`
	CatalogProperties        map[string]any `json:"catalogProperties"`
	CatalogRoles             map[string]any `json:"catalogRoles"`
	PreparedStatements       map[string]any `json:"preparedStatements"`
	ProtocolName             string         `json:"protocolName"`
	QueryDataEncoding        string         `json:"queryDataEncoding,omitempty"`
	TimeZone                 string         `json:"timeZone"`
}

// DynamicFiltersStats is embedded in queryStats.dynamicFiltersStats.
type DynamicFiltersStats struct {
	DynamicFilterDomainStats []any `json:"dynamicFilterDomainStats"`
	LazyDynamicFilters       int   `json:"lazyDynamicFilters"`
	ReplicatedDynamicFilters int   `json:"replicatedDynamicFilters"`
	TotalDynamicFilters      int   `json:"totalDynamicFilters"`
	DynamicFiltersCompleted  int   `json:"dynamicFiltersCompleted"`
}

// StagesWrapper is the top-level "stages" object: a flat list plus the output stage ID.
type StagesWrapper struct {
	OutputStageID string      `json:"outputStageId"`
	Stages        []StageInfo `json:"stages"`
}

// QueryInfo is the query object returned by the coordinator, including:
//   - GET /v1/query/{queryId}
//   - GET /ui/api/query/{id}
type QueryInfo struct {
	QueryID         string        `json:"queryId"`
	SessionUser     string        `json:"sessionUser,omitempty"`
	SessionPrincipal string       `json:"sessionPrincipal,omitempty"`
	SessionSource   string        `json:"sessionSource,omitempty"`
	ResourceGroupID []string      `json:"resourceGroupId,omitempty"`
	State           string        `json:"state"`
	Scheduled       bool          `json:"scheduled"`
	Self            string        `json:"self,omitempty"`
	FieldNames      []string      `json:"fieldNames,omitempty"`
	QueryTextPreview string       `json:"queryTextPreview,omitempty"`
	Query           string        `json:"query,omitempty"`
	PreparedQuery   string        `json:"preparedQuery,omitempty"`
	UpdateType      string        `json:"updateType,omitempty"`
	QueryStats      QueryStats    `json:"queryStats"`
	Session         *QuerySession `json:"session,omitempty"`
	Stages          *StagesWrapper `json:"stages,omitempty"`

	ResetAuthorizationUser        bool                `json:"resetAuthorizationUser,omitempty"`
	SetOriginalRoles              []any               `json:"setOriginalRoles,omitempty"`
	SetSessionProperties          map[string]any      `json:"setSessionProperties,omitempty"`
	ResetSessionProperties        []any               `json:"resetSessionProperties,omitempty"`
	SetRoles                      map[string]any      `json:"setRoles,omitempty"`
	AddedPreparedStatements       map[string]any      `json:"addedPreparedStatements,omitempty"`
	DeallocatedPreparedStatements []string            `json:"deallocatedPreparedStatements,omitempty"`
	ClearTransactionID            bool                `json:"clearTransactionId,omitempty"`
	Warnings                      []any               `json:"warnings,omitempty"`
	Inputs                        []InputRef          `json:"inputs,omitempty"`
	ReferencedTables              []TableRef          `json:"referencedTables,omitempty"`
	Routines                      []any               `json:"routines,omitempty"`
	FinalQueryInfo                bool                `json:"finalQueryInfo,omitempty"`
	Pruned                        bool                `json:"pruned,omitempty"`
	Version                       *CoordinatorVersion `json:"version,omitempty"`

	ErrorType   string     `json:"errorType,omitempty"`
	ErrorCode   *ErrorCode `json:"errorCode,omitempty"`
	QueryType   string     `json:"queryType,omitempty"`
	RetryPolicy string     `json:"retryPolicy,omitempty"`
	ClientTags  []string   `json:"clientTags,omitempty"`

	ProgressPercentage *float64 `json:"progressPercentage,omitempty"`
	RunningPercentage  *float64 `json:"runningPercentage,omitempty"`
}

// QueryStats is embedded in QueryInfo. Durations and sizes are string tokens (e.g. "0B", "4.96s").
type QueryStats struct {
	CreateTime         string `json:"createTime,omitempty"`
	ExecutionStartTime string `json:"executionStartTime,omitempty"`
	LastHeartbeat      string `json:"lastHeartbeat,omitempty"`
	EndTime            string `json:"endTime,omitempty"`

	QueuedTime          string `json:"queuedTime,omitempty"`
	ResourceWaitingTime string `json:"resourceWaitingTime,omitempty"`
	DispatchingTime     string `json:"dispatchingTime,omitempty"`
	ElapsedTime         string `json:"elapsedTime,omitempty"`
	ExecutionTime       string `json:"executionTime,omitempty"`
	AnalysisTime        string `json:"analysisTime,omitempty"`
	PlanningTime        string `json:"planningTime,omitempty"`
	PlanningCPUTime     string `json:"planningCpuTime,omitempty"`
	StartingTime        string `json:"startingTime,omitempty"`
	FinishingTime       string `json:"finishingTime,omitempty"`

	TotalTasks     int `json:"totalTasks,omitempty"`
	RunningTasks   int `json:"runningTasks,omitempty"`
	CompletedTasks int `json:"completedTasks,omitempty"`
	FailedTasks    int `json:"failedTasks,omitempty"`

	TotalDrivers     int `json:"totalDrivers,omitempty"`
	QueuedDrivers    int `json:"queuedDrivers,omitempty"`
	RunningDrivers   int `json:"runningDrivers,omitempty"`
	CompletedDrivers int `json:"completedDrivers,omitempty"`
	BlockedDrivers   int `json:"blockedDrivers,omitempty"`

	CumulativeUserMemory       float64 `json:"cumulativeUserMemory,omitempty"`
	FailedCumulativeUserMemory float64 `json:"failedCumulativeUserMemory,omitempty"`

	UserMemoryReservation          string `json:"userMemoryReservation,omitempty"`
	RevocableMemoryReservation     string `json:"revocableMemoryReservation,omitempty"`
	TotalMemoryReservation         string `json:"totalMemoryReservation,omitempty"`
	PeakUserMemoryReservation      string `json:"peakUserMemoryReservation,omitempty"`
	PeakRevocableMemoryReservation string `json:"peakRevocableMemoryReservation,omitempty"`
	PeakTotalMemoryReservation     string `json:"peakTotalMemoryReservation,omitempty"`
	PeakTaskUserMemory             string `json:"peakTaskUserMemory,omitempty"`
	PeakTaskRevocableMemory        string `json:"peakTaskRevocableMemory,omitempty"`
	PeakTaskTotalMemory            string `json:"peakTaskTotalMemory,omitempty"`
	SpilledDataSize                string `json:"spilledDataSize,omitempty"`

	Scheduled *bool `json:"scheduled,omitempty"`

	TotalScheduledTime  string `json:"totalScheduledTime,omitempty"`
	FailedScheduledTime string `json:"failedScheduledTime,omitempty"`
	TotalCpuTime        string `json:"totalCpuTime,omitempty"`
	FailedCpuTime       string `json:"failedCpuTime,omitempty"`
	TotalBlockedTime    string `json:"totalBlockedTime,omitempty"`

	FullyBlocked   bool     `json:"fullyBlocked,omitempty"`
	BlockedReasons []string `json:"blockedReasons,omitempty"`

	PhysicalInputDataSize        string `json:"physicalInputDataSize,omitempty"`
	FailedPhysicalInputDataSize  string `json:"failedPhysicalInputDataSize,omitempty"`
	PhysicalInputReadTime        string `json:"physicalInputReadTime,omitempty"`
	FailedPhysicalInputReadTime  string `json:"failedPhysicalInputReadTime,omitempty"`
	PhysicalInputPositions       int    `json:"physicalInputPositions,omitempty"`
	FailedPhysicalInputPositions int    `json:"failedPhysicalInputPositions,omitempty"`

	InternalNetworkInputDataSize        string `json:"internalNetworkInputDataSize,omitempty"`
	FailedInternalNetworkInputDataSize  string `json:"failedInternalNetworkInputDataSize,omitempty"`
	InternalNetworkInputPositions       int    `json:"internalNetworkInputPositions,omitempty"`
	FailedInternalNetworkInputPositions int    `json:"failedInternalNetworkInputPositions,omitempty"`

	ProcessedInputDataSize        string `json:"processedInputDataSize,omitempty"`
	FailedProcessedInputDataSize  string `json:"failedProcessedInputDataSize,omitempty"`
	ProcessedInputPositions       int    `json:"processedInputPositions,omitempty"`
	FailedProcessedInputPositions int    `json:"failedProcessedInputPositions,omitempty"`

	InputBlockedTime       string `json:"inputBlockedTime,omitempty"`
	FailedInputBlockedTime string `json:"failedInputBlockedTime,omitempty"`

	OutputDataSize          string `json:"outputDataSize,omitempty"`
	FailedOutputDataSize    string `json:"failedOutputDataSize,omitempty"`
	OutputPositions         int    `json:"outputPositions,omitempty"`
	FailedOutputPositions   int    `json:"failedOutputPositions,omitempty"`
	OutputBlockedTime       string `json:"outputBlockedTime,omitempty"`
	FailedOutputBlockedTime string `json:"failedOutputBlockedTime,omitempty"`

	PhysicalWrittenDataSize       string `json:"physicalWrittenDataSize,omitempty"`
	FailedPhysicalWrittenDataSize string `json:"failedPhysicalWrittenDataSize,omitempty"`
	LogicalWrittenDataSize        string `json:"logicalWrittenDataSize,omitempty"`
	WrittenPositions              int    `json:"writtenPositions,omitempty"`

	StageGCStatistics       []any               `json:"stageGcStatistics,omitempty"`
	DynamicFiltersStats     DynamicFiltersStats  `json:"dynamicFiltersStats"`
	CatalogMetadataMetrics  map[string]any       `json:"catalogMetadataMetrics,omitempty"`
	ExchangeMetrics         map[string]any       `json:"exchangeMetrics,omitempty"`
	OperatorSummaries       []OperatorSummary    `json:"operatorSummaries,omitempty"`
	OptimizerRulesSummaries []OptimizerRuleSummary `json:"optimizerRulesSummaries,omitempty"`

	ProgressPercentage *float64 `json:"progressPercentage,omitempty"`
	RunningPercentage  *float64 `json:"runningPercentage,omitempty"`
}

// StageInfo is a single stage in the flat stages list.
type StageInfo struct {
	StageID         string     `json:"stageId"`
	State           string     `json:"state"`
	Plan            any        `json:"plan,omitempty"`
	CoordinatorOnly bool       `json:"coordinatorOnly,omitempty"`
	Types           []string   `json:"types,omitempty"`
	StageStats      StageStats `json:"stageStats"`
	Tasks           []TaskInfo `json:"tasks,omitempty"`
	SubStages       []string   `json:"subStages,omitempty"`
	Tables          any        `json:"tables,omitempty"`
}

// StageStats holds per-stage metrics. Field names match the real Trino JSON exactly.
type StageStats struct {
	TotalCpuTime       string `json:"totalCpuTime,omitempty"`
	FailedCpuTime      string `json:"failedCpuTime,omitempty"`
	TotalScheduledTime string `json:"totalScheduledTime,omitempty"`
	FailedScheduledTime string `json:"failedScheduledTime,omitempty"`
	TotalBlockedTime   string `json:"totalBlockedTime,omitempty"`

	TotalTasks     int `json:"totalTasks,omitempty"`
	RunningTasks   int `json:"runningTasks,omitempty"`
	CompletedTasks int `json:"completedTasks,omitempty"`
	FailedTasks    int `json:"failedTasks,omitempty"`
	TotalDrivers   int `json:"totalDrivers,omitempty"`
	QueuedDrivers  int `json:"queuedDrivers,omitempty"`
	RunningDrivers int `json:"runningDrivers,omitempty"`
	CompletedDrivers int `json:"completedDrivers,omitempty"`
	BlockedDrivers int `json:"blockedDrivers,omitempty"`

	CumulativeUserMemory       float64 `json:"cumulativeUserMemory,omitempty"`
	FailedCumulativeUserMemory float64 `json:"failedCumulativeUserMemory,omitempty"`

	UserMemoryReservation     string `json:"userMemoryReservation,omitempty"`
	RevocableMemoryReservation string `json:"revocableMemoryReservation,omitempty"`
	TotalMemoryReservation    string `json:"totalMemoryReservation,omitempty"`
	PeakUserMemoryReservation string `json:"peakUserMemoryReservation,omitempty"`
	PeakRevocableMemoryReservation string `json:"peakRevocableMemoryReservation,omitempty"`
	SpilledDataSize           string `json:"spilledDataSize,omitempty"`

	PhysicalInputDataSize    string `json:"physicalInputDataSize,omitempty"`
	PhysicalInputPositions   int    `json:"physicalInputPositions,omitempty"`
	PhysicalInputReadTime    string `json:"physicalInputReadTime,omitempty"`
	PhysicalWrittenDataSize  string `json:"physicalWrittenDataSize,omitempty"`

	InternalNetworkInputDataSize  string `json:"internalNetworkInputDataSize,omitempty"`
	InternalNetworkInputPositions int    `json:"internalNetworkInputPositions,omitempty"`

	ProcessedInputDataSize  string `json:"processedInputDataSize,omitempty"`
	ProcessedInputPositions int    `json:"processedInputPositions,omitempty"`

	InputBlockedTime  string `json:"inputBlockedTime,omitempty"`
	OutputBlockedTime string `json:"outputBlockedTime,omitempty"`

	OutputDataSize  string `json:"outputDataSize,omitempty"`
	OutputPositions int    `json:"outputPositions,omitempty"`

	FullyBlocked   bool     `json:"fullyBlocked,omitempty"`
	BlockedReasons []string `json:"blockedReasons,omitempty"`

	OperatorSummaries []OperatorSummary `json:"operatorSummaries,omitempty"`

	SchedulingComplete string `json:"schedulingComplete,omitempty"`
}

// OperatorSummary is one operator's aggregated stats across all drivers in a pipeline.
type OperatorSummary struct {
	StageID                    int    `json:"stageId"`
	PipelineID                 int    `json:"pipelineId"`
	OperatorID                 int    `json:"operatorId"`
	PlanNodeID                 string `json:"planNodeId"`
	OperatorType               string `json:"operatorType"`
	TotalDrivers               int    `json:"totalDrivers"`
	AddInputCpu                string `json:"addInputCpu"`
	GetOutputCpu               string `json:"getOutputCpu"`
	InputPositions             int64  `json:"inputPositions"`
	OutputPositions            int64  `json:"outputPositions"`
	PeakUserMemoryReservation  string `json:"peakUserMemoryReservation"`
	SpilledDataSize            string `json:"spilledDataSize"`
	BlockedWall                string `json:"blockedWall"`
}

// TaskInfo is a single task within a stage.
type TaskInfo struct {
	TaskStatus any       `json:"taskStatus,omitempty"`
	Stats      TaskStats `json:"stats"`
}

// TaskStats holds per-task metrics within a stage.
type TaskStats struct {
	TotalCpuTime              string `json:"totalCpuTime,omitempty"`
	TotalScheduledTime        string `json:"totalScheduledTime,omitempty"`
	TotalBlockedTime          string `json:"totalBlockedTime,omitempty"`
	OutputPositions           int64  `json:"outputPositions,omitempty"`
	PeakUserMemoryReservation string `json:"peakUserMemoryReservation,omitempty"`
	SpilledDataSize           string `json:"spilledDataSize,omitempty"`
	TotalDrivers              int    `json:"totalDrivers,omitempty"`
}

// InputRef represents a table/connector input in QueryInfo.inputs[].
type InputRef struct {
	CatalogName    string   `json:"catalogName,omitempty"`
	Schema         string   `json:"schema,omitempty"`
	Table          string   `json:"table,omitempty"`
	ConnectorInfo  any      `json:"connectorInfo,omitempty"`
	Columns        []any    `json:"columns,omitempty"`
	ConnectorMetrics map[string]any `json:"connectorMetrics,omitempty"`
}

// TableRef represents a table reference in QueryInfo.referencedTables[].
type TableRef struct {
	CatalogName   string `json:"catalogName,omitempty"`
	SchemaName    string `json:"schemaName,omitempty"`
	TableName     string `json:"tableName,omitempty"`
	Authorization string `json:"authorization,omitempty"`
}

// OptimizerRuleSummary is one optimizer rule's application summary.
// TotalTime is in nanoseconds.
type OptimizerRuleSummary struct {
	Rule        string `json:"rule"`
	Invocations int    `json:"invocations"`
	Applied     int    `json:"applied"`
	TotalTime   int64  `json:"totalTime,omitempty"`
	Failures    int    `json:"failures,omitempty"`
}
