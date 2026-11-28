## MODIFIED Requirements

### Requirement: Exports must contain the mapped columns
The system SHALL reject an export whose header lacks any column named in the bank's `mappings`, with an error naming the missing column, and SHALL store nothing from it. The exception is a column the mapping lists under `optionalColumns`: if the header lacks it, the field it maps to stays empty and the import proceeds. A `Date` or `Amount` column SHALL NOT be optional: loading a mapping file that lists one, a column absent from `mappings`, a duplicate or empty name, an identity column or the pending column under `optionalColumns` fails with an error naming the bank and the column.

#### Scenario: Misspelled column in a mapping
- **WHEN** a bank mapping contains `Buchungstg: Date` and the export's header has `Buchungstag`
- **THEN** the import fails with an error naming `Buchungstg`, and no expense is stored

#### Scenario: Optional column absent from the export
- **WHEN** a mapping lists `Waehrung` under `optionalColumns` and the export's header has no `Waehrung`
- **THEN** the import succeeds and every transaction has an empty `Currency`

#### Scenario: Optional column present in the export
- **WHEN** a mapping lists `Waehrung` under `optionalColumns` and the export's header has it
- **THEN** `Currency` is read from it as usual

#### Scenario: Non-optional column still required
- **WHEN** a mapping lists `Waehrung` under `optionalColumns` and the export's header lacks `Verwendungszweck`, which is mapped but not optional
- **THEN** the import fails with an error naming `Verwendungszweck`

#### Scenario: Date or Amount column marked optional
- **WHEN** a mapping lists the column mapped to `Date` or `Amount` under `optionalColumns`
- **THEN** loading the mapping file fails with an error naming the bank and the column
