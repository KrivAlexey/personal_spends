## ADDED Requirements

### Requirement: Bank mappings are validated on load
The system SHALL fail to load the bank mapping file, with an error naming the bank, when a bank's `mappings` contains a field other than `Date`, `Merchant`, `Amount`, `Currency` or `Description`, maps two columns to the same field, or maps no column to `Date` or to `Amount`. Loading SHALL also fail when two mappings share a `bankName`.

#### Scenario: Misspelled field name
- **WHEN** a bank mapping contains `Datum: date`
- **THEN** loading the mapping file fails with an error naming the bank

#### Scenario: No date column
- **WHEN** a bank mapping maps no column to `Date`
- **THEN** loading the mapping file fails with an error naming the bank

#### Scenario: No amount column
- **WHEN** a bank mapping maps no column to `Amount`
- **THEN** loading the mapping file fails with an error naming the bank

#### Scenario: Field mapped twice
- **WHEN** a bank mapping maps both `Buchungstag` and `Valutadatum` to `Date`
- **THEN** loading the mapping file fails with an error naming the bank

#### Scenario: Duplicate bank name
- **WHEN** the mapping file lists `sparkasse` twice
- **THEN** loading the mapping file fails with an error naming `sparkasse`

### Requirement: Exports must contain the mapped columns
The system SHALL reject an export whose header lacks any column named in the bank's `mappings`, with an error naming the missing column, and SHALL store nothing from it.

#### Scenario: Misspelled column in a mapping
- **WHEN** a bank mapping contains `Buchungstg: Date` and the export's header has `Buchungstag`
- **THEN** the import fails with an error naming `Buchungstg`, and no expense is stored
