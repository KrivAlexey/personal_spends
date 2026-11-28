package categorizer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/KrivAlexey/personal_spends/internal/categories"
	"log/slog"
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
	client anthropic.Client
	tool   anthropic.ToolParam
	logger *slog.Logger
}

var _ Categorizer = (*Claude)(nil)

// NewClaude returns a Claude categorizer that logs to logger (nil discards).
// opts are applied after the API key, e.g. option.WithBaseURL in tests.
func NewClaude(apiKey string, cats []categories.Category, logger *slog.Logger, opts ...option.RequestOption) *Claude {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	names := make([]string, len(cats))
	for i, c := range cats {
		names[i] = c.Name
	}
	return &Claude{
		client: anthropic.NewClient(append([]option.RequestOption{option.WithAPIKey(apiKey)}, opts...)...),
		tool:   categorizeBatchTool(names),
		logger: logger,
	}
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

	tools := []anthropic.ToolUnionParam{{OfTool: &claude.tool}}

	start := time.Now()
	message, err := claude.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model: anthropic.ModelClaudeHaiku4_5_20251001,
		// A full 50-transaction batch needs ~1,500 output tokens (#65).
		MaxTokens:  4096,
		Messages:   mesagePrams,
		Tools:      tools,
		ToolChoice: anthropic.ToolChoiceParamOfTool("categorize_batch"),
	})
	if err != nil {
		return nil, fmt.Errorf("categorizer: create message: %w", safeAPIError(err))
	}
	// Logged before validation, so a max_tokens answer that fails it still
	// shows its stop_reason (#65).
	claude.logger.DebugContext(ctx, "categorize batch",
		"size", len(transactions),
		"duration_ms", time.Since(start).Milliseconds(),
		"input_tokens", message.Usage.InputTokens,
		"output_tokens", message.Usage.OutputTokens,
		"stop_reason", string(message.StopReason))

	return unpackCategorizeBatch(message, transactions)
}

// safeAPIError replaces an API error, whose text carries the raw response body
// that could echo the prompt, with its status, error type and request ID
// (ADR 0013). Other errors, e.g. context cancellation, pass through.
func safeAPIError(err error) error {
	apiErr, ok := errors.AsType[*anthropic.Error](err)
	if !ok {
		return err
	}
	return fmt.Errorf("status %d, type %s, request-id %s", apiErr.StatusCode, apiErr.Type(), apiErr.RequestID)
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

		categorized := make([]CategorizedTransaction, len(transactions))
		seen := make([]bool, len(transactions))
		for _, r := range result.Results {
			if r.Index < 0 || r.Index >= len(transactions) {
				return nil, fmt.Errorf("categorizer: result index %d out of range for batch of %d transactions", r.Index, len(transactions))
			}
			if seen[r.Index] {
				return nil, fmt.Errorf("categorizer: duplicate result for transaction index %d", r.Index)
			}
			seen[r.Index] = true
			categorized[r.Index] = CategorizedTransaction{
				Transaction: transactions[r.Index],
				Category:    r.Category,
				Confidence:  r.Confidence,
			}
		}
		for i, ok := range seen {
			if !ok {
				return nil, fmt.Errorf("categorizer: missing result for transaction index %d", i)
			}
		}
		return categorized, nil
	}

	return nil, fmt.Errorf("categorizer: no categorize_batch tool call in response")
}

func categorizeBatchTool(categoryNames []string) anthropic.ToolParam {
	return anthropic.ToolParam{
		Name:        "categorize_batch",
		Description: anthropic.String("Assign a category and confidence score to each transaction in the batch, in the same order as the input."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"results": map[string]any{
					"type":        "array",
					"description": "One result per transaction, same order and length as the input batch.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"index":      map[string]any{"type": "integer", "description": "Zero-based index of the transaction in the input batch."},
							"category":   map[string]any{"type": "string", "enum": categoryNames, "description": "Best-matching category name."},
							"confidence": map[string]any{"type": "number", "description": "Confidence from 0.0 to 1.0."},
						},
						"required": []string{"index", "category", "confidence"},
					},
				},
			},
			Required: []string{"results"},
		},
	}
}
