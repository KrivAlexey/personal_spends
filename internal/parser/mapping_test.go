package parser

import (
	"context"
	"errors"
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
	if got.Mappings["Betrag"] != "Amount" {
		t.Errorf("sparkasse Betrag maps to %q, want Amount", got.Mappings["Betrag"])
	}

	if _, err := provider.GetMapping(context.Background(), "unknown"); !errors.Is(err, ErrBankMappingNotFound) {
		t.Errorf("GetMapping(unknown) error = %v, want ErrBankMappingNotFound", err)
	}
}
