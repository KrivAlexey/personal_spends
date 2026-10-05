package categorizer

import (
	"context"
	"time"
)

type Categorizer interface {
	Categorize(ctx context.Context, batch []Transaction) ([]CategorizedTransaction, error)
}

type Transaction struct {
	// ID identifies the transaction across exports (ADR 0011); set by the parser.
	ID          string
	Date        time.Time
	Merchant    string
	Amount      float64
	Currency    string
	Description string
	Source      string
}

type CategorizedTransaction struct {
	Transaction
	Category   string
	Confidence float64
}
