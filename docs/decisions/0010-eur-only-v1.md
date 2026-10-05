# 0010 — EUR only for v1

**Status:** Accepted

## Context

Bank exports carry a currency column, and the data model stores `currency` per expense. Supporting several currencies means choosing a base currency, getting exchange rates for each transaction date, and summing converted amounts.

## Decision

v1 assumes every amount is in EUR. `currency` is still stored as read from the CSV, but summaries add amounts without converting them.

## Rationale

- **Real data:** the only supported bank so far (Sparkasse) exports EUR accounts.
- **Cheap to extend:** storing `currency` per item now means multi-currency support later needs no data migration, only conversion at query time.

## Consequences

- A non-EUR transaction would be summed as if it were EUR. Acceptable while every supported bank account is in EUR; rejecting non-EUR rows at upload is a small guard to add if that changes.
- Multi-currency is added only if a non-EUR account appears.
