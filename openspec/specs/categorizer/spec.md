# categorizer Specification

## Purpose
Turns bank transactions into categorized transactions by asking an LLM to classify a batch and mapping its structured tool-use response back onto the input, safely.

## Requirements

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

### Requirement: Every Categorizer logs each batch
Every `Categorizer` implementation SHALL write one DEBUG record per `Categorize` call that gets a model answer (for an implementation that calls no model, every call), through the logger it was constructed with and with the call's context, carrying `size`, `duration_ms`, `input_tokens`, `output_tokens` and `stop_reason`. An implementation that calls no model SHALL write `0` for the token fields and `none` for `stop_reason`. It obeys the `logging` spec's bank-data rule.

#### Scenario: Batch record from the Claude categorizer
- **WHEN** `categorizer.Claude` categorizes a batch of 3 transactions at DEBUG level and the response reports 812 input tokens, 140 output tokens and `stop_reason=tool_use`
- **THEN** one DEBUG record is written with `size=3`, `input_tokens=812`, `output_tokens=140`, `stop_reason=tool_use` and a non-negative `duration_ms`

#### Scenario: Context attributes carried
- **WHEN** `Categorize` is called with a context holding `request_id`, `batch=2` and `batches=3`
- **THEN** the batch record carries those three fields

### Requirement: The batch record is written before validation
The batch record SHALL be written as soon as the model has answered, before its output is validated, so a call that then fails still has its record. It SHALL NOT be written when the call fails before a model answer exists.

#### Scenario: Truncated response still logged
- **WHEN** a response stops with `stop_reason=max_tokens` and its tool output then fails validation
- **THEN** the DEBUG record with `stop_reason=max_tokens` is written and `Categorize` returns the validation error

#### Scenario: No model answer
- **WHEN** the model API call itself fails
- **THEN** no batch record is written, and the error is returned for the boundary to log
