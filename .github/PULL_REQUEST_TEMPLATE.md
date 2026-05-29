<!-- Thanks for contributing! Please fill in the sections below. -->

## Summary

<!-- 1-3 sentences. What does this PR do, and why? -->

## Type of change

<!-- Tick all that apply. -->

- [ ] New diagnostic rule
- [ ] New connector-aware pattern (unpushable-expression catalogue entry)
- [ ] Fix for a false-positive / wrong finding
- [ ] New MCP tool or new field in `QueryFacts`
- [ ] Threshold or configuration change
- [ ] Documentation
- [ ] Build / CI / infrastructure
- [ ] Other (describe in summary)

## Related issues

<!-- e.g. Closes #123, Refs #456 -->

## How was this tested?

<!--
For rule changes: which positive and negative cases are covered by the new tests?
For pushdown patterns: include a regression test using verbatim plan text from a real query.
For threshold changes: explain why the new default is right.
-->

## Checklist

- [ ] `make test` passes locally
- [ ] `gofmt -l ./...` reports no files
- [ ] New rule has tests for positive, negative, and threshold-respect cases
- [ ] New connector-aware pattern has a regression test using verbatim plan text
- [ ] `README.md` and `docs/rules.md` updated when a new rule or threshold landed
- [ ] `pkg/trino.go/toolset_instructions.go` vocabulary updated when a new rule landed
- [ ] `CHANGELOG.md` updated under `[Unreleased]`
- [ ] Commit messages explain the *signal* and the *false-positive reasoning*, not just "added rule X"
