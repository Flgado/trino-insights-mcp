# Diagnostic rules reference

Every rule produces a `Finding` with:

- `rule_id` — stable identifier (e.g. `trino.long-blocked`)
- `severity` — one of `critical`, `warn`, `info`
- `title` — short human-readable headline
- `details` — plain-English explanation with the specific numbers that triggered it
- `evidence` — typed payload the agent can quote verbatim
- `remediation` — concrete next step

All thresholds are tunable via the public struct fields shown below.
Override them by constructing the rule explicitly when building a custom
engine, e.g. `LongBlocked{MinBlockedMs: 5000}`.

---

## `trino.failed` · critical

Query terminated in `FAILED` state.

| Field | Default | What it does |
|---|---|---|
| _(none)_ | — | Always fires when `state == FAILED` |

**Evidence:** `error_type`, `error_code_name`, `error_message`.

**Remediation:** depends on the error code; the rule maps common
Trino error codes to actionable guidance.

---

## `trino.memory-pressure` · warn

Peak **per-task** user memory is near the node-level limit.

| Field | Default | What it does |
|---|---|---|
| `Threshold` | 1 GiB | Floor at which the rule starts firing |

**Why per-task and not per-query?** Trino's memory limits are enforced
per task. A query that totals 4 GiB across 8 tasks is fine; a single
task spiking to 4 GiB is a hot key or a join skew waiting to OOM.

---

## `trino.disk-spill` · warn

The query spilled aggregation/sort state to disk. Identifies the
specific operator that spilled.

| Field | Default | What it does |
|---|---|---|
| `MinSpilledBytes` | 100 MiB | Floor; tiny spills are noise |

---

## `trino.cpu-bound` · warn

`total_cpu_time / total_scheduled_time` is unusually high — the query
is spending most of its scheduled time burning CPU rather than waiting.

| Field | Default | What it does |
|---|---|---|
| `Threshold` | 0.85 | Fires when ratio ≥ this |

**Caveat:** CPU-bound is almost always a *symptom*, not a cause.
The real driver is usually skew, a hot key, or an expensive expression
on a large input. The agent is instructed to read the SQL before
giving the user a fix.

---

## `trino.stage-skew` · warn

Within a single stage, the slowest task spent much more CPU than the
median task — a sign of partition skew, a hot key, or a non-uniform
join distribution.

| Field | Default | What it does |
|---|---|---|
| `TaskSkewRatio` | 5.0 | `max_task_cpu / p50_task_cpu` ratio that triggers per-task skew |
| `StageSkewRatio` | 10.0 | Fallback ratio comparing one stage's CPU to the rest, used when per-task stats are unavailable |

The per-task path is preferred. The stage-vs-stage fallback only fires
when at least 9 other stages exist for comparison.

---

## `trino.hotspot-stage` · warn

One stage carries ≥ 60% of total query CPU. Names the operator type.

| Field | Default | What it does |
|---|---|---|
| `Threshold` | 0.60 | CPU-share that defines a hotspot |

---

## `trino.row-explosion` · warn

An operator emits ≥ 10× more rows than it consumes. Usually a join
fan-out from a missing predicate, an unintended cross join, or an
`UNNEST` over too-large arrays.

| Field | Default | What it does |
|---|---|---|
| `MinInputRows` | 10,000 | Ignore tiny operators where the ratio is meaningless |
| `Amplification` | 10.0 | Output/input row ratio that triggers the rule |

---

## `trino.long-blocked` · warn

The query spent a large share of its scheduled time blocked (waiting on
I/O, memory, or downstream consumers) AND the absolute blocked time is
non-trivial.

| Field | Default | What it does |
|---|---|---|
| `Threshold` | 0.40 | `blocked / scheduled` ratio floor |
| `MinBlockedMs` | 2,000 | Absolute floor; sub-second waits never fire |

**Why both thresholds?** Ratio alone is noisy on sub-second queries —
a 600 ms blocked / 1,000 ms scheduled (60%) shouldn't flag. The
absolute floor protects against false positives on healthy short queries.

---

## `trino.scan-too-large` · warn

A table scan exceeds the row or byte budget.

| Field | Default | What it does |
|---|---|---|
| `MaxRows` | 1,000,000,000 | Row-count floor |
| `MaxBytes` | 100 GiB | Byte-count floor |

Either condition triggers the rule.

---

## `trino.local-filter-dominates` · warn

A scan returned many rows but a Trino-side `filterPredicate` (the
"local filter") rejected most of them — i.e. the connector over-fetched
because it couldn't push the predicate down.

| Field | Default | What it does |
|---|---|---|
| `MinPhysicalRows` | 100 | Skip tiny scans |
| `MaxSelectivity` | 0.05 | Fires when `output_rows / physical_input_positions ≤ this` |
| `MaxScansInEvidence` | 5 | Cap on per-scan detail in evidence |

**Companion rule:** `trino.unpushable-expression` names the *exact*
syntactic construct that caused the over-fetch. When both fire on the
same scan, fold them into a single root-cause line in the report.

---

## `trino.duplicate-federated-scans` · warn

The same federated table is scanned 2+ times in a single query — almost
always caused by CTE inlining. Each duplicate scan is a separate
round-trip to the source.

| Field | Default | What it does |
|---|---|---|
| `MinDuplicateScans` | 2 | Number of scans of the same table that triggers the rule |

**Evidence** includes the per-scan stage IDs and physical input
positions so the agent can show how each round-trip cost the user.

