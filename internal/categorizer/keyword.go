package categorizer

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

// fallbackCategory is assigned when no rule matches; it exists in
// categories.yaml.
const fallbackCategory = "uncategorized"

// KeywordRule assigns Category to a transaction whose merchant or description
// contains Keyword, ignoring case.
type KeywordRule struct {
	Keyword  string
	Category string
}

// Keyword is a Categorizer that calls no model: the first matching rule wins
// with confidence 1, otherwise "uncategorized" with confidence 0. It lets the
// server run offline for local development.
type Keyword struct {
	rules  []KeywordRule
	logger *slog.Logger
}

var _ Categorizer = (*Keyword)(nil)

// NewKeyword returns a Keyword categorizer that logs to logger (nil discards).
func NewKeyword(rules []KeywordRule, logger *slog.Logger) *Keyword {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Keyword{rules: rules, logger: logger}
}

// Categorize returns one CategorizedTransaction per transaction, in input
// order.
func (k *Keyword) Categorize(ctx context.Context, batch []Transaction) ([]CategorizedTransaction, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	start := time.Now()
	out := make([]CategorizedTransaction, len(batch))
	for i, tx := range batch {
		out[i] = CategorizedTransaction{Transaction: tx, Category: fallbackCategory}
		text := strings.ToLower(tx.Merchant + " " + tx.Description)
		for _, r := range k.rules {
			if strings.Contains(text, strings.ToLower(r.Keyword)) {
				out[i].Category, out[i].Confidence = r.Category, 1
				break
			}
		}
	}
	// Same record as Claude's; no model, so zero tokens and stop_reason "none".
	k.logger.DebugContext(ctx, "categorize batch",
		"size", len(batch),
		"duration_ms", time.Since(start).Milliseconds(),
		"input_tokens", 0,
		"output_tokens", 0,
		"stop_reason", "none")
	return out, nil
}
