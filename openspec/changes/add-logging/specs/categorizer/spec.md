## ADDED Requirements

### Requirement: Every Categorizer logs each batch
Every `Categorizer` implementation SHALL write one DEBUG record per `Categorize` call, through the logger it was constructed with and with the call's context, carrying `size` (transactions in the batch), `duration_ms`, `input_tokens`, `output_tokens` and `stop_reason`. An implementation that calls no model SHALL write `0` for the token fields and `none` for `stop_reason`. The record follows the `logging` spec, including "Bank data never reaches the logs".

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
