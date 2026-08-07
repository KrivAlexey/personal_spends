package categorizer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type ClaudeTransaction struct {
	Index       int       `json:"index"`
	Date        time.Time `json:"date"`
	Merchant    string    `json:"merchant"`
	Amount      float64   `json:"amount"`
	Currency    string    `json:"currency"`
	Description string    `json:"description"`
}

type Claude struct {
	client     anthropic.Client
	categories []string
}

func NewClaude(apiKey string, categoriies []string) *Claude {
	claude := &Claude{
		client:     anthropic.NewClient(option.WithAPIKey(apiKey)),
		categories: categoriies,
	}

	return claude
}

func marshalBatch(batch []Transaction) (string, error) {
	claudeTransactions := make([]ClaudeTransaction, len(batch))

	for i, transaction := range batch {
		claudeTransactions[i] = ClaudeTransaction{
			Index:       i,
			Date:        transaction.Date,
			Merchant:    transaction.Merchant,
			Amount:      transaction.Amount,
			Currency:    transaction.Currency,
			Description: transaction.Description,
		}
	}

	data, err := json.Marshal(claudeTransactions)
	if err != nil {
		return "", fmt.Errorf("categorizer: marshal transactions: %w", err)
	}

	return string(data), nil
}

func (claude *Claude) Categorize(
	ctx context.Context,
	transactions []Transaction) ([]CategorizedTransaction, error) {

	txJSON, err := marshalBatch(transactions)
	if err != nil {
		return nil, err
	}

	mesagePrams := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(
			"Categorize each transaction below. Use the categorize_batch tool.\n\n" + txJSON,
		)),
	}

	toolParam, err := categorizeBatchTool(claude.categories)
	if err != nil {
		return nil, err
	}

	tools := make([]anthropic.ToolUnionParam, 1)
	tools[0] = anthropic.ToolUnionParam{OfTool: &toolParam}

	message, err := claude.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeHaiku4_5_20251001,
		MaxTokens: 1024,
		Messages:  mesagePrams,
		Tools:     tools,
	})
	if err != nil {
		return nil, fmt.Errorf("categorizer: create message: %w", err)
	}

	return unpackCategorizeBatch(message, transactions)
}

func unpackCategorizeBatch(message *anthropic.Message, transactions []Transaction) ([]CategorizedTransaction, error) {
	for _, block := range message.Content {
		toolUse, ok := block.AsAny().(anthropic.ToolUseBlock)
		if !ok || toolUse.Name != "categorize_batch" {
			continue
		}

		var result struct {
			Results []struct {
				Index      int     `json:"index"`
				Category   string  `json:"category"`
				Confidence float64 `json:"confidence"`
			} `json:"results"`
		}
		if err := json.Unmarshal([]byte(toolUse.JSON.Input.Raw()), &result); err != nil {
			return nil, fmt.Errorf("categorizer: unmarshal tool input: %w", err)
		}

		categorized := make([]CategorizedTransaction, len(result.Results))
		for i, r := range result.Results {
			categorized[i] = CategorizedTransaction{
				Transaction: transactions[r.Index],
				Category:    r.Category,
				Confidence:  r.Confidence,
			}
		}
		return categorized, nil
	}

	return nil, fmt.Errorf("categorizer: no categorize_batch tool call in response")
}

func categorizeBatchTool(categoryNames []string) (anthropic.ToolParam, error) {
	namesJSON, err := json.Marshal(categoryNames)
	if err != nil {
		return anthropic.ToolParam{}, fmt.Errorf("categorizer: marshal category names: %w", err)
	}

	schemaJSON := `{
		"properties": {
			"results": {
				"type": "array",
				"description": "One result per transaction, same order and length as the input batch.",
				"items": {
					"type": "object",
					"properties": {
						"index": {"type": "integer", "description": "Zero-based index of the transaction in the input batch."},
						"category": {"type": "string", "enum": ` + string(namesJSON) + `, "description": "Best-matching category name."},
						"confidence": {"type": "number", "description": "Confidence from 0.0 to 1.0."}
					},
					"required": ["index", "category", "confidence"]
				}
			}
		},
		"required": ["results"]
	}`

	var schema struct {
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal([]byte(schemaJSON), &schema); err != nil {
		return anthropic.ToolParam{}, fmt.Errorf("categorizer: parse tool schema: %w", err)
	}

	return anthropic.ToolParam{
		Name:        "categorize_batch",
		Description: anthropic.String("Assign a category and confidence score to each transaction in the batch, in the same order as the input."),
		InputSchema: anthropic.ToolInputSchemaParam{Properties: schema.Properties},
	}, nil
}
