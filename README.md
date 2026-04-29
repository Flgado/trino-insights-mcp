# Trino Insights MCP Server

MCP server that turns LLM agents into a **Trino performance copilot**. It speaks the [Model Context Protocol](https://modelcontextprotocol.io) over stdio, fetches data from Trino's REST/UI APIs, projects it into compact agent-friendly DTOs, and exposes tools for per-query deep dives. Read-only by default; never submits or cancels SQL.

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

The rule engine detects the following issues automatically:

| Finding | Description |
|---|---|
| `trino.failed` | Query failed — error code and remediation |
| `trino.cpu-bound` | CPU/scheduled ratio is high |
| `trino.memory-pressure` | Peak per-task memory near node limit |
| `trino.disk-spill` | Spilled to disk; identifies the operator |
| `trino.queued-too-long` | Queued time >= 30% of total |
| `trino.stage-skew` | Per-task CPU skew within a stage |
| `trino.hotspot-stage` | One stage carries >= 60% of query CPU |
| `trino.scan-too-large` | >= 1B rows or >= 100 GiB scanned |
| `trino.poor-selectivity` | Very low output/input row ratio |
| `trino.under-parallelised` | Very few drivers for elapsed time |
| `trino.long-blocked` | Blocked >= 40% of scheduled time |
| `trino.row-explosion` | Operator produces >= 10x more rows than it consumes |
| `trino.missed-pushdown` | Optimizer pushdown rules tried but never applied |

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

## License

MIT
