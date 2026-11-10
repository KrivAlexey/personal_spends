package categorizer

import (
	"context"
	"log/slog"
)

// KeywordRule assigns Category to a transaction whose merchant or description
// contains Keyword.
type KeywordRule struct {
	Keyword  string
	Category string
}

// Keyword is a Categorizer that calls no model.
type Keyword struct {
	rules  []KeywordRule
	logger *slog.Logger
}

var _ Categorizer = (*Keyword)(nil)

// NewKeyword returns a Keyword categorizer.
func NewKeyword(rules []KeywordRule, logger *slog.Logger) *Keyword {
	return &Keyword{rules: rules, logger: logger}
}

// Categorize is not implemented yet.
func (k *Keyword) Categorize(ctx context.Context, batch []Transaction) ([]CategorizedTransaction, error) {
	return nil, nil
}
