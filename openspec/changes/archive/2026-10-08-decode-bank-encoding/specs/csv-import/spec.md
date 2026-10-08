## ADDED Requirements

### Requirement: Exports are decoded from the bank's encoding
The system SHALL decode every export from the character encoding declared in the bank's mapping (UTF-8 when none is declared) before reading any column, so stored text is valid UTF-8 and transaction identity is computed over decoded text.

#### Scenario: Sparkasse export with umlauts
- **WHEN** a Sparkasse export contains the ISO-8859-1 bytes for `Grundpreis für Kontoführung` in `Verwendungszweck`
- **THEN** the stored expense's description is `Grundpreis für Kontoführung`

#### Scenario: Unsupported encoding in a mapping
- **WHEN** a bank mapping declares an encoding other than `utf-8` or `iso-8859-1`
- **THEN** loading the mapping file fails with an error naming the bank
