# Proposal

## Why

Testing the pipeline end to end means uploading exports into the compose stack, and repeating a run from a clean state takes several manual steps: restart three containers, delete the stale `local` Terraform state, re-apply the DynamoDB table in the `local` workspace, and switch back to `default`. Missing a step (often the stale state) leaves a stack where the server starts but the table is gone. One command should get back to a clean, empty, ready stack.

## What Changes

- New `scripts/reset-local.sh`: wipes all local test data and recreates the empty stack. It recreates the compose containers (DynamoDB Local, Postgres, S3Mock; all in memory, so recreating them wipes them), drops the `local` workspace's Terraform state, and re-applies the DynamoDB table into DynamoDB Local.
- The script touches only the local stack: Terraform runs with `TF_WORKSPACE=local`, so it never selects or applies the real-AWS workspace and leaves the saved workspace selection alone.
- It refuses to run while something answers on `localhost:8080`, because a running server would keep running without its Postgres table (the server creates that table only at startup).
- README "Offline" section: the manual restart steps are replaced by a pointer to the script.
- Out of scope: real AWS data (the real DynamoDB table and uploads bucket), starting the server, re-uploading samples.

## Capabilities

### New Capabilities

None. This is developer tooling: no REST/MCP contract, `Categorizer` contract, CSV parsing rule or operational log changes, so the change sets `skip_specs: true`.

### Modified Capabilities

None.

## Impact

- New: `scripts/reset-local.sh`
- Docs: `README.md` (Offline section); `docs/architecture.md` only if it lists the local scripts
- No Go code, no Terraform changes, no new dependencies. Destroys only data in the compose containers and `terraform/terraform.tfstate.d/local/`.
