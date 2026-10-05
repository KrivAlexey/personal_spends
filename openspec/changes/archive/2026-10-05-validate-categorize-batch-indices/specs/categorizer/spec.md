## Purpose

Turns bank transactions into categorized transactions by asking an LLM to classify a batch and mapping its structured tool-use response back onto the input, safely.

## ADDED Requirements

### Requirement: Categorize batch tool output is validated against the input batch
The system SHALL validate the `categorize_batch` tool-use result against the input transaction batch before using it to build `CategorizedTransaction` values. A result is valid only if every entry's index refers to a transaction that exists in the input batch, and every input transaction is covered by exactly one result entry.

#### Scenario: Result index out of range
- **WHEN** the tool-use result contains an entry whose index is negative or `>= len(transactions)`
- **THEN** categorization SHALL fail with a descriptive error instead of panicking, and no `CategorizedTransaction` values are returned

#### Scenario: Result count does not match input batch size
- **WHEN** the tool-use result has fewer or more entries than the input batch, or omits an input index, or repeats an index
- **THEN** categorization SHALL fail with a descriptive error instead of silently producing a mismatched or incomplete result

#### Scenario: Valid, fully-covered result
- **WHEN** the tool-use result contains exactly one entry per input transaction, each with an in-range, unique index
- **THEN** categorization SHALL succeed and return one `CategorizedTransaction` per input transaction, in input order
