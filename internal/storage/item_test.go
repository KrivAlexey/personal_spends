package storage

import (
	"testing"
	"time"
)

func TestNewExpenseItem(t *testing.T) {
	date := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 7, 31, 12, 30, 0, 0, time.UTC)

	expense := Expense{
		ID:             "abc123",
		Date:           date,
		Amount:         42.5,
		Currency:       "EUR",
		Merchant:       "Rewe",
		RawDescription: "REWE SAGT DANKE",
		Category:       "Groceries",
		Confidence:     0.95,
		Source:         "csv",
		CreatedAt:      createdAt,
	}

	item := newExpenseItem(expense)

	wantSK := "2026-07-31#abc123"
	if item.PK != expensesPK {
		t.Errorf("PK = %q, want %q", item.PK, expensesPK)
	}
	if item.SK != wantSK {
		t.Errorf("SK = %q, want %q", item.SK, wantSK)
	}
	if item.GSI1PK != "CAT#Groceries" {
		t.Errorf("GSI1PK = %q, want %q", item.GSI1PK, "CAT#Groceries")
	}
	if item.GSI1SK != wantSK {
		t.Errorf("GSI1SK = %q, want %q", item.GSI1SK, wantSK)
	}
	if item.CreatedAt != createdAt.Format(time.RFC3339) {
		t.Errorf("CreatedAt = %q, want %q", item.CreatedAt, createdAt.Format(time.RFC3339))
	}
	if item.Amount != expense.Amount || item.Currency != expense.Currency ||
		item.Merchant != expense.Merchant || item.RawDescription != expense.RawDescription ||
		item.Category != expense.Category || item.Confidence != expense.Confidence ||
		item.Source != expense.Source {
		t.Errorf("newExpenseItem() did not copy all attributes: got %+v, from %+v", item, expense)
	}
}