---

## `trino.iceberg-metadata-table-disables-pushdown` · warn

A scan addresses an Iceberg metadata pseudo-table (`$data@<snapshot-id>`,
`$files`, `$partitions`, `$snapshots`, `$history`, `$manifests`,
`$refs`, `$properties`, `$entries`). This routes through the connector's
metadata code path and silently disables predicate pushdown +
partition pruning.

| Field | Default | What it does |
|---|---|---|
| `MaxScansInEvidence` | 5 | Cap evidence size |

**Special case:** The `$data@<snapshot-id>` form has a direct rewrite
to `FOR VERSION AS OF <id>` that preserves pushdown. The remediation
mentions this when it applies.

---

## `trino.unpushable-expression` · warn

A `LocalFilter` expression contains a syntactic construct the target
connector cannot translate to source SQL.

| Field | Default | What it does |
|---|---|---|
| `MinPhysicalRows` | 100 | Skip tiny scans |
| `MaxHitsInEvidence` | 5 | Cap evidence size |

**Pattern catalogue:**

| Pattern | Connectors | Why it can't push |
|---|---|---|
| Function wrapper on column | all | Connector doesn't have a matching scalar function |
| CAST on column | all | Type coercion blocks index use |
| Arithmetic on column | all | Computed expression isn't a column reference the connector understands |
| `CASE` expression | all | No direct SQL translation in most connectors |
| `CARDINALITY(...)` / `element_at(...)` | MongoDB | No equivalent in `$match` |
| `COALESCE(...)` wrapping a column | JDBC (MySQL, PostgreSQL) | JDBC pushdown is strict about column refs |
| `JSON_EXTRACT(...)` | MongoDB, JDBC | Mongo uses `.dot.path`, JDBC uses `JSON_VALUE` — different syntax |

Each pattern carries a connector-specific rewrite suggestion. New
patterns belong in `unpushableCatalogue` in
[`pkg/diagnose/rules/unpushable_expression.go`](../pkg/diagnose/rules/unpushable_expression.go).

---

## `trino.divergent-scan-rowcounts` · info

Sibling scans of the same table have wildly different physical input
positions — observational evidence that a value-specific predicate IS
being pushed, even when `PushPredicateIntoTableScan` was not "applied"
in the optimizer rule list.

| Field | Default | What it does |
|---|---|---|
| `MinRatio` | 10.0 | Ratio between the largest and smallest sibling scan that triggers the rule |

This is a *positive* signal — the rule helps the agent avoid telling
the user "predicate pushdown isn't working" when it actually is.

---

## `trino.missed-pushdown` · info

Important optimizer pushdown rules (`PushPredicateIntoTableScan`,
`PushProjectionIntoTableScan`, `PushAggregationIntoTableScan`,
`PushTopNIntoTableScan`) were invoked but never applied.

| Field | Default | What it does |
|---|---|---|
| _(none)_ | — | Fires when `invocations > 0 AND applied == 0` for any important rule |

**Caveat:** This rule is *informational only*. Many false positives
come from queries where the predicate genuinely can't push (e.g. uses
a non-pushable function) — `trino.unpushable-expression` catches the
specific cause when it exists.

---

## `trino.poor-selectivity` · info

`output_rows / processed_input_rows` is very low — the query reads
vastly more data than it returns.

| Field | Default | What it does |
|---|---|---|
| `Threshold` | 0.0001 | Selectivity floor |
| `AggregationOutputFloor` | 100 | Suppress when an aggregation operator emits ≤ this many rows — the low selectivity is the *purpose* of the query (`COUNT`, `SUM`, small `GROUP BY`) |

**Why the aggregation carve-out?** Without it, every `SELECT COUNT(*)`
trips the rule. The carve-out only suppresses when an aggregation
operator is present in the plan and its output is small — large
`GROUP BY` results still fire.

---

## `trino.slow-empty-scan` · info

A scan returned ZERO rows from ZERO physical input, but the stage
still spent meaningful wall-clock waiting on the connector.

| Field | Default | What it does |
|---|---|---|
| `MinWaitMs` | 500 | Sub-half-second waits are noise |
| `MaxScansInEvidence` | 5 | Cap evidence size |

**Common cause:** a MongoDB `find()` with an un-indexed predicate — the
source matched nothing but still scanned the collection to confirm it.
The same pattern shows up on JDBC connectors when the underlying
`SELECT` triggers a full table scan that finds no rows.

**Different from `trino.local-filter-dominates`:** there the source
returned rows that Trino dropped; here the source itself returned an
empty cursor while taking too long to do so.

---

## `trino.under-parallelised` · info

The query had very few drivers running for its elapsed time — usually a
sign that the cluster could parallelise the work much further (large
input but small `task.concurrency`, low `node-scheduler.max-splits-per-node`,
etc.).

| Field | Default | What it does |
|---|---|---|
| `MinElapsedMs` | 30,000 | Only fire on long-running queries |
| `MaxDriversPerSecond` | 0.5 | Drivers/elapsed_s below this is the trigger |

---

## `trino.queued-too-long` · info

The query spent ≥ 30% of its total time queued before execution
started — usually a cluster-level resource group or admission-control
issue, not a query problem.

| Field | Default | What it does |
|---|---|---|
| `Threshold` | 0.30 | `queued / elapsed` ratio floor |

---

## Adding a new rule

See [CONTRIBUTING.md](../CONTRIBUTING.md#adding-a-new-diagnostic-rule-5-steps).
