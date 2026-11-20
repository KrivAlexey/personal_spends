# Tasks

No Go code: the script follows the CLAUDE.md exception for tooling (checked by actually running it, as below), so there is no failing-tests commit. The PR names this exception.

## 1. Issue and branch

- [x] 1.1 Open a GitHub issue for the reset script and branch `chore/reset-local-stack` off up-to-date `main`; verify with `git branch --show-current`

## 2. Reset script

- [x] 2.1 Write executable `scripts/reset-local.sh` per design.md (refuse busy `:8080`, `docker compose down` + `up -d`, wait for `:8000`, `terraform init -input=false`, create the `local` workspace if missing without changing the selection, delete its state files, `TF_WORKSPACE=local terraform apply -auto-approve`); verify `bash -n` passes and `shellcheck` is clean if installed
- [x] 2.2 Verify a dirty stack is wiped: with the compose stack up and the server running via `scripts/run-local.sh` (`CATEGORIZER=fake`), upload `samples/sparkasse_fake.csv`; confirm the script refuses while the server runs; stop the server, run the script, then check `aws dynamodb scan --table-name personal-spends-expenses --endpoint-url http://localhost:8000` returns `Count: 0`, `psql` shows no `uploads` table, and `aws s3 ls s3://personal-spends-uploads --endpoint-url http://localhost:9090` is empty
- [x] 2.3 Verify the clean stack works again: start the server, upload the same sample, and confirm it imports 5 rows and skips 1 pending (not "same upload_id")
- [x] 2.4 Verify idempotence and safety: run the script twice in a row and once with the stack down; both succeed; `terraform -chdir=terraform workspace show` still prints the workspace selected before (normally `default`) and `terraform/terraform.tfstate` is unchanged (`git`-ignored, so compare `sha256sum` before/after)
- [x] 2.5 Update README "Offline" section: replace the manual restart/state-delete instructions with `scripts/reset-local.sh`, keeping the first-time setup steps; check `docs/architecture.md` and `CLAUDE.md` for local-workflow text that needs the same pointer; verify each documented command runs as written

## 3. Review and PR

- [ ] 3.1 Run the go-reviewer agent on `git diff main...HEAD`, `git log --oneline main..HEAD` and the `go build ./... && go vet ./... && go test ./...` result; fix bug and risk findings
- [ ] 3.2 Open the PR with every template section filled (tooling exception and the manual runs above as verification; Docs: README; Not verified: anything not run), `Closes #<issue>`; wait for `gh pr checks <PR> --watch` to show `ci` green

## Workflow follow-up

- Archive the change with `/opsx:archive` once the PR is merged.
