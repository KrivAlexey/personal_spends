package parser

import (
	"context"
	"maps"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Header of a Sparkasse CSV-CAMT export. All values in this file are made up.
var sparkasseHeader = []string{
	"Auftragskonto", "Buchungstag", "Valutadatum", "Buchungstext", "Verwendungszweck",
	"Glaeubiger ID", "Mandatsreferenz", "Kundenreferenz (End-to-End)", "Sammlerreferenz",
	"Lastschrift Ursprungsbetrag", "Auslagenersatz Ruecklastschrift",
	"Beguenstigter/Zahlungspflichtiger", "Kontonummer/IBAN", "BIC (SWIFT-Code)",
	"Betrag", "Waehrung", "Info", "Kategorie",
}

func bookedRow() map[string]string {
	return map[string]string{
		"Auftragskonto":                     "DE00123456780000000001",
		"Buchungstag":                       "14.08.26",
		"Valutadatum":                       "14.08.26",
		"Buchungstext":                      "KARTENZAHLUNG",
		"Verwendungszweck":                  "2026-08-13T19:25 Debitk.1 2099-12 ",
		"Mandatsreferenz":                   "111111",
		"Sammlerreferenz":                   "00000000000001130826192533",
		"Beguenstigter/Zahlungspflichtiger": "TEST DROGERIE//HAMBURG/DE",
		"Kontonummer/IBAN":                  "DE00999999990000000002",
		"BIC (SWIFT-Code)":                  "TESTDEFFXXX",
		"Betrag":                            "-8,99",
		"Waehrung":                          "EUR",
		"Info":                              "Umsatz gebucht",
		"Kategorie":                         "Lebensmittel",
	}
}

func pendingRow() map[string]string {
	return with(bookedRow(), map[string]string{
		"Valutadatum":                       "",
		"Buchungstext":                      "SONSTIGER EINZUG",
		"Verwendungszweck":                  "MO 00000001 140826143201C12 ",
		"Beguenstigter/Zahlungspflichtiger": "F000000000001",
		"Info":                              "Umsatz vorgemerkt",
		"Kategorie":                         "",
	})
}

func with(row map[string]string, overrides map[string]string) map[string]string {
	maps.Copy(row, overrides)
	return row
}

// csvOf renders rows under header as a semicolon-separated, fully quoted CSV.
func csvOf(header []string, rows ...map[string]string) string {
	quote := func(cells []string) string {
		for i, c := range cells {
			cells[i] = `"` + c + `"`
		}
		return strings.Join(cells, ";")
	}
	lines := []string{quote(append([]string(nil), header...))}
	for _, row := range rows {
		cells := make([]string, len(header))
		for i, col := range header {
			cells[i] = row[col]
		}
		lines = append(lines, quote(cells))
	}
	return strings.Join(lines, "\n") + "\n"
}

type staticProvider map[string]BankMapping

func (p staticProvider) GetMapping(_ context.Context, bank string) (BankMapping, error) {
	m, ok := p[bank]
	if !ok {
		return BankMapping{}, ErrBankMappingNotFound
	}
	return m, nil
}

func committedSparkasse(t *testing.T) BankMapping {
	t.Helper()
	provider, err := NewFileMappingProvider(context.Background(), "bank_mappings.yaml")
	if err != nil {
		t.Fatalf("NewFileMappingProvider: %v", err)
	}
	m, err := provider.GetMapping(context.Background(), "sparkasse")
	if err != nil {
		t.Fatalf("GetMapping: %v", err)
	}
	return m
}

func parse(t *testing.T, mapping BankMapping, csv string) (ParseResult, error) {
	t.Helper()
	p := NewParser(staticProvider{mapping.BankName: mapping})
	return p.ParseCSV(context.Background(), mapping.BankName, strings.NewReader(csv))
}

func mustParse(t *testing.T, mapping BankMapping, csv string) ParseResult {
	t.Helper()
	res, err := parse(t, mapping, csv)
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	return res
}

func onlyID(t *testing.T, mapping BankMapping, row map[string]string) string {
	t.Helper()
	res := mustParse(t, mapping, csvOf(sparkasseHeader, row))
	if len(res.Transactions) != 1 {
		t.Fatalf("got %d transactions, want 1", len(res.Transactions))
	}
	return res.Transactions[0].ID
}

var idFormat = regexp.MustCompile(`^[0-9a-f]{16}-\d+$`)

func TestParseCSV_PendingRows(t *testing.T) {
	sparkasse := committedSparkasse(t)
	noPending := sparkasse
	noPending.BankName, noPending.PendingColumn, noPending.PendingValue = "nopending", "", ""

	tests := []struct {
		name        string
		mapping     BankMapping
		rows        []map[string]string
		wantTx      int
		wantSkipped int
	}{
		{name: "pending rows skipped and counted", mapping: sparkasse, rows: []map[string]string{pendingRow(), bookedRow(), pendingRow()}, wantTx: 1, wantSkipped: 2},
		{name: "only booked rows", mapping: sparkasse, rows: []map[string]string{bookedRow()}, wantTx: 1, wantSkipped: 0},
		{name: "only pending rows", mapping: sparkasse, rows: []map[string]string{pendingRow()}, wantTx: 0, wantSkipped: 1},
		{name: "bank without pending marker imports all rows", mapping: noPending, rows: []map[string]string{pendingRow(), bookedRow()}, wantTx: 2, wantSkipped: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := mustParse(t, tt.mapping, csvOf(sparkasseHeader, tt.rows...))
			if len(res.Transactions) != tt.wantTx || res.PendingSkipped != tt.wantSkipped {
				t.Errorf("got %d transactions, %d pending skipped; want %d, %d",
					len(res.Transactions), res.PendingSkipped, tt.wantTx, tt.wantSkipped)
			}
		})
	}
}

