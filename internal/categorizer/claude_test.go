package categorizer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/KrivAlexey/personal_spends/internal/categories"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
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

	var cats []categories.Category
	for _, name := range []string{
		"groceries", "restaurants", "coffee_shops", "transportation_public", "fuel",
		"housing_rent", "utilities", "internet_phone", "streaming_subscriptions", "shopping_clothing",
	} {
		cats = append(cats, categories.Category{Name: name})
	}
	claude := NewClaude(apiKey, cats, nil)

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

// TestClaudeImplementsCategorizer guards the wiring the handler depends on:
// NewClaude's return value has to be assignable to a Categorizer field.
func TestClaudeImplementsCategorizer(t *testing.T) {
	var c Categorizer = NewClaude("test-key", []categories.Category{{Name: "groceries"}}, nil)
	if c == nil {
		t.Fatal("NewClaude returned a nil Categorizer")
	}
}

// messagesRequest is the part of a POST /v1/messages body Categorize is
// responsible for.
type messagesRequest struct {
	MaxTokens  int `json:"max_tokens"`
	ToolChoice struct {
		Type string `json:"type"`
		Name string `json:"name"`
	} `json:"tool_choice"`
	Messages []struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	} `json:"messages"`
	Tools []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		InputSchema struct {
			Required   []string `json:"required"`
			Properties struct {
				Results struct {
					Items struct {
						Properties struct {
							Category struct {
								Enum []string `json:"enum"`
							} `json:"category"`
						} `json:"properties"`
					} `json:"items"`
				} `json:"results"`
			} `json:"properties"`
		} `json:"input_schema"`
	} `json:"tools"`
}

func TestClaudeCategorize(t *testing.T) {
	cats := []categories.Category{
		{Name: "groceries", Description: "supermarket and food shopping"},
		{Name: "streaming_subscriptions", Description: "video and music subscriptions"},
		{Name: "fuel"},
		{Name: "transportation_public", Description: "buses, trains and tickets"},
		{Name: "coffee_shops", Description: "small, frequent purchases"},
	}
	batch := testTransactions[:2]

	tests := []struct {
		name           string
		status         int
		response       string
		cancelCtx      bool
		wantErr        error  // checked with errors.Is when set
		wantErrMsg     string // substring of the error when set
		wantCategories []string
	}{
		{
			name:   "tool call is unpacked in input order",
			status: http.StatusOK,
			response: `{"id": "msg_test", "type": "message", "role": "assistant",
				"model": "claude-haiku-4-5-20251001", "stop_reason": "tool_use",
				"content": [{"type": "tool_use", "id": "toolu_test", "name": "categorize_batch",
					"input": {"results": [
						{"index": 1, "category": "streaming_subscriptions", "confidence": 0.95},
						{"index": 0, "category": "groceries", "confidence": 0.9}
					]}}]}`,
			wantCategories: []string{"groceries", "streaming_subscriptions"},
		},
		{
			name:       "API error is wrapped",
			status:     http.StatusInternalServerError,
			response:   `{"type": "error", "error": {"type": "api_error", "message": "boom"}}`,
			wantErrMsg: "categorizer: create message",
		},
		{
			name:   "response without tool call is an error",
			status: http.StatusOK,
			response: `{"id": "msg_test", "type": "message", "role": "assistant",
				"model": "claude-haiku-4-5-20251001", "stop_reason": "end_turn",
				"content": [{"type": "text", "text": "Sure, here are the categories."}]}`,
			wantErrMsg: "no categorize_batch tool call",
		},
		{
			name:      "cancelled context",
			cancelCtx: true,
			wantErr:   context.Canceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/v1/messages" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read request body: %v", err)
				}
				assertCategorizeRequest(t, body, cats, batch)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.response))
			}))
			defer srv.Close()

			claude := NewClaude("test-key", cats, nil, option.WithBaseURL(srv.URL), option.WithMaxRetries(0))

			ctx := t.Context()
			if tt.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			results, err := claude.Categorize(ctx, batch)

			if tt.wantErr != nil || tt.wantErrMsg != "" {
				if err == nil {
					t.Fatalf("expected error, got results %v", results)
				}
				if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
					t.Errorf("expected error %v, got %v", tt.wantErr, err)
				}
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("expected error containing %q, got %v", tt.wantErrMsg, err)
				}
				if results != nil {
					t.Errorf("expected nil results on error, got %v", results)
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
				if r.Category != tt.wantCategories[i] {
					t.Errorf("result %d: expected category %q, got %q", i, tt.wantCategories[i], r.Category)
				}
			}
		})
	}
}

// assertCategorizeRequest checks what Categorize sends: the categorize_batch
// tool restricted to the configured categories, and every transaction in the
// prompt. It runs on the server's goroutine, so it reports with Errorf, never
// Fatalf.
func assertCategorizeRequest(t *testing.T, body []byte, cats []categories.Category, batch []Transaction) {
	t.Helper()

	var req messagesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Errorf("unmarshal request body: %v", err)
		return
	}

	if len(req.Tools) != 1 || req.Tools[0].Name != "categorize_batch" {
		t.Errorf("expected one categorize_batch tool, got %+v", req.Tools)
		return
	}
	// 50 result entries need ~1,500 output tokens; 1024 truncated them (#65).
	if req.MaxTokens < 4096 {
		t.Errorf("expected max_tokens >= 4096, got %d", req.MaxTokens)
	}
	if req.ToolChoice.Type != "tool" || req.ToolChoice.Name != "categorize_batch" {
		t.Errorf("expected tool_choice forcing categorize_batch, got %+v", req.ToolChoice)
	}
	if want := []string{"results"}; !reflect.DeepEqual(req.Tools[0].InputSchema.Required, want) {
		t.Errorf("expected required %v, got %v", want, req.Tools[0].InputSchema.Required)
	}
	enum := req.Tools[0].InputSchema.Properties.Results.Items.Properties.Category.Enum
	var names []string
	for _, c := range cats {
		names = append(names, c.Name)
		// Claude tells borderline categories apart by their descriptions (#103).
		if c.Description != "" && !strings.Contains(req.Tools[0].Description, c.Name+": "+c.Description) {
			t.Errorf("tool description is missing %q", c.Name+": "+c.Description)
		}
	}
	if !reflect.DeepEqual(enum, names) {
		t.Errorf("expected category enum %v, got %v", names, enum)
	}

	if len(req.Messages) != 1 || len(req.Messages[0].Content) != 1 {
		t.Errorf("expected one message with one content block, got %+v", req.Messages)
		return
	}
	prompt := req.Messages[0].Content[0].Text
	for _, tx := range batch {
		if !strings.Contains(prompt, tx.Description) {
			t.Errorf("prompt is missing transaction %q", tx.Description)
		}
	}
}
