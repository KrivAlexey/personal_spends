# personal_spends

Go backend for personal expense tracking and AI categorization. Accepts bank export CSVs, categorizes expenses using Claude AI, stores results in DynamoDB, and exposes an MCP server so AI agents can query spending data.

v1 is the smallest end-to-end path: CSV upload, serial categorization, DynamoDB write.

**Learning goals for this project:** Go concurrency patterns, Claude API (tool use, and vision once images land), AWS Lambda, Terraform, MCP server implementation, agentic development practices.

## Where things are documented

- `docs/architecture.md` — system diagram, request flows, interfaces, data model, API, MCP tools, configuration, and the ordered list of later changes (the backlog)
- `docs/decisions/` — ADRs, one per design decision
- `openspec/` — specs and change proposals

Read the relevant doc before changing a subsystem; update it in the same PR when the change alters what it says.

## Project Structure

```
cmd/          lambda/, server/ (localhost:8080), mcp/ (stdio) — thin main.go files
internal/     handler, categorizer, parser, storage, categories, mcp
terraform/    flat main.tf
docs/         architecture.md, decisions/
samples/      test fixtures — fake data only
```

## Development

Prerequisites: Go 1.22+, AWS CLI configured, Terraform 1.6+, `ANTHROPIC_API_KEY` in `.env`.

```bash
go run ./cmd/server        # HTTP server on localhost:8080
go build ./... && go vet ./... && go test ./...
cd terraform && terraform init && terraform plan   # apply only when asked
```

## Code Conventions

- Standard Go project layout: `cmd/` for entry points, `internal/` for all shared logic
- Errors returned, not panicked — `fmt.Errorf("context: %w", err)`
- `context.Context` as the first argument in every function that does I/O
- No global state — dependencies injected via structs
- Table-driven tests in `_test.go` files alongside the code they test
- Interfaces defined in the package that uses them, not the package that implements them

## Working mode

Default: you implement everything: code, tests, Terraform, docs.
Learning mode: only when my request contains [learn]. Then don't write the code.
Explain the concept, give me the failing tests or skeleton, and review what I write
(findings ranked bug > risk > design > style, with file:line and a failing input).

In either mode, ask before `terraform apply`, before anything that costs money or touches real AWS resources, and before changing a core interface (`Categorizer`, `CategoryProvider`, `BankMappingProvider`).

## Definition of done

A change is done when all of these hold:

1. `go build ./... && go vet ./... && go test ./...` passes
2. New logic has table-driven tests, including the error paths
3. `docs/architecture.md` / ADRs reflect the change (new decision → new ADR)
4. It is committed on its own branch and has an open PR that links its issue
5. No real bank data or secrets in the diff

## Git workflow

- One issue per distinct change, docs-only changes included. Unrelated changes get separate issues and separate PRs, never a ride-along on an existing branch.
- Branch off up-to-date `main`: `feature/<slug>`, `docs/<slug>` or `chore/<slug>`. If the change depends on an unmerged PR, branch off that PR's branch and target it with the PR.
- Commit messages and PR titles start with the issue number: `#47 short description`. The PR body ends with `Closes #<issue>`.
- Never commit to `main` directly, never force-push a shared branch, and merge only when the owner asks.

## Model selection

Use the **default model** for implementation, tests, single-component refactors, Terraform/YAML boilerplate, docs, and routine debugging.

Switch to the **strongest model** for cross-subsystem architecture changes (e.g. changing a core interface), trade-off analysis spanning Go + AWS + cost + maintainability, hard bugs where several components could be at fault, and new abstractions or refactors across layers. When a conversation heads into one of these areas, suggest the owner runs `/model`.
