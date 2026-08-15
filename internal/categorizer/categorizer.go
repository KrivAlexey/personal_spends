package categorizer

import (
	"context"
	"time"
)

type Categorizer interface {
	Categorize(ctx context.Context, batch []Transaction) ([]CategorizedTransaction, error)
	ExtractFromImage(ctx context.Context, imageURL string) ([]Transaction, error)
}

type Transaction struct {
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
