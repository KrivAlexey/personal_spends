# 0005 — MCP server runs locally, not in Lambda

**Status:** Accepted

## Context

The MCP server exposes expense query tools to AI agents (Claude Code). It needs to be reachable by the MCP host. Two options: deploy it to Lambda alongside the REST API, or run it as a local binary via stdio.

## Decision

The MCP server is a local Go binary (`cmd/mcp/`) that communicates via stdio. It calls the deployed REST API as an HTTP client.

## Rationale

- **Latency:** Lambda cold starts add 500ms–2s per invocation. MCP tools are called interactively during a Claude Code session; that delay is noticeable and disruptive.
- **Transport fit:** stdio is the standard MCP transport for local tools. Claude Code spawns the binary as a child process — no network configuration needed.
- **Simplicity:** no additional Lambda function, no extra API Gateway route, no extra IAM role. The MCP binary just calls the existing REST API.
- **Auth reuse:** the MCP binary reads the same `API_KEY` from local config and passes it as a header.

## Consequences

- The MCP server only works on the local machine where the binary is installed. It cannot be used from claude.ai web or other remote agents without adding an SSE transport layer.
- Phase 2 (optional): wrap the same tool handlers in an SSE HTTP server for remote access. The tool logic stays unchanged.
