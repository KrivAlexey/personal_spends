---
name: implement-issues
description: Implement GitHub issues end to end in parallel worktrees, one subagent per lane of independent issues, from failing tests to a green PR. Use when the owner says "/implement-issues", "implement issue 65", "implement issues 65 102", or "implement all ready issues".
---

# /implement-issues

Usage:

- `/implement-issues 65`: one issue
- `/implement-issues 65 101 140`: these issues
- `/implement-issues ready`: every open issue labeled `ready`

The owner's invocation authorizes the whole flow for the listed issues, including the OpenSpec propose step, whose own skill would otherwise stop after planning. It does not authorize merging, `terraform apply`, live Anthropic or AWS calls, or changes to `.github/workflows/`.

CLAUDE.md wins wherever this file is silent or seems to disagree: test-first workflow, definition of done, git workflow.

## 1. Select (main session)

```bash
git switch main && git pull --ff-only     # stop and ask if either fails
gh issue list --state open --label ready --json number,title,labels,body   # for "ready"
gh issue view <N> --json number,title,state,labels,body,comments           # for explicit numbers
```

- Skip, and list in the report, any issue that is closed, labeled `blocked` or `needs-clarification`, or already has an open PR (`gh pr list --state open --search "<N> in:title"`).
- An explicit number without the `ready` label: proceed, but say so in the report.

## 2. Plan the lanes (main session)

Issues that touch the same files, or where one depends on another, run **in one lane, one after another**, each on its own branch and PR. A later issue in a lane stacks on the earlier one's branch (CLAUDE.md "Branch off up-to-date `main`"). Independent issues get their own lanes and run in parallel.

Decide overlap from the issue bodies and `git grep` on the paths and symbols they name. Example: #65, #102, #103 all edit `internal/categorizer/claude.go`, and #100's 50-row case needs #65, so they form one lane: 65 → 102 → 103 → 100.

Show the lanes to the owner in one short table before launching. Then launch.

## 3. Launch (main session)

One `Agent` call per lane, all in one message, `isolation: "worktree"`, run in the background. Give each agent:

- the lane's issue numbers, in order, with titles and bodies
- the base for the first branch (`origin/main`)
- the whole of section 4 below, verbatim

Do not start a lane's next issue yourself while its agent runs. Wait for the completion notifications.

## 4. Per issue (subagent, inside its worktree)

You can't ask the owner anything. When a rule below says "stop", stop work on this issue, push what you have if it builds, and report the blocker. In a lane, a stopped issue stops the issues after it.

### 4.1 Branch

```bash
git fetch origin
git switch --no-track -c <type>/<slug> <base>   # feature/, docs/ or chore/; base = origin/main or origin/<previous lane branch>
git push -u origin HEAD
```

Never commit to `main`. Never force-push.

### 4.2 OpenSpec, only when behavior changes

`openspec/` covers observable behavior only: REST/MCP contracts, the `Categorizer` contract, CSV parsing rules, operational logging (`openspec/config.yaml`). Decide which applies:

- **An active change already covers the issue** (`openspec list`): read its proposal, design and tasks, and follow its tasks.
- **Behavior changes and no change exists:** create one with the openspec-propose skill, run `openspec validate <change> --strict`, and commit it (`#N OpenSpec change for …`).
- **No behavior change** (refactor, dead code, internal limits, tests only): no OpenSpec change. Say so in the PR's Docs section.

### 4.3 Red

Read the Go skills `.claude/rules/go.md` lists. Write table-driven tests for the normal path, edge cases and errors (the go-test-writer agent can do it). Run them and confirm they fail **on an assertion**, not on a build error. Commit them alone: `#N failing tests for …`.

Exceptions from CLAUDE.md (docs/config only, Terraform, thin `cmd/*/main.go` wiring) skip this step; the PR says which exception applies and what ran instead.

### 4.4 Green

Implement until it passes, then run the full check:

```bash
go build ./... && go vet ./... && go test ./...
gofmt -l .                                  # must print nothing
```

If the diff touches a `*.sh` file: `shellcheck <files>`. If `shellcheck` isn't installed, put that in **Not verified**; don't install it.

Commit the implementation separately: `#N <what>`.

Stop if: tests still fail after a few honest attempts, passing needs a core interface change (`Categorizer`, `CategoryProvider`, `BankMappingProvider`), a new direct dependency, a skipped or weakened test, or a change under `.github/workflows/` (the agent token can't push workflow files). Live tests (`ANTHROPIC_API_KEY` set, real AWS) are never run; list them in **Not verified**.

### 4.5 Self-review

Run the go-reviewer agent with `git diff <base>...HEAD`, `git log --oneline <base>..HEAD` and the check output. Fix its bug and risk findings.

For shell scripts and any run instructions you added, also check:

- `set -euo pipefail` holds: no unset variable reads, `"${VAR:-}"` where a variable may be missing
- Linux/WSL: `xdg-open`, never `open`; no macOS-only flags (`sed -i ''`, BSD `date`)
- no `pkill -f <pattern>` (it can match your own shell); kill by tracked PID
- every server or background process started is stopped, and its port freed, on exit and on failure (`trap … EXIT`)

### 4.6 Docs

Check against the definition of done in CLAUDE.md: `docs/architecture.md` (status, flows, interfaces, data model, API, config, backlog), `docs/decisions/` (new or reversed decision → ADR), `README.md`, `CLAUDE.md`. Fix what no longer holds, in this branch.

### 4.7 Archive the OpenSpec change

If 4.2 created or used a change and all its tasks are done: archive it with the openspec-archive-change skill (it syncs the delta into `openspec/specs/`), and commit (`#N archive OpenSpec change …`). Do it before opening the PR, so the reviewed diff includes the synced spec.

### 4.8 PR and CI

Follow `.claude/skills/ship/SKILL.md` step 5: open the PR with every section of `.github/pull_request_template.md` filled, ending with `Closes #N` and the attribution line. A stacked PR says so in its first line.

```bash
gh pr checks <PR> --watch
```

If `ci` fails: read the failing log (`gh run view <run> --log-failed`), fix the cause, push, watch again. At most **5** rounds; then stop.

When green, write the result into the `## CI` section with `gh api -X PATCH` as ship step 5 shows (`gh pr edit` fails here). Then **verify the body saved**:

```bash
gh pr view <PR> --json body -q .body | grep -A3 '^## CI'
```

If the section doesn't show the new result and SHA, patch again; still wrong after a second try → report it.

### 4.9 Review comments

The Claude Review workflow comments on every push. After CI is green, wait for it (it runs as the `review` check, so `gh pr checks <PR> --watch` covers it), then handle every unresolved thread per ship step 6: fix what's right, reply `Fixed in <SHA>` or `Not fixed: <reason>`, resolve fixed threads. A fix means a push, so go back to 4.8 (the 5-round cap counts all CI rounds). A comment asking for anything in the 4.4 stop list → reply `Not fixed`, leave it open, report it.

After the last push, re-verify the PR body (4.8) once more.

### 4.10 Report to the main session

One line per issue: issue, branch, PR URL, `ci` result on the last commit, open review threads, blocker (or "none"), and what is in **Not verified**.

## 5. Report (main session)

When all lanes finish, give the owner one table:

| Issue | PR | CI | Review threads open | Blocker needing your decision |
|---|---|---|---|---|

Then list the skipped issues with the reason. **Never merge.** Never relabel issues unless asked.
