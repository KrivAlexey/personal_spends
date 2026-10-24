---
name: ship
description: Turn the current uncommitted or unpushed changes into GitHub issue(s), branch(es) and PR(s) following this repo's git workflow. Use when the owner says "ship", "/ship", "create an issue and PR for my changes", or "open a PR for this". Also use after fixing review comments on a PR, to reply to and resolve its review threads.
---

# /ship

Follows the **Git workflow** and **Definition of done** sections in CLAUDE.md. Optional argument: a short description of the change, used for the issue title.

## 1. Inspect

```bash
git status -sb
git diff HEAD --stat
git log --oneline origin/main..HEAD
gh pr list --head "$(git branch --show-current)"
```

- If nothing is changed or unpushed, say so and stop.
- If the current branch already has an **open** PR and the changes belong to it, just commit and push there. Do not open a new issue.
- If the current branch's PR is **merged** or the changes are unrelated to it, they need a new issue and branch (continue below).

## 2. Group

Split the changes by concern: one concern per issue, branch and PR, docs-only changes included. If they span unrelated concerns, list the groups with their files and confirm the split with the owner before going on.

Never include: `.env*`, credentials, `*.tfstate`, real bank data in `samples/` or `data/`. If any of these show up in the diff, stop and tell the owner.

## 3. Check (Go changes only)

```bash
go build ./... && go vet ./... && go test ./...
```

If a check fails, report it and stop. Do not open a PR with a red build.

## 4. Issue, branch, commit, push

For each group:

```bash
gh issue create --title "<title>" --body "<what and why, 1–5 bullets>"
```

Pick the base: `origin/main` after a `git fetch`. If the change depends on an unmerged PR, use that PR's branch and target the PR at it.

```bash
git fetch -q
git switch -c <prefix>/<slug> --no-track origin/main   # uncommitted changes carry over
git add <only this group's files>
git commit -m "#<N> <short description>

Co-Authored-By: Claude <noreply@anthropic.com>"
git push -u origin <prefix>/<slug>
```

- Prefix: `feature/` for code, `docs/` for docs only, `chore/` for tooling and config.
- If `git switch` refuses because the changes conflict with the base, `git stash`, switch, then `git stash pop` and resolve.
- With several groups, commit and push one group, then go back for the next. Files that aren't committed travel with each `git switch`.

## 5. PR

```bash
gh pr create --base <base> --title "#<N> <Title>" --body "<sections from .github/pull_request_template.md:
What and why / Tests / Ran / Not verified / Docs / CI (\"pending\")>

Closes #<N>

🤖 Generated with [Claude Code](https://claude.com/claude-code)"
gh pr checks <PR> --watch
```

When the watch finishes, write the `ci` result into the PR body's CI section. The PATCH replaces the whole body, so start from the current one and change only the `## CI` section. `gh pr edit` fails on the deprecated Projects (classic) GraphQL field, so use:

```bash
gh pr view <PR> --json body -q .body > <file>
# edit only the ## CI section of <file>: result, commit SHA, run link
gh api -X PATCH 'repos/{owner}/{repo}/pulls/<PR>' -F body=@<file>
```

If `ci` failed: fix, push, then repeat this step (watch and update the CI section) until it's green, so the body never keeps a stale result.

If the PR is stacked on another PR's branch, say so in its first line and note that it retargets to `main` once that PR merges.

## 6. Review threads

After pushing fixes for review comments, answer every unresolved review thread on the PR:

```bash
# Each node: id = <thread id>, comments.nodes[0].databaseId = <comment id>
gh api graphql -f query='query{repository(owner:"<owner>",name:"<repo>"){pullRequest(number:<PR>){reviewThreads(first:100){pageInfo{hasNextPage} nodes{id isResolved comments(first:1){nodes{databaseId body}}}}}}}'
gh api -X POST repos/<owner>/<repo>/pulls/<PR>/comments/<comment id>/replies -f body="<reply>"
gh api graphql -f query='mutation($id:ID!){resolveReviewThread(input:{threadId:$id}){thread{isResolved}}}' -f id="<thread id>"
```

- **Fixed:** reply with the commit SHA(s) and what changed, then resolve the thread.
- **Not fixed** (waiting for the owner, declined, or out of scope): reply with the reason and leave the thread open.
- Skip threads where `isResolved` is true. If `hasNextPage` is true, say so instead of reporting done.
- State only what you checked; a reply is a claim the owner will rely on.

## 7. Report

Give the issue and PR URLs and the `ci` result. Report done only when it's green. Also list anything that was left out and why. **Never merge.** Merging is the owner's step.
