package categorizer

import (
	"os"
	"testing"
	"time"
)

var testTransactions = []Transaction{
	{
		Date:        time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC),
		Merchant:    "REWE",
		Amount:      47.32,
		Currency:    "EUR",
		Description: "REWE SAGT DANKE 4210",
		Source:      "csv",
	},
	{
		Date:        time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC),
		Merchant:    "Netflix",
		Amount:      12.99,
		Currency:    "EUR",
		Description: "NETFLIX.COM",
		Source:      "csv",
	},
	{
		Date:        time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC),
		Merchant:    "Shell",
		Amount:      68.50,
		Currency:    "EUR",
		Description: "SHELL TANKSTELLE BERLIN",
		Source:      "csv",
	},
	{
		Date:        time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
		Merchant:    "BVG",
		Amount:      9.00,
		Currency:    "EUR",
		Description: "BVG EINZELTICKET",
		Source:      "csv",
	},
	{
		Date:        time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC),
		Merchant:    "Starbucks",
		Amount:      4.85,
		Currency:    "EUR",
		Description: "STARBUCKS ALEXANDERPLATZ",
		Source:      "csv",
	},
}

func TestClaudeCategorizer_CallClaude(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}

	categories := []string{
		"groceries", "restaurants", "coffee_shops", "transportation_public", "fuel",
		"housing_rent", "utilities", "internet_phone", "streaming_subscriptions", "shopping_clothing",
	}
	claude := NewClaude(apiKey, categories)

	results, err := claude.Categorize(t.Context(), testTransactions)
	_ = results
	_ = err
}
