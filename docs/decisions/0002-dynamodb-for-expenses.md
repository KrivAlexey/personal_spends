# 0002 — DynamoDB for expense storage

**Status:** Accepted

## Context

Expenses need to be stored and queried by date range and category. Options considered: DynamoDB, PostgreSQL (RDS), SQLite (embedded).

## Decision

Use DynamoDB for expense records.

## Rationale

- **Lambda compatibility:** DynamoDB is a managed HTTP service with no persistent connections. Lambda's stateless model fits naturally — no connection pool, no warm-up.
- **No VPC required:** RDS requires a VPC, which adds Terraform complexity and increases Lambda cold start time. DynamoDB works without one.
- **Access patterns are simple:** the two queries needed (date range, category) map cleanly to a single table with one GSI. No joins required.
- **Learning value:** Single-table DynamoDB design with access-pattern-driven keys is a widely used pattern worth understanding.

## Consequences

- DynamoDB's query model is less flexible than SQL. Complex ad-hoc queries (e.g., "expenses between €50 and €100 in Q1") require a scan, which is slower and costs more reads.
- Phase 2 adds PostgreSQL RDS for categories, vendor rules, and budgets — a relational model fits that data better. Expenses stay in DynamoDB.
- Schema changes require manual data migration since DynamoDB is schemaless; discipline around attribute names is needed.
