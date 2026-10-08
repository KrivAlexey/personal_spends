# csv-import Specification

## Purpose
Imports a bank's CSV export into stored expenses so that uploading the same or an overlapping export never stores a transaction twice and never loses a real one.
## Requirements
### Requirement: Pending transactions are not imported
The system SHALL skip every row the bank's mapping marks as pending and SHALL count it in the import result as pending-skipped. If a bank's mapping declares no pending marker, every row SHALL be treated as booked.

#### Scenario: Pending row in a Sparkasse export
- **WHEN** a Sparkasse export contains a row whose `Info` column is `Umsatz vorgemerkt`
- **THEN** no expense is stored for that row, and the import result's pending-skipped count includes it

#### Scenario: Same transaction later booked
- **WHEN** a transaction was pending in one export and appears as booked (`Umsatz gebucht`) in a later export
- **THEN** the later import stores it exactly once, as a booked expense

#### Scenario: Bank without a pending marker
- **WHEN** an export is imported for a bank whose mapping declares no pending column
- **THEN** every row is considered for import

### Requirement: Re-importing an export stores nothing twice
The system SHALL recognize a transaction already stored by any earlier import of the same bank account, and SHALL NOT store it again. A transaction is the same when every one of the bank's configured identity columns has the same text, ignoring leading and trailing whitespace.

#### Scenario: Same file uploaded twice
- **WHEN** an export is imported and then the identical file is imported again
- **THEN** the second import stores nothing and reports every booked row as already known

#### Scenario: Overlapping exports
- **WHEN** an export covering 01.09–30.09 is imported, then an export covering 15.09–15.10
- **THEN** the second import stores only the rows booked after 30.09 and reports the 15.09–30.09 rows as already known

#### Scenario: Bank re-categorizes a transaction
- **WHEN** a booked row is exported again with a different value only in a column outside the identity columns (for Sparkasse, `Kategorie`)
- **THEN** it is reported as already known and not stored again

### Requirement: Distinct transactions are never merged
The system SHALL store every booked row of an export as its own expense, even when two rows are identical in every identity column.

#### Scenario: Two identical rows in one export
- **WHEN** one export contains two booked rows with the same text in every identity column
- **THEN** two expenses are stored, and re-importing the same export stores neither again

#### Scenario: Same transaction text in two accounts
- **WHEN** two exports from different accounts of the same bank each contain a row identical except for the account column
- **THEN** both rows are stored as separate expenses

### Requirement: Known transactions keep their stored data
The system SHALL NOT re-categorize or rewrite an expense that is already stored when an export containing it is imported again.

#### Scenario: Category unchanged on re-import
- **WHEN** a stored expense's transaction appears in a later import
- **THEN** its stored category and confidence are unchanged

### Requirement: Import reports what it did
Each import SHALL report three counts: imported, already known, and pending-skipped. Their sum SHALL equal the number of data rows in the export.

#### Scenario: Mixed export
- **WHEN** an export with 10 data rows contains 2 pending rows and 3 rows already stored
- **THEN** the result reports imported 5, already known 3, pending-skipped 2

### Requirement: Expense date is the booking date
The system SHALL use the bank's booking date as the expense date for every row, including card payments whose description contains a purchase time.

#### Scenario: Card payment booked the next day
- **WHEN** a card payment's description says `2026-08-13T19:25` and its booking date is 14.08.26
- **THEN** the stored expense date is 2026-08-14

### Requirement: Exports must contain the identity columns
The system SHALL reject an export whose header lacks any of the bank's configured identity columns, and SHALL store nothing from it.

#### Scenario: Identity column missing
- **WHEN** a Sparkasse export's header has no `Sammlerreferenz` column
- **THEN** the import fails with an error naming the missing column, and no expense is stored

### Requirement: Exports are decoded from the bank's encoding
The system SHALL decode every export from the character encoding declared in the bank's mapping (UTF-8 when none is declared) before reading any column, so stored text is valid UTF-8 and transaction identity is computed over decoded text.

#### Scenario: Sparkasse export with umlauts
- **WHEN** a Sparkasse export contains the ISO-8859-1 bytes for `Grundpreis für Kontoführung` in `Verwendungszweck`
- **THEN** the stored expense's description is `Grundpreis für Kontoführung`

#### Scenario: Unsupported encoding in a mapping
- **WHEN** a bank mapping declares an encoding other than `utf-8` or `iso-8859-1`
- **THEN** loading the mapping file fails with an error naming the bank

