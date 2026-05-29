# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Canonical report skeleton** baked into the agent instructions so every
  `analyze_query` response follows the same 8-section Markdown structure
  (Header → TL;DR → SQL → Stage map → Findings → Root cause → Recommendations
  → Expected impact → Verification checklist).
- **Wall-clock vs cumulative guidance** in the agent instructions, teaching
  the calling LLM to translate cumulative driver totals into elapsed-wall-clock
  attribution (`§1.2 Where the elapsed time went`) and to never sum parallel
  sibling stages.
- **Plain-English query description** as a required §1.1 subsection, with
  an explicit anti-fabrication rail (default to descriptive when intent is
  unclear; never invent feature names).
- **`trino.iceberg-metadata-table-disables-pushdown`** diagnostic rule —
  detects Iceberg metadata pseudo-tables (`$data@<snapshot-id>`, `$files`,
  `$partitions`, `$snapshots`, `$history`, `$manifests`, `$refs`,
  `$properties`, `$entries`) that disable predicate pushdown and partition
  pruning. The `$data@<snapshot>` form ships with a direct `FOR VERSION AS
  OF` rewrite that preserves pushdown.
- **`trino.unpushable-expression`** diagnostic rule — names the exact
  syntactic construct in a `LocalFilter` that the target connector cannot
  push (function/CAST wrappers, MongoDB `CARDINALITY` / `element_at`,
  JDBC `COALESCE` on column refs, `JSON_EXTRACT` on Mongo/JDBC) and provides
  a connector-specific rewrite suggestion.
- **`trino.slow-empty-scan`** diagnostic rule — catches the "source matched
  nothing but the round-trip was still slow" pattern (typical of MongoDB
  queries with un-indexed predicates).
- **`trino.duplicate-federated-scans`** and **`trino.divergent-scan-rowcounts`**
  diagnostic rules — detect CTE-inlining redundancy and value-specific
  pushdown evidence respectively.
- **`trino.local-filter-dominates`** diagnostic rule — names the exact
  `filterPredicate` that caused a scan to over-fetch.
- **Pre-computed I/O facts** in `QueryFacts`: per-stage `io_wait_ms` and
  `io_wait_kind`, per-connector roll-ups (`connector_io`), and top-N stages
  by I/O and CPU (`top_io_stages`, `top_cpu_stages`). Removes the need for
  the LLM to recompute these on every analysis.
- **Prepared-statement literal substitution** in `get_query_sql`: when the
  query was issued via `EXECUTE … USING …`, the response now also returns
  `prepared_query_with_literals`, the parameterised template with concrete
  values substituted in.
- `CONTRIBUTING.md`, `docs/rules.md`, GitHub
  Actions CI workflow, goreleaser config, multi-arch Docker builds via
  the release workflow.

### Changed

- **`trino.long-blocked`** now requires an absolute floor (`MinBlockedMs`,
  default 2,000 ms) in addition to the ratio threshold. Sub-second queries
  with a high blocked-to-scheduled ratio no longer fire — a recurring false
  positive identified during dogfooding.
- **`trino.poor-selectivity`** now suppresses when an aggregation operator
  emits a small number of rows (`AggregationOutputFloor`, default 100).
  `SELECT COUNT(*)` and small `GROUP BY` queries are intentional summary
  reductions, not poor selectivity.
- Report §1 is now a tight three-section TL;DR (What this query does /
  Where the elapsed time went / One-sentence diagnosis) plus an optional
  connector-breakdown row. The old cumulative-totals table has been
  removed; that information already lives in the header block and §3.1.
- README rewritten with a concrete pitch, a full 19-rule diagnostics
  table with severities, a supported-connectors matrix, and an inline
  example output section.

### Fixed

- Sub-second queries no longer trigger spurious `trino.long-blocked`
  findings.
- Summary aggregations (`SELECT COUNT(*)`, small `GROUP BY`) no longer
  trigger spurious `trino.poor-selectivity` findings.
- Repository-wide gofmt drift cleaned up so the new CI's formatting gate
  is green from day one.

---

## How to read this changelog

- **Added** — new tools, rules, fields, or capabilities.
- **Changed** — behavioural changes to existing rules or tools. Look here
  when a query that used to produce N findings now produces N-1.
- **Fixed** — bugs that produced wrong findings or wrong measurements.
- **Removed** — dropped tools, rules, or fields. Will always include a
  migration note.

Each release links its findings, fields, and rule IDs back to the docs in
`docs/rules.md` so old changelog entries stay browsable as the surface
area grows.
