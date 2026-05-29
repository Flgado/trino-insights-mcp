# Contributing to Trino Insights MCP

Thanks for considering a contribution. This project is built on a simple
premise: an LLM agent gets only as good as the **structured facts** and
**rule findings** we give it. Most contributions land in one of three
shapes:

1. **A new diagnostic rule** — detects a performance anti-pattern we don't catch today.
2. **A new connector-aware pattern** — extends `unpushable-expression` or one of the other connector-specific rules to recognise a syntactic construct another connector cannot push.
3. **A regression test from a real query** — verbatim plan text from a query that fooled us, with the expected finding pinned.

All three are valuable. The rest of this doc walks through how to do each.

---

## Project layout

```
cmd/                          # CLI entry point (stdio MCP server)
pkg/
  diagnose/                   # rule engine core (Engine, Finding, Severity)
    rules/                    # ONE FILE PER RULE — this is where most PRs land
  queryinfo/                  # QueryInfo → QueryFacts projection layer
  rest/                       # Trino REST client (read-only)
  trino.go/                   # MCP tool implementations (analyze_query, get_query_sql)
  config/                     # Viper-backed config + flag binding
  inventory/                  # Tool registry & toolset gating
  translations/               # MCP descriptor i18n helpers
scripts/                      # dev helpers (cursor-mcp.sh, ...)
docs/                         # per-rule reference, runbooks
```

The agent never sees raw `QueryInfo`. It sees `QueryFacts` — a compact,
JSON-tagged projection that strips noise and pre-computes derived metrics
(I/O wait, per-stage selectivity, connector types, top-N stages). When in
doubt: **enrich `QueryFacts` rather than asking the agent to compute**.

---

## Running locally

```bash
# Build the binary
make build

# Run all tests
make test

# Run only the rule-engine tests (fast iteration)
go test ./pkg/diagnose/...

# Smoke-run the MCP server against a real Trino coordinator
export TRINO_INSIGHTS_COORDINATOR_URL=http://localhost:8080
export TRINO_INSIGHTS_USER=admin
go run ./cmd stdio
```

You don't need a running Trino to develop a rule — the rule tests work
against synthetic `QueryFacts` fixtures.

---

## Adding a new diagnostic rule (5 steps)

This walkthrough adds a hypothetical `trino.example-rule`. Every existing
rule was built using this pattern; see [`long_blocked.go`](pkg/diagnose/rules/long_blocked.go)
for the smallest example and [`unpushable_expression.go`](pkg/diagnose/rules/unpushable_expression.go)
for the most complex.

### Step 1 — Define the rule type

Create `pkg/diagnose/rules/example_rule.go`:

```go
package rules

import (
    "fmt"

    "github.com/Flgado/trino-insights-mcp/pkg/diagnose"
    "github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

// ExampleRule fires when <plain-English condition>.
//
// Document the signal you're detecting AND the cases you deliberately
// skip — false-positive carve-outs belong here, not in code comments
// scattered through Eval().
type ExampleRule struct {
    Threshold float64 // default 0.5
}

func (r ExampleRule) ID() string { return "trino.example-rule" }

func (r ExampleRule) threshold() float64 {
    if r.Threshold <= 0 {
        return 0.5
    }
    return r.Threshold
}

func (r ExampleRule) Eval(facts *queryinfo.QueryFacts) *diagnose.Finding {
    // Guard early. Returning nil is how a rule says "I have nothing to say
    // about this query." Make those guards explicit and easy to read.
    if facts == nil || facts.IO.PhysicalInputPositions < 1000 {
        return nil
    }

    // Compute. Use named locals; avoid burying the math in the Finding
    // constructor.
    ratio := float64(facts.IO.OutputPositions) / float64(facts.IO.PhysicalInputPositions)
    if ratio >= r.threshold() {
        return nil
    }

    return &diagnose.Finding{
        RuleID:   r.ID(),
        Severity: diagnose.SeverityInfo,
        Title:    fmt.Sprintf("Example signal at %.2f%%", ratio*100),
        Details:  "Plain-English description with the specific numbers that triggered the rule.",
        Evidence: map[string]any{
            "ratio":     ratio,
            "threshold": r.threshold(),
        },
        Remediation: "Concrete next step. NOT 'optimise the query'.",
    }
}
```

### Step 2 — Register it

Add the rule to [`pkg/diagnose/rules/all.go`](pkg/diagnose/rules/all.go):

```go
func DefaultEngine() *diagnose.Engine {
    return diagnose.NewEngine(
        // ... existing rules ...
        ExampleRule{},
    )
}
```

### Step 3 — Add the rule to the agent's vocabulary

