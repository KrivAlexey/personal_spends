## MODIFIED Requirements

### Requirement: Exports must contain the mapped columns
The system SHALL reject an export whose header lacks any column named in the bank's `mappings`, with an error naming the missing column, and SHALL store nothing from it. The exception is a column the mapping lists in `optionalColumns`: when the header lacks it, the export imports and that field is empty on every transaction.

#### Scenario: Misspelled column in a mapping
- **WHEN** a bank mapping contains `Buchungstg: Date` and the export's header has `Buchungstag`
- **THEN** the import fails with an error naming `Buchungstg`, and no expense is stored

#### Scenario: Optional column missing
- **WHEN** a bank mapping lists `Waehrung` in `optionalColumns` and the export's header has no `Waehrung`
- **THEN** the import succeeds and every transaction has an empty `Currency`

#### Scenario: Non-optional column missing
- **WHEN** a bank mapping lists `Waehrung` in `optionalColumns` and the export's header has no `Verwendungszweck`, which is mapped but not optional
- **THEN** the import fails with an error naming `Verwendungszweck`

## ADDED Requirements

### Requirement: Optional columns are validated on load
Loading the mapping file SHALL fail with an error naming the bank when `optionalColumns` lists a column that is not a key of `mappings`, a column mapped to `Date` or `Amount`, or a column that is also an identity column.

#### Scenario: Required field marked optional
- **WHEN** a bank mapping lists the column mapped to `Amount` in `optionalColumns`
- **THEN** loading the mapping file fails with an error naming the bank

#### Scenario: Unmapped column marked optional
- **WHEN** a bank mapping lists a column absent from `mappings` in `optionalColumns`
- **THEN** loading the mapping file fails with an error naming the bank
