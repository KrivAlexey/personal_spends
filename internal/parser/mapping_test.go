package parser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileMappingProvider_CommittedMappings(t *testing.T) {
	provider, err := NewFileMappingProvider(context.Background(), "bank_mappings.yaml")
	if err != nil {
		t.Fatalf("NewFileMappingProvider: %v", err)
	}

	got, err := provider.GetMapping(context.Background(), "sparkasse")
	if err != nil {
		t.Fatalf("GetMapping(sparkasse): %v", err)
	}
	if got.Comma() != ';' || got.DecimalSeparator != "," || got.DateFormat != "02.01.06" {
		t.Errorf("sparkasse = delimiter %q, decimal %q, date %q; want ';', ',', 02.01.06",
			got.Delimiter, got.DecimalSeparator, got.DateFormat)
	}
	if got.Encoding != "iso-8859-1" {
		t.Errorf("sparkasse encoding = %q, want iso-8859-1", got.Encoding)
	}
	if got.Mappings["Betrag"] != "Amount" {
		t.Errorf("sparkasse Betrag maps to %q, want Amount", got.Mappings["Betrag"])
	}
	if got.PendingColumn != "Info" || got.PendingValue != "Umsatz vorgemerkt" {
		t.Errorf("sparkasse pending = %q/%q, want Info/Umsatz vorgemerkt", got.PendingColumn, got.PendingValue)
	}
	identity := make(map[string]bool, len(got.IdentityColumns))
	for _, c := range got.IdentityColumns {
		identity[c] = true
	}
	for _, c := range []string{"Auftragskonto", "Buchungstag", "Sammlerreferenz", "Betrag"} {
		if !identity[c] {
			t.Errorf("sparkasse identityColumns lacks %q", c)
		}
	}
	for _, c := range []string{"Info", "Kategorie"} {
		if identity[c] {
			t.Errorf("sparkasse identityColumns must not include %q (changes between exports)", c)
		}
	}

	if _, err := provider.GetMapping(context.Background(), "unknown"); !errors.Is(err, ErrBankMappingNotFound) {
		t.Errorf("GetMapping(unknown) error = %v, want ErrBankMappingNotFound", err)
	}
}

func TestNewFileMappingProvider_Invalid(t *testing.T) {
	const bank = `
  - bankName: %s
    dateFormat: "02.01.06"
    identityColumns: [Datum, Betrag]
    mappings:
      Datum: Date
      Betrag: %s
`
	tests := []struct {
		name     string
		yaml     string
		wantErr  bool
		wantBank string
	}{
		{name: "valid", yaml: "allMappings:" + fmt.Sprintf(bank, "a", "Amount")},
		{name: "two banks", yaml: "allMappings:" + fmt.Sprintf(bank, "a", "Amount") + fmt.Sprintf(bank, "b", "Amount")},
		{name: "duplicate bank name", yaml: "allMappings:" + fmt.Sprintf(bank, "a", "Amount") + fmt.Sprintf(bank, "a", "Amount"), wantErr: true, wantBank: "a"},
		{name: "misspelled field", yaml: "allMappings:" + fmt.Sprintf(bank, "a", "amount"), wantErr: true, wantBank: "a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "mappings.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := NewFileMappingProvider(context.Background(), path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewFileMappingProvider() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "bank "+tt.wantBank) {
				t.Errorf("error %q does not name bank %q", err, tt.wantBank)
			}
		})
	}
}

func TestFileMappingProvider_Banks(t *testing.T) {
	committed, err := os.ReadFile("bank_mappings.yaml")
	if err != nil {
		t.Fatalf("read committed mappings: %v", err)
	}
	// Two copies of the committed Sparkasse entry under other names.
	_, entry, ok := strings.Cut(string(committed), "allMappings:\n")
	if !ok {
		t.Fatal("bank_mappings.yaml has no allMappings: line")
	}
	twoBanks := "allMappings:\n" +
		strings.Replace(entry, "bankName: sparkasse", "bankName: zbank", 1) +
		strings.Replace(entry, "bankName: sparkasse", "bankName: abank", 1)
	path := filepath.Join(t.TempDir(), "two.yaml")
	if err := os.WriteFile(path, []byte(twoBanks), 0o600); err != nil {
		t.Fatal(err)
	}

	empty := filepath.Join(t.TempDir(), "empty.yaml")
	if err := os.WriteFile(empty, []byte("allMappings: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		file string
		want []string
	}{
		{name: "committed mappings", file: "bank_mappings.yaml", want: []string{"sparkasse"}},
		{name: "sorted", file: path, want: []string{"abank", "zbank"}},
		{name: "no mappings is empty, not nil", file: empty, want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewFileMappingProvider(context.Background(), tt.file)
			if err != nil {
				t.Fatalf("NewFileMappingProvider: %v", err)
			}
			got := provider.Banks()
			if got == nil || fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Fatalf("Banks() = %#v, want %#v", got, tt.want)
			}
			if len(got) == 0 {
				return
			}
			got[0] = "changed"
			if again := provider.Banks(); fmt.Sprint(again) != fmt.Sprint(tt.want) {
				t.Errorf("Banks() after caller changed the result = %v, want %v", again, tt.want)
			}
		})
	}
}
