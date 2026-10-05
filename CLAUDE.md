# personal_spends

Go backend for personal expense tracking and AI categorization. Accepts bank export CSVs, categorizes expenses using Claude AI, stores results in DynamoDB, and exposes an MCP server so AI agents can query spending data.

v1 is the smallest end-to-end path: CSV upload, serial categorization, DynamoDB write.

**Learning goals for this project:** Go concurrency patterns, Claude API (tool use, and vision once images land), AWS Lambda, Terraform, MCP server implementation, agentic development practices.

## Where things are documented

- `docs/architecture.md` — implementation status, system diagram, request flows, interfaces, data model, API, MCP tools, configuration, and the ordered list of later changes (the backlog)
- `docs/decisions/` — ADRs, one per design decision
- `openspec/` — behavior specs only: REST/MCP contracts, the `Categorizer` contract, CSV parsing rules. Changes to them go through `/opsx:propose` → apply → archive

Read the relevant doc before changing a subsystem; update it in the same PR when the change alters what it says.

## Project Structure

```
cmd/          lambda/, server/ (localhost:8080), mcp/ (stdio) — thin main.go files (not started)
internal/     categorizer, parser, storage, categories; handler, mcp (not started)
terraform/    flat main.tf
docs/         architecture.md, decisions/
openspec/     specs/, changes/
samples/      test fixtures — fake data only
```

## Development

Prerequisites: Go 1.26+ (per `go.mod`), AWS CLI configured, Terraform 1.6+, `ANTHROPIC_API_KEY` in `.env`.

```bash
go run ./cmd/server        # HTTP server on localhost:8080 (once cmd/server exists)
go build ./... && go vet ./... && go test ./...
cd terraform && terraform init && terraform plan   # apply only when asked
```

## Code Conventions

- Standard Go project layout: `cmd/` for entry points, `internal/` for all shared logic
- Errors returned, not panicked — `fmt.Errorf("context: %w", err)`
- `context.Context` as the first argument in every function that does I/O
- No global state — dependencies injected via structs
- Table-driven tests in `_test.go` files alongside the code they test
- Domain interfaces with planned multiple implementations (`Categorizer`, `CategoryProvider`, `BankMappingProvider`) live in their domain package, next to the shared types and sentinel errors
- Narrow interfaces a consumer needs only for itself (like `storage.dynamoDBAPI`) are unexported and defined in the consuming package
- Constructors accept interfaces and return concrete types

## Working mode

Default: you implement everything: code, tests, Terraform, docs.
Learning mode: only when my request contains [learn]. Then don't write the code.
Explain the concept, give me the failing tests or skeleton, and review what I write
(findings ranked bug > risk > design > style, with file:line and a failing input).

In either mode, ask before `terraform apply`, before anything that costs money or touches real AWS resources, and before changing a core interface (`Categorizer`, `CategoryProvider`, `BankMappingProvider`).

## Test-first workflow

Every delegated code change, bug fix or feature alike:

1. **Red:** write table-driven tests covering the normal path, edge cases and error paths. Run them and confirm they fail for the right reason: a failing assertion, not a build error or typo. Commit the tests on their own (`#N failing tests for …`), so checking out that commit shows red.
2. **Green:** implement until the tests pass, then run the full `go build ./... && go vet ./... && go test ./...`. Commit the implementation separately.
3. **CI:** open the PR, wait with `gh pr checks <PR> --watch`, and report done only when `ci` is green. If it fails, fix it, push and wait again.

Rules while doing it:

- **Never weaken a test to make it pass.** If a test is wrong, fix it in its own commit and explain why in the PR. A weakened assertion, `t.Skip` or deleted case is a finding, not a fix.
- **Stop and ask** if the tests still fail after a few honest attempts, or if passing them would need a core interface change, a new dependency or a skipped test.
- **Tests stay offline:** no real Anthropic or AWS calls. Use fakes behind interfaces (like `storage.dynamoDBAPI`). A live test runs only when its environment variable is set, like the existing `ANTHROPIC_API_KEY` skip.
- **Exceptions:** docs/config-only changes need no tests. Terraform gets `terraform fmt -check && terraform validate && terraform plan`. Thin `cmd/*/main.go` wiring is checked by actually running it. The PR says which exception applies and what was run instead.

## Definition of done

A change is done when all of these hold:

1. `go build ./... && go vet ./... && go test ./...` passes
2. Code changes followed the test-first workflow above: the failing-tests commit comes before the implementation commit
3. Docs are verified against the change, before the PR is opened:
   - `docs/architecture.md`: Implementation Status, request flows, interfaces, data model, API, MCP tools, configuration and backlog still describe the code
   - `docs/decisions/`: a new or reversed decision gets a new ADR; a superseded ADR gets its status updated, not rewritten
   - `openspec/specs/`: a change to observable behavior (REST/MCP contracts, `Categorizer` contract, CSV parsing rules) goes through an OpenSpec change, archived once implemented
   - `README.md` and `CLAUDE.md`: status, setup, commands and conventions still hold
   - The PR body says which docs changed, or "Docs: no change needed" with the reason
4. It is committed on its own branch and has an open PR that links its issue, with every section of `.github/pull_request_template.md` filled in
5. No real bank data or secrets in the diff
6. The PR's **Not verified** section lists anything assumed rather than checked, including facts in docs that came from neither the code nor the owner
7. `ci` is green on the PR's latest commit

## Git workflow

- One issue per distinct change, docs-only changes included. Unrelated changes get separate issues and separate PRs, never a ride-along on an existing branch.
- Branch off up-to-date `main`: `feature/<slug>`, `docs/<slug>` or `chore/<slug>`. If the change depends on an unmerged PR, branch off that PR's branch and target it with the PR.
- Commit messages and PR titles start with the issue number: `#47 short description`. The PR body ends with `Closes #<issue>`.
- Never commit to `main` directly, never force-push a shared branch, and merge only when the owner asks.

## Model selection

Use the **default model** for implementation, tests, single-component refactors, Terraform/YAML boilerplate, docs, and routine debugging.

Switch to the **strongest model** for cross-subsystem architecture changes (e.g. changing a core interface), trade-off analysis spanning Go + AWS + cost + maintainability, hard bugs where several components could be at fault, and new abstractions or refactors across layers. When a conversation heads into one of these areas, suggest the owner runs `/model`.
