# 0008 — Bank CSV mappings written by hand

**Status:** Accepted

## Context

Every bank exports CSVs differently: column names, delimiter (`,` or `;`), date format, and decimal separator (`1.234,56` vs `1,234.56`). The early design had Claude read the first rows and detect the layout at runtime (`parser.DetectSchema()`), but it was never built.

## Decision

Each bank's layout is written by hand in `internal/parser/bank_mappings.yaml` and committed when a new bank's export first shows up. `Parser.ParseCSV` looks the bank up through the `BankMappingProvider` interface. An unknown bank is an error (`ErrBankMappingNotFound`), not a guess.

## Rationale

- **Correctness over convenience:** a wrong guess about the decimal separator silently turns €1.234,56 into €1.23456. A reviewed mapping can't do that.
- **Rare event:** a personal setup adds a new bank every few months. Writing ten lines of YAML is cheaper than building and testing detection.
- **Seam kept:** runtime detection can come later as another `BankMappingProvider` implementation, without touching the parser.

## Consequences

- Uploading from a new bank fails until its mapping is added.
- Mappings are validated at load time (single-character delimiter, `.` or `,` decimal separator), so a bad entry fails at startup, not mid-upload.
- Runtime detection (Claude reads the first N rows and persists a new mapping) stays on the backlog.
