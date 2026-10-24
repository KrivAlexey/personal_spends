#!/usr/bin/env bash
# PreToolUse guard for the go-reviewer agent: one read-only command at a time, plus go build/vet/test.
cmd=$(python3 -c 'import json, sys; print(json.load(sys.stdin).get("tool_input", {}).get("command", ""))')
block() { echo "go-reviewer: $1" >&2; exit 2; }

[[ $cmd == *$'\n'* ]] && block "multi-line commands are not allowed"
grep -qE '[;&|<>`$]' <<<"$cmd" && block "chaining, pipes, redirects and substitution are not allowed; run one command at a time"
grep -qE '(^| )(-o|-w|--fix|--output|-exec|-toolexec)(=| |$)' <<<"$cmd" && block "flags that write files or run other programs are not allowed"
grep -qE '^(go (build|vet|test)|gofmt -l|golangci-lint run|git (diff|log|show|status|blame)|openspec (list|show|status|instructions))( |$)' <<<"$cmd" ||
	block "allowed: go build/vet/test, gofmt -l, golangci-lint run, git diff/log/show/status/blame, openspec list/show/status/instructions"
exit 0