Add a one-line entry to the `Findings vocabulary` section of
[`pkg/trino.go/toolset_instructions.go`](pkg/trino.go/toolset_instructions.go).
This is what the agent sees when deciding how to interpret the finding —
keep it short and concrete:

```
- trino.example-rule — <one-line description of the signal>
```

If the rule has a non-obvious causal relationship with another rule
(e.g. it's a downstream symptom or a more specific instance), say so —
the agent uses this to fold related findings together in §5 of the
report.

### Step 4 — Write tests

Add tests to [`pkg/diagnose/rules/rules_test.go`](pkg/diagnose/rules/rules_test.go)
(if the rule fits the shared `baseFacts()` fixture) or create a dedicated
`example_rule_test.go` if you need a custom fixture.

Mandatory tests for any rule:

1. **Positive case** — `TestExampleRule_Fires`: the minimum facts shape that triggers it.
2. **Negative case** — `TestExampleRule_NotFired`: the same shape minus the triggering signal.
3. **Threshold respect** — `TestExampleRule_RespectsCustomThreshold`: confirm the public knob actually works.

Strongly encouraged:

4. **Real-world regression** — `TestExampleRule_VerbatimPlanText`: a fixture built from an actual query that confused us. Paste the offending plan text verbatim. These are gold for catching regex/parser regressions.

### Step 5 — Update docs

Add a row to the **Diagnostics** table in [README.md](README.md) and a
detailed entry in [`docs/rules.md`](docs/rules.md) covering:

- What signal the rule detects
- The default threshold(s) and how to override them
- The evidence shape (keys + types)
- Known false-positive scenarios and how the rule guards against them

---

## Adding a connector-specific pushdown pattern

Most contributions to `unpushable-expression` look like this:

1. **Find the offending plan text.** Run the query, fetch its plan with
   `analyze_query`, copy the verbatim `LocalFilter` string.
2. **Add a regex** to `unpushableCatalogue` in
   [`pkg/diagnose/rules/unpushable_expression.go`](pkg/diagnose/rules/unpushable_expression.go),
   scoped to the connector(s) it applies to.
3. **Add a regression test** to
   [`unpushable_expression_test.go`](pkg/diagnose/rules/unpushable_expression_test.go)
   using the verbatim plan text from step 1. We pin these because Trino's
   plan text varies subtly between connector versions (some use quoted
   identifiers, some don't) and a regex that works on synthetic input
   often misses real-world input.

The catalogue is the source of truth — the test file just guarantees
each entry actually matches what Trino emits in production.

---

## Evidence-shape conventions

`Finding.Evidence` is typed `any` so each rule can attach what's most
useful, but please follow these conventions:

- Use `map[string]any` (not a struct) so the JSON shape is self-describing.
- Use **snake_case** keys (`stage_id`, `physical_input_positions`) to
  match the rest of `QueryFacts`.
- When citing a stage, include its **full stage ID**
  (`20260419_080123_00042_abcde.5`) — agents render the short form for
  the user, but the full ID is the unambiguous key.
- When citing a scan, include both `stage_id` AND the fully-qualified
  table name (`catalog.schema.table`).
- For multi-hit rules (`local-filter-dominates`, `duplicate-federated-scans`),
  cap evidence at 5 entries by default and include a `scans_matched`
  count so the agent can mention "and N more not shown".

---

## PR checklist

Before opening a PR:

- [ ] `make test` is green locally
- [ ] `gofmt -l ./...` reports no files
- [ ] New rule has tests for the positive case, negative case, and any
      thresholds; new connector pattern has a regression test from real
      plan text
- [ ] README and `docs/rules.md` updated if a new rule was added or a
      threshold changed
- [ ] `toolset_instructions.go` vocabulary updated if a new rule was
      added
- [ ] Commit messages explain the *signal* and the *false-positive
      reasoning*, not just "added rule X"

We don't require a CLA. By submitting a PR you agree to license your
contribution under the MIT License (see [LICENSE](LICENSE)).

---

## Reporting bad findings

False positives erode user trust faster than missing rules. If you see a
finding that's wrong, please open an issue with:

1. The exact `rule_id`.
2. The misleading title or details.
3. The `query_id` if you can share it, otherwise an anonymised excerpt
   of the offending plan text or `LocalFilter`.
4. Why you believe the rule is wrong (the SQL was correct, the connector
   *did* push the predicate, the query was actually fast, …).

Real-world false positives and false negatives are the most valuable
feedback we get — they almost always turn into regression tests.

---

## Code of conduct

Be respectful. Disagree on technical merits. Don't be a jerk. We will
remove anyone who can't manage that.
