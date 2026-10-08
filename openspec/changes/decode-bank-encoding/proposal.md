## Why

Sparkasse CSV-CAMT exports are ISO-8859-1. The parser reads bytes as UTF-8, so umlauts in merchant and description text arrive as invalid UTF-8 and reach Claude and DynamoDB garbled (#80).

## What Changes

- A bank mapping MAY declare `encoding`: `utf-8` (default) or `iso-8859-1`. Any other value is rejected when the mapping file loads.
- The parser decodes the export from that encoding before reading CSV, so stored text and transaction IDs are computed from decoded text.
- The committed Sparkasse mapping declares `iso-8859-1`.

## Capabilities

### Modified Capabilities
- `csv-import`: exports are decoded from the bank's declared encoding.

## Impact

- `internal/parser/mapping.go`, `parser.go`, `bank_mappings.yaml` and their tests.
- `BankMappingProvider` interface unchanged; `BankMapping` gains one field.
