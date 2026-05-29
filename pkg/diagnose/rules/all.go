package rules

import "github.com/Flgado/trino-insights-mcp/pkg/diagnose"

// DefaultEngine returns an Engine with all built-in rules using default thresholds.
func DefaultEngine() *diagnose.Engine {
	return diagnose.NewEngine(
		Failed{},
		CPUBound{},
		MemoryPressure{},
		DiskSpill{},
		QueuedTooLong{},
		StageSkew{},
		HotspotStage{},
		ScanTooLarge{},
		PoorSelectivity{},
		UnderParallelised{},
		LongBlocked{},
		RowExplosion{},
		MissedPushdown{},
		LocalFilterDominates{},
		DuplicateFederatedScans{},
		DivergentScanRowcounts{},
		IcebergMetadataTable{},
		UnpushableExpression{},
		SlowEmptyScan{},
	)
}
