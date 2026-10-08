## 1. Encoding

- [x] 1.1 Failing tests: mapping validation of `encoding`, committed Sparkasse mapping declares `iso-8859-1`, ParseCSV decodes ISO-8859-1 text
- [x] 1.2 `BankMapping.Encoding` with `utf-8` default and validation in `applyDefaults`
- [x] 1.3 Decode ISO-8859-1 input in `ParseCSV` before the CSV reader
- [x] 1.4 Set `encoding: iso-8859-1` for Sparkasse in `bank_mappings.yaml`
