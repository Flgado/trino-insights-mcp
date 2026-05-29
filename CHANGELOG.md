# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-05-29

First public release of **Trino Insights MCP Server** — an MCP server that turns
LLM agents into Trino performance copilots. Connect it to your coordinator,
point Cursor/Claude/Copilot at it, and ask "why was query X slow?" to get a
structured root-cause report with stage-level evidence and SQL-level fixes.

### Added

- **MCP server** over stdio with tools for query analysis, SQL retrieval, and
  diagnostic rule evaluation (read-only by default).
- **19 diagnostic rules** covering CPU bottlenecks, memory pressure, stage skew,
  scan pushdown failures, Iceberg metadata tables, federated scan duplication,
  and more — see [docs/rules.md](docs/rules.md) for the full catalog.
- **Canonical report skeleton** in the agent instructions so every
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
- **`trino.iceberg-metadata-table-disables-pushdown`** — detects Iceberg
  metadata pseudo-tables (`$data@<snapshot-id>`, `$files`, `$partitions`, etc.)
  that disable predicate pushdown; includes a `FOR VERSION AS OF` rewrite for
  `$data@<snapshot>`.
- **`trino.unpushable-expression`** — names the exact syntactic construct in a
  `LocalFilter` that the connector cannot push, with connector-specific rewrite
  suggestions (MongoDB, JDBC, etc.).
- **`trino.slow-empty-scan`** — catches scans that matched nothing but still
  took a long round-trip (typical of un-indexed MongoDB predicates).
- **`trino.duplicate-federated-scans`** and **`trino.divergent-scan-rowcounts`**
  — detect CTE-inlining redundancy and value-specific pushdown evidence.
- **`trino.local-filter-dominates`** — names the exact `filterPredicate` that
  caused a scan to over-fetch.
- **Pre-computed I/O facts** in `QueryFacts`: per-stage `io_wait_ms`,
  per-connector roll-ups, and top-N stages by I/O and CPU.
- **Dynamic filter stats** in `QueryFacts` (`dynamic_filters`: total, completed,
  lazy, replicated) with agent guidance when filters fail to complete — e.g.
  incomplete dynamic filters on a join suggest reordering build/probe sides or
  adding explicit predicates (surfaced in analysis, not a standalone rule).
- **Prepared-statement literal substitution** in `get_query_sql` — returns
  `prepared_query_with_literals` when the query was issued via
  `EXECUTE … USING …`.
- **Distribution**: pre-built binaries (Linux/macOS/Windows), multi-arch Docker
  images (`ghcr.io/flgado/trino-insights-mcp`), GitHub Actions CI, and GoReleaser
  release automation.
- **Docs & community**: `README.md`, `CONTRIBUTING.md`, `docs/rules.md`, issue
  templates, Dependabot, and CODEOWNERS.

### Changed

- **`trino.long-blocked`** now requires an absolute floor (`MinBlockedMs`,
  default 2,000 ms) in addition to the ratio threshold.
- **`trino.poor-selectivity`** suppresses when an aggregation emits a small
  number of rows (`AggregationOutputFloor`, default 100).
- Report §1 is now a tight three-section TL;DR instead of a cumulative-totals
  table.
- README rewritten with pitch, diagnostics table, connectors matrix, and
  example output.

### Fixed

- Sub-second queries no longer trigger spurious `trino.long-blocked` findings.
- Summary aggregations (`SELECT COUNT(*)`, small `GROUP BY`) no longer trigger
  spurious `trino.poor-selectivity` findings.

[0.1.0]: https://github.com/Flgado/trino-insights-mcp/releases/tag/v0.1.0

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
