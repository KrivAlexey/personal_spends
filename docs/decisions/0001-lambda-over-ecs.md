# 0001 — Lambda over ECS

**Status:** Accepted

## Context

The backend needs compute to run the expense processing pipeline. Two realistic options for a Go service on AWS are Lambda (serverless functions) and ECS Fargate (containerised service).

## Decision

Use Lambda for all compute.

## Rationale

- **Cost:** Personal-scale workload. Lambda scales to zero between uploads; a Fargate task runs 24/7 at ~$15–20/month even with no traffic.
- **Simplicity:** No VPC required, no load balancer, no ECS task definitions, no service discovery. Less Terraform to maintain.
- **Go fit:** Go binaries compile to a single static binary with fast cold starts (~50ms), making Lambda's cold start penalty negligible.
- **Learning goal:** Lambda + API Gateway is the more common serverless pattern to know; ECS is better learned on a team project with persistent workloads.

## Consequences

- Max execution time is 15 minutes per invocation. Acceptable — even a 1,000-row CSV with batched Claude calls finishes well under that.
- Adding VPC later (for RDS in Phase 2) will increase cold start time slightly. RDS Proxy mitigates this.
- If workload grows significantly, migrating to ECS is straightforward: the same `internal/` packages work unchanged; only `cmd/lambda/main.go` is replaced.
