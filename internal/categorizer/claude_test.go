package categorizer

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
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

// toolUseMessage builds a fake API response message carrying a single
// categorize_batch tool-use block, so unpackCategorizeBatch can be tested
// without calling the real API.
func toolUseMessage(t *testing.T, toolInput string) *anthropic.Message {
	t.Helper()

	raw := fmt.Sprintf(`{
		"id": "msg_test",
		"type": "message",
		"role": "assistant",
		"model": "claude-haiku-4-5-20251001",
		"content": [
			{
				"type": "tool_use",
				"id": "toolu_test",
				"name": "categorize_batch",
				"input": %s
			}
		],
		"stop_reason": "tool_use"
	}`, toolInput)

	var message anthropic.Message
	if err := json.Unmarshal([]byte(raw), &message); err != nil {
		t.Fatalf("unmarshal fake message: %v", err)
	}
	return &message
}

func TestUnpackCategorizeBatch(t *testing.T) {
	tests := []struct {
		name      string
		toolInput string
		wantErr   bool
	}{
		{
			name: "valid full coverage",
			toolInput: `{"results": [
				{"index": 0, "category": "groceries", "confidence": 0.9},
				{"index": 1, "category": "streaming_subscriptions", "confidence": 0.95},
				{"index": 2, "category": "fuel", "confidence": 0.8},
				{"index": 3, "category": "transportation_public", "confidence": 0.99},
				{"index": 4, "category": "coffee_shops", "confidence": 0.85}
			]}`,
			wantErr: false,
		},
		{
			name: "out of range index",
			toolInput: `{"results": [
				{"index": 5, "category": "groceries", "confidence": 0.9}
			]}`,
			wantErr: true,
		},
		{
			name: "negative index",
			toolInput: `{"results": [
				{"index": -1, "category": "groceries", "confidence": 0.9}
			]}`,
			wantErr: true,
		},
		{
			name: "duplicate index",
			toolInput: `{"results": [
				{"index": 0, "category": "groceries", "confidence": 0.9},
				{"index": 0, "category": "restaurants", "confidence": 0.5}
			]}`,
			wantErr: true,
		},
		{
			name: "missing index",
			toolInput: `{"results": [
				{"index": 0, "category": "groceries", "confidence": 0.9}
			]}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batch := testTransactions[:5]
			message := toolUseMessage(t, tt.toolInput)

			results, err := unpackCategorizeBatch(message, batch)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got none")
				}
				if results != nil {
					t.Fatalf("expected nil results on error, got %v", results)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(results) != len(batch) {
				t.Fatalf("expected %d results, got %d", len(batch), len(results))
			}
			for i, r := range results {
				if r.Transaction != batch[i] {
					t.Errorf("result %d: expected transaction %+v, got %+v", i, batch[i], r.Transaction)
				}
			}
		})
	}
}