func TestParseCSV_BookedRowFields(t *testing.T) {
	res := mustParse(t, committedSparkasse(t), csvOf(sparkasseHeader, bookedRow()))
	if len(res.Transactions) != 1 {
		t.Fatalf("got %d transactions, want 1", len(res.Transactions))
	}
	tx := res.Transactions[0]

	if want := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC); !tx.Date.Equal(want) {
		t.Errorf("Date = %v, want booking date %v (not the purchase time in the description)", tx.Date, want)
	}
	if tx.Amount != -8.99 || tx.Merchant != "TEST DROGERIE//HAMBURG/DE" || tx.Source != "sparkasse" {
		t.Errorf("got amount %v, merchant %q, source %q", tx.Amount, tx.Merchant, tx.Source)
	}
	if !idFormat.MatchString(tx.ID) || !strings.HasSuffix(tx.ID, "-0") {
		t.Errorf("ID = %q, want 16 hex chars + \"-0\"", tx.ID)
	}
}

func TestParseCSV_Identity(t *testing.T) {
	sparkasse := committedSparkasse(t)

	tests := []struct {
		name     string
		other    map[string]string // overrides applied to a second copy of bookedRow
		wantSame bool
	}{
		{name: "identical row", other: map[string]string{}, wantSame: true},
		{name: "only Kategorie differs", other: map[string]string{"Kategorie": "Drogerie"}, wantSame: true},
		{name: "only Info differs (non-pending value)", other: map[string]string{"Info": ""}, wantSame: true},
		{name: "only surrounding whitespace differs", other: map[string]string{"Verwendungszweck": "  2026-08-13T19:25 Debitk.1 2099-12", "Betrag": " -8,99 "}, wantSame: true},
		{name: "different account", other: map[string]string{"Auftragskonto": "DE00123456780000000009"}, wantSame: false},
		{name: "different amount", other: map[string]string{"Betrag": "-9,99"}, wantSame: false},
		{name: "different card reference", other: map[string]string{"Sammlerreferenz": "00000000000001130826192534"}, wantSame: false},
		{name: "cells shifted across the separator", other: map[string]string{"Buchungstext": "KARTENZAHLUNG2026-08-13T19:25", "Verwendungszweck": " Debitk.1 2099-12"}, wantSame: false},
	}

	base := onlyID(t, sparkasse, bookedRow())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := onlyID(t, sparkasse, with(bookedRow(), tt.other))
			if (got == base) != tt.wantSame {
				t.Errorf("ID %q vs base %q: same = %v, want %v", got, base, got == base, tt.wantSame)
			}
		})
	}
}

func TestParseCSV_IdenticalRowsInOneFile(t *testing.T) {
	sparkasse := committedSparkasse(t)
	csv := csvOf(sparkasseHeader, bookedRow(), bookedRow(), with(bookedRow(), map[string]string{"Betrag": "-1,00"}), bookedRow())

	first := mustParse(t, sparkasse, csv)
	second := mustParse(t, sparkasse, csv)

	ids := make([]string, len(first.Transactions))
	for i, tx := range first.Transactions {
		ids[i] = tx.ID
	}
	if len(ids) != 4 {
		t.Fatalf("got %d transactions, want 4", len(ids))
	}
	base := strings.TrimSuffix(ids[0], "-0")
	if want := []string{base + "-0", base + "-1", "", base + "-2"}; ids[0] != want[0] || ids[1] != want[1] || ids[3] != want[3] {
		t.Errorf("identical rows got IDs %v, want %s-0, -1, -2 (counted over the whole file)", ids, base)
	}
	if strings.HasPrefix(ids[2], base) || !strings.HasSuffix(ids[2], "-0") {
		t.Errorf("different row got ID %q, want its own hash with -0", ids[2])
	}
	for i, tx := range second.Transactions {
		if tx.ID != ids[i] {
			t.Errorf("re-parse row %d: ID %q, want %q (deterministic)", i, tx.ID, ids[i])
		}
	}
}

func TestParseCSV_MissingColumns(t *testing.T) {
	sparkasse := committedSparkasse(t)
	without := func(col string) []string {
		var h []string
		for _, c := range sparkasseHeader {
			if c != col {
				h = append(h, c)
			}
		}
		return h
	}

	tests := []struct {
		name   string
		header []string
		errHas string
	}{
		{name: "identity column missing", header: without("Sammlerreferenz"), errHas: "Sammlerreferenz"},
		{name: "pending column missing", header: without("Info"), errHas: "Info"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := parse(t, sparkasse, csvOf(tt.header, bookedRow()))
			if err == nil || !strings.Contains(err.Error(), tt.errHas) {
				t.Fatalf("error = %v, want one naming %q", err, tt.errHas)
			}
			if len(res.Transactions) != 0 {
				t.Errorf("got %d transactions, want none", len(res.Transactions))
			}
		})
	}
}
