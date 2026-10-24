---
paths:
  - "**/*.go"
---

# Go skills

Before writing or reviewing Go code, read and follow these skills (`.agents/skills/<name>/SKILL.md`, plus their `references/` when relevant):

- Always: `golang-code-style`, `golang-naming`, `golang-error-handling`, `golang-structs-interfaces`
- `context.Context` propagation, cancellation, timeouts: `golang-context`
- Goroutines, channels, mutexes, errgroup: `golang-concurrency`
- `_test.go` files: `golang-testing`, `golang-stretchr-testify`
- Doc comments: `golang-documentation`
- golangci-lint config or `nolint`: `golang-lint`

CLAUDE.md conventions win where a skill disagrees. For a review, use the `go-reviewer` agent, which has these skills preloaded.
