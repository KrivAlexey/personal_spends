# 0006 — Flat Terraform before modules

**Status:** Accepted

## Context

Terraform code can be organised as a flat set of resources in `main.tf` or split into reusable modules. Both are valid; the trade-off is between simplicity and reusability.

## Decision

Start with a flat `terraform/main.tf`. Refactor into modules as a dedicated learning exercise once the infrastructure is working end-to-end.

## Rationale

- **Learning order:** understanding what each resource does is harder when it is hidden inside a module abstraction. Reading flat Terraform first makes the resource-to-AWS mapping obvious.
- **No reuse needed yet:** modules pay off when the same infrastructure pattern is deployed multiple times (e.g., staging and production). A single personal environment has no reuse requirement.
- **Faster iteration:** flat Terraform is quicker to write and change during initial setup.

## Consequences

- Refactoring flat resources into modules later is mechanical but requires care with `moved` blocks to avoid destroying and recreating resources.
- The refactor is itself a Terraform learning exercise: it teaches module structure, input variables, output values, and state management.
- If a second environment (e.g., staging) is needed before the refactor, some duplication is acceptable in the interim.
