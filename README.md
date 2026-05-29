# Trino Insights MCP Server

[![CI](https://github.com/Flgado/trino-insights-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/Flgado/trino-insights-mcp/actions/workflows/ci.yml)
[![Release](https://github.com/Flgado/trino-insights-mcp/actions/workflows/release.yml/badge.svg)](https://github.com/Flgado/trino-insights-mcp/actions/workflows/release.yml)
[![Go Version](https://img.shields.io/badge/go-1.25%2B-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![MCP](https://img.shields.io/badge/MCP-Server-blue)](https://modelcontextprotocol.io)

MCP server that turns any LLM agent (Claude, Cursor, Copilot, …) into a **Trino performance copilot**. Ask "why was query X slow?" and get a structured root-cause report with stage-level evidence, pushdown analysis, and concrete SQL-level remediations.

**What you get:**

- "This MongoDB scan spent 3.2 s returning zero rows because the `branch_id` index is missing" — not "blocked time is high".
- "user_membership is scanned 4× in the same query due to CTE inlining; here are the redundant stages" — not "you have a lot of stages".
- "The `COALESCE(json_array_length(...))` predicate cannot push to MySQL; rewrite as `(col IS NULL OR JSON_LENGTH(col) > 0)`" — not "consider optimizing the query".

It speaks the [Model Context Protocol](https://modelcontextprotocol.io) over stdio, fetches data from Trino's REST/UI APIs, projects it into compact agent-friendly DTOs, and exposes tools for per-query deep dives. Read-only by default; never submits or cancels SQL.

---

## Quick Start

### Prerequisites

1. A running Trino coordinator with the REST API accessible
2. **Go 1.25+** (to build from source) or **Docker**

### Option 1 — Docker (recommended)

```bash
docker run -i --rm \
  -e TRINO_INSIGHTS_COORDINATOR_URL=http://your-trino:8080 \
  -e TRINO_INSIGHTS_USER=admin \
  ghcr.io/flgado/trino-insights-mcp
```

### Option 2 — Build from source

```bash
git clone https://github.com/Flgado/trino-insights-mcp.git
cd trino-insights-mcp
make build
./trino-insights-mcp stdio
```

Or without building first:

```bash
go run ./cmd stdio
```

Required environment variables (or equivalent flags):

```bash
export TRINO_INSIGHTS_COORDINATOR_URL=http://your-trino:8080
export TRINO_INSIGHTS_USER=admin
```

---

## Installation

### Cursor

Add to your `.cursor/mcp.json` (global: `~/.cursor/mcp.json`, or project-local: `.cursor/mcp.json`):

**Using Docker:**

```json
{
  "mcpServers": {
    "trino-insights": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "-e", "TRINO_INSIGHTS_COORDINATOR_URL",
        "-e", "TRINO_INSIGHTS_USER",
        "ghcr.io/flgado/trino-insights-mcp"
      ],
      "env": {
        "TRINO_INSIGHTS_COORDINATOR_URL": "http://your-trino:8080",
        "TRINO_INSIGHTS_USER": "admin"
      }
    }
  }
}
```

**Using a pre-built binary:**

```json
{
  "mcpServers": {
    "trino-insights": {
      "command": "/path/to/trino-insights-mcp",
      "args": ["stdio"],
      "env": {
        "TRINO_INSIGHTS_COORDINATOR_URL": "http://your-trino:8080",
        "TRINO_INSIGHTS_USER": "admin"
      }
    }
  }
}
```

### VS Code / GitHub Copilot

Add to your `.vscode/mcp.json`:

```json
{
  "servers": {
    "trino-insights": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "-e", "TRINO_INSIGHTS_COORDINATOR_URL",
        "-e", "TRINO_INSIGHTS_USER",
        "ghcr.io/flgado/trino-insights-mcp"
      ],
      "env": {
        "TRINO_INSIGHTS_COORDINATOR_URL": "http://your-trino:8080",
        "TRINO_INSIGHTS_USER": "admin"
      }
    }
  }
}
```

### Claude Desktop / Claude Code

```json
{
  "mcpServers": {
    "trino-insights": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "-e", "TRINO_INSIGHTS_COORDINATOR_URL",
        "-e", "TRINO_INSIGHTS_USER",
        "ghcr.io/flgado/trino-insights-mcp"
      ],
      "env": {
        "TRINO_INSIGHTS_COORDINATOR_URL": "http://your-trino:8080",
        "TRINO_INSIGHTS_USER": "admin"
      }
    }
  }
}
```

---

## Configuration

All configuration is via environment variables (prefixed `TRINO_INSIGHTS_`) or equivalent CLI flags.

| Environment Variable | Flag | Default | Description |
|---|---|---|---|
| `TRINO_INSIGHTS_COORDINATOR_URL` | `--coordinator-url` | **(required)** | Trino coordinator base URL |
| `TRINO_INSIGHTS_USER` | `--user` | `insights` | X-Trino-User header value |
| `TRINO_INSIGHTS_PASSWORD` | `--password` | | HTTP basic auth password |
| `TRINO_INSIGHTS_TOKEN` | `--token` | | Bearer token (mutually exclusive with password) |
| `TRINO_INSIGHTS_INSECURE_SKIP_TLS_VERIFY` | `--insecure-skip-tls-verify` | `false` | Skip TLS certificate verification |
| `TRINO_INSIGHTS_TIMEOUT` | `--timeout` | `15s` | HTTP request timeout |
| `TRINO_INSIGHTS_TOOLSETS` | `--toolsets` | all defaults | Comma-separated toolset IDs to enable |
| `TRINO_INSIGHTS_TOOLS` | `--tools` | | Additional tool names to enable |
| `TRINO_INSIGHTS_EXCLUDE_TOOLS` | `--exclude-tools` | | Tool names to disable |
| `TRINO_INSIGHTS_READ_ONLY` | `--read-only` | `true` | Disable write tools |
| `TRINO_INSIGHTS_CONTENT_WINDOW_SIZE` | `--content-window-size` | `16384` | Per-tool response payload soft cap (bytes) |
| `TRINO_INSIGHTS_QUERYINFO_CACHE_TTL` | `--queryinfo-cache-ttl` | `5m` | TTL for cached QueryInfo responses |
| `TRINO_INSIGHTS_QUERYINFO_CACHE_SIZE` | `--queryinfo-cache-size` | `256` | Max QueryInfo entries in memory |
| `TRINO_INSIGHTS_LOG_FILE` | `--log-file` | stderr | Log to file instead of stderr |

---

## Tools

### `analyze_query`

Analyze a single Trino query: fetch metrics from the coordinator, project them to compact facts, and run the built-in rule engine to detect performance issues.

**Input:** `query_id` (string, required) — e.g. `20260419_080123_00042_abcde`

**Returns:** headline, findings with evidence, and the underlying facts including per-stage operator pipelines, optimizer rules, and dynamic filter stats.

### `get_query_sql`

Return the full SQL text of a Trino query, sanitized and truncated to the configured content window size.

**Input:** `query_id` (string, required)

---

## Diagnostics

The rule engine detects the following issues automatically. Findings are
ordered by severity (critical → warn → info) and each one carries evidence
(stage IDs, operator names, exact filter expressions) plus a remediation
hint the agent can turn into a concrete SQL diff.

| Finding | Severity | Description |
|---|---|---|
| `trino.failed` | critical | Query failed — surfaces `error_type` + `error_code_name` |
| `trino.memory-pressure` | warn | Peak per-task memory near node limit |
| `trino.disk-spill` | warn | Spilled to disk; identifies the spilling operator |
| `trino.cpu-bound` | warn | CPU/scheduled ratio is high |
| `trino.stage-skew` | warn | Per-task CPU skew within a stage; falls back to stage-vs-stage |
| `trino.hotspot-stage` | warn | One stage carries ≥ 60% of query CPU; names the operator |
| `trino.row-explosion` | warn | Operator produces ≥ 10× more rows than it consumes (join fan-out) |
| `trino.long-blocked` | warn | Blocked ≥ 40% of scheduled AND ≥ 2 s in absolute terms (sub-second floor protects against false positives) |
| `trino.scan-too-large` | warn | ≥ 1 B rows or ≥ 100 GiB scanned |
| `trino.local-filter-dominates` | warn | A scan returned many rows but a local (non-pushed) filter rejected most; names the exact `filterPredicate` |
| `trino.duplicate-federated-scans` | warn | The same federated table is scanned 2+ times (usually CTE inlining → N parallel round-trips) |
| `trino.iceberg-metadata-table-disables-pushdown` | warn | Iceberg `$data@<snapshot-id>`, `$files`, `$partitions` etc. disable predicate pushdown; the snapshot-pinned case has a direct `FOR VERSION AS OF` rewrite that preserves pushdown |
| `trino.unpushable-expression` | warn | A `LocalFilter` contains a construct the target connector cannot push (function/CAST wrappers, MongoDB `CARDINALITY`/`element_at`, JDBC `COALESCE`, `JSON_EXTRACT` on Mongo/JDBC, …) with a connector-specific rewrite |
| `trino.divergent-scan-rowcounts` | info | Sibling scans of the same table have wildly different row counts; observational evidence that a value-specific predicate IS pushing |
| `trino.missed-pushdown` | info | Optimizer pushdown rules invoked but never applied; usually a connector limitation |
| `trino.poor-selectivity` | info | Very low output/input row ratio (excludes summary aggregations — `COUNT`, `SUM`, small `GROUP BY` — so dashboard queries don't spam the report) |
| `trino.slow-empty-scan` | info | A scan returned ZERO rows but the stage still spent meaningful wall-clock on the connector (missing index, source-side full scan, or slow plan) |
| `trino.under-parallelised` | info | Very few drivers for elapsed time |
| `trino.queued-too-long` | info | Queued time ≥ 30% of total |

See [`docs/rules.md`](docs/rules.md) for thresholds, evidence shapes, and tuning knobs per rule.

---

## Example output

Asking your agent *"Why was query 20260419_080123_00042_abcde slow?"* produces a structured Markdown report:

```markdown
> Query: 20260419_080123_00042_abcde · FINISHED · elapsed 14,320 ms · planning 1,180 ms · execution 13,140 ms

## 1. TL;DR

### 1.1 What this query does
Paginated "list active members for branch X" query that drives the Members page; returns
500 rows at a time with a total-count badge. Federates app_documents (MongoDB) for the
member document, app_reporting (MySQL) for entitlements, and app_lakehouse
(Iceberg) for the historical roster snapshot.

### 1.2 Where the elapsed time went (wall-clock attribution)
| Slice | Wall-clock | What it was |
|---|---:|---|
| Planning | 1,180 ms | Coordinator query compilation |
| max(.5 MySQL, .9 Mongo, .11 MySQL) | ~9,200 ms | Parallel federated reads stalled on COALESCE-wrapped predicates that didn't push |
| Join + aggregate (.2, .3) | ~3,600 ms | Back-pressure waiting on the scans above |
| Output (.0) | ~340 ms | — |
| **Total** | **~14,320 ms** | matches elapsed_ms |

### 1.3 One-sentence diagnosis
`user_credits` is scanned 4× for the same `branch_id` because Trino inlines CTEs by default,
and the predicate cannot push to MySQL because of `json_array_length` wrapped in `COALESCE`.

## 4. Findings

### trino.unpushable-expression  ·  warn
The `LocalFilter` on stage .11 contains `COALESCE("active", true)` — a JDBC pattern that
cannot be pushed because COALESCE wraps a bare column ref. Trino fetches every row and
filters in-process. Rewrite as `(active IS NULL OR active = true)`.

### trino.duplicate-federated-scans  ·  warn
`app_reporting.public.user_credits` is scanned in stages .5, .7, .11, .14 —
each round-trip to MySQL costs ~1.2 s. Materialise the CTE upstream or use
`session.cte_materialization_strategy='ALL'`.
```

The agent renders this with stage-by-stage tables, root-cause summaries,
recommendations ranked by impact, and a verification checklist (deltas to
recheck after the fix ships). See [`docs/rules.md`](docs/rules.md) for the
full per-rule reference (thresholds, evidence shapes, and tuning knobs).

---

## Supported connectors

The base rule set works on any Trino connector. The following have **connector-aware** intelligence (specific patterns, remediation advice, regression-tested fixtures):

| Connector | Pushdown intelligence | Connector-specific rules |
|---|---|---|
| **Hive** | ✓ | partition-aware (planned) |
| **Iceberg** | ✓ | `iceberg-metadata-table-disables-pushdown` (catches `$data@<snapshot>`, `$files`, `$partitions`, …) |
| **MongoDB** | ✓ | `unpushable-expression` (CARDINALITY, element_at, JSON_EXTRACT) |
| **MySQL** (JDBC) | ✓ | `unpushable-expression` (COALESCE-on-column, function wrappers) |
| **PostgreSQL** (JDBC) | ✓ | `unpushable-expression` (COALESCE-on-column, function wrappers) |
| Memory | basic | — |
| Other JDBC / FS connectors | basic | inherits agnostic rules |

Missing a connector? See [CONTRIBUTING.md](CONTRIBUTING.md) — adding a new connector-specific pattern is usually a one-file rule + a regression test using verbatim plan text.

---

## Development

```bash
# Build
make build

# Run tests
make test

# Lint (requires golangci-lint)
make lint

# Build Docker image
make docker

# Run locally during development
go run ./cmd stdio
```

---

## Contributing

We welcome new rules, new connector patterns, and bug reports. See [CONTRIBUTING.md](CONTRIBUTING.md) for:

- A 5-step walkthrough on how to add a new diagnostic rule
- The evidence-shape conventions findings should follow
- How to add a regression test using verbatim plan text from a real query
- The PR review checklist

Found a query Trino Insights gets wrong? Open an issue with the rule ID, the misleading output, and (if you can share it) a query ID or anonymised plan snippet. Real-world false positives and false negatives are the most valuable feedback we get.

---

## License

[MIT](LICENSE) © Joao Folgado
