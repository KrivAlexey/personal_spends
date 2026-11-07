package parser

import "testing"

func TestBankMapping_ApplyDefaults(t *testing.T) {
	valid := func() BankMapping {
		return BankMapping{
			BankName:        "testbank",
			DateFormat:      "02.01.06",
			Delimiter:       ";",
			PendingColumn:   "Info",
			PendingValue:    "Umsatz vorgemerkt",
			IdentityColumns: []string{"Account", "Date", "Amount"},
			Mappings:        map[string]string{"Date": "Date", "Amount": "Amount"},
		}
	}

	tests := []struct {
		name    string
		mutate  func(*BankMapping)
		wantErr bool
	}{
		{name: "valid with pending marker", mutate: func(*BankMapping) {}},
		{name: "valid without pending marker", mutate: func(m *BankMapping) { m.PendingColumn, m.PendingValue = "", "" }},
		{name: "no identity columns", mutate: func(m *BankMapping) { m.IdentityColumns = nil }, wantErr: true},
		{name: "duplicate identity column", mutate: func(m *BankMapping) { m.IdentityColumns = []string{"Date", "Amount", "Date"} }, wantErr: true},
		{name: "empty identity column name", mutate: func(m *BankMapping) { m.IdentityColumns = []string{"Date", ""} }, wantErr: true},
		{name: "pending column without value", mutate: func(m *BankMapping) { m.PendingValue = "" }, wantErr: true},
		{name: "pending value without column", mutate: func(m *BankMapping) { m.PendingColumn = "" }, wantErr: true},
		{name: "utf-8 encoding", mutate: func(m *BankMapping) { m.Encoding = "utf-8" }},
		{name: "iso-8859-1 encoding", mutate: func(m *BankMapping) { m.Encoding = "iso-8859-1" }},
		{name: "unsupported encoding", mutate: func(m *BankMapping) { m.Encoding = "windows-1251" }, wantErr: true},
		{name: "all five fields mapped", mutate: func(m *BankMapping) {
			m.Mappings = map[string]string{"D": "Date", "M": "Merchant", "A": "Amount", "C": "Currency", "T": "Description"}
		}},
		{name: "lowercase field name", mutate: func(m *BankMapping) { m.Mappings["Datum"] = "date" }, wantErr: true},
		{name: "unknown field name", mutate: func(m *BankMapping) { m.Mappings["Kategorie"] = "Category" }, wantErr: true},
		{name: "empty field name", mutate: func(m *BankMapping) { m.Mappings["Info"] = "" }, wantErr: true},
		{name: "no mappings", mutate: func(m *BankMapping) { m.Mappings = nil }, wantErr: true},
		{name: "no Date mapping", mutate: func(m *BankMapping) { delete(m.Mappings, "Date") }, wantErr: true},
		{name: "no Amount mapping", mutate: func(m *BankMapping) { delete(m.Mappings, "Amount") }, wantErr: true},
		{name: "field mapped twice", mutate: func(m *BankMapping) { m.Mappings["Valuta"] = "Date" }, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := valid()
			tt.mutate(&m)
			err := m.applyDefaults()
			if (err != nil) != tt.wantErr {
				t.Fatalf("applyDefaults() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && m.Encoding == "" {
				t.Errorf("Encoding left empty, want a default")
			}
		})
	}
}
