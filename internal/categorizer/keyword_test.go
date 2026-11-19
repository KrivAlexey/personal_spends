package categorizer_test

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"testing"

	"github.com/KrivAlexey/personal_spends/internal/categorizer"
	"github.com/KrivAlexey/personal_spends/internal/categorizer/categorizertest"
)

var keywordRules = []categorizer.KeywordRule{
	{Keyword: "rewe", Category: "groceries"},
	{Keyword: "netflix", Category: "streaming_subscriptions"},
	{Keyword: "tankstelle", Category: "fuel"},
	{Keyword: "markt", Category: "home_goods"}, // also matches "REWE Markt": first rule must win
}

func TestKeywordCategorize(t *testing.T) {
	tests := []struct {
		name  string
		batch []categorizer.Transaction
		want  []categorizer.CategorizedTransaction
	}{
		{
			name:  "empty batch",
			batch: nil,
			want:  []categorizer.CategorizedTransaction{},
		},
		{
			name:  "merchant match is case-insensitive, first matching rule wins",
			batch: []categorizer.Transaction{{ID: "a", Merchant: "REWE Markt GmbH"}},
			want: []categorizer.CategorizedTransaction{
				{Transaction: categorizer.Transaction{ID: "a", Merchant: "REWE Markt GmbH"}, Category: "groceries", Confidence: 1},
			},
		},
		{
			name:  "description matches when merchant does not",
			batch: []categorizer.Transaction{{ID: "b", Merchant: "Shell", Description: "SHELL TANKSTELLE"}},
			want: []categorizer.CategorizedTransaction{
				{Transaction: categorizer.Transaction{ID: "b", Merchant: "Shell", Description: "SHELL TANKSTELLE"}, Category: "fuel", Confidence: 1},
			},
		},
		{
			name: "no match falls back to uncategorized with zero confidence, input order kept",
			batch: []categorizer.Transaction{
				{ID: "c", Merchant: "Unknown Shop"},
				{ID: "d", Merchant: "Netflix"},
			},
			want: []categorizer.CategorizedTransaction{
				{Transaction: categorizer.Transaction{ID: "c", Merchant: "Unknown Shop"}, Category: "uncategorized", Confidence: 0},
				{Transaction: categorizer.Transaction{ID: "d", Merchant: "Netflix"}, Category: "streaming_subscriptions", Confidence: 1},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := categorizer.NewKeyword(keywordRules, nil).Categorize(context.Background(), tt.batch)
			if err != nil {
				t.Fatalf("Categorize: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestKeywordCategorizeCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := categorizer.NewKeyword(keywordRules, nil).Categorize(ctx, batch)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if got != nil {
		t.Errorf("got %+v, want nil", got)
	}
}

func TestKeywordBatchLogging(t *testing.T) {
	categorizertest.AssertBatchLogging(t, func(l *slog.Logger) categorizer.Categorizer {
		return categorizer.NewKeyword(keywordRules, l)
	}, batch)
}
