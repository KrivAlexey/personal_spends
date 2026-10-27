package categorizer_test

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KrivAlexey/personal_spends/internal/categorizer"
	"github.com/KrivAlexey/personal_spends/internal/categorizer/categorizertest"
	"github.com/KrivAlexey/personal_spends/internal/logging/logtest"
	"github.com/anthropics/anthropic-sdk-go/option"
)

var batch = []categorizer.Transaction{
	{Merchant: "REWE Markt GmbH", Amount: -23.45, Currency: "EUR", Description: "Kartenzahlung 12.03"},
	{Merchant: "Netflix", Amount: -12.99, Currency: "EUR", Description: "NETFLIX.COM"},
	{Merchant: "Shell", Amount: -68.5, Currency: "EUR", Description: "SHELL TANKSTELLE"},
}

// apiResponse is a Messages API answer whose tool input is toolInput.
func apiResponse(stopReason, toolInput string) string {
	return fmt.Sprintf(`{"id": "msg_test", "type": "message", "role": "assistant",
		"model": "claude-haiku-4-5-20251001", "stop_reason": %q,
		"usage": {"input_tokens": 812, "output_tokens": 140},
		"content": [{"type": "tool_use", "id": "toolu_test", "name": "categorize_batch", "input": %s}]}`,
		stopReason, toolInput)
}

const validToolInput = `{"results": [
	{"index": 0, "category": "groceries", "confidence": 0.9},
	{"index": 1, "category": "streaming_subscriptions", "confidence": 0.95},
	{"index": 2, "category": "fuel", "confidence": 0.8}]}`

func fakeAPIWithLogger(t *testing.T, status int, body string, logger *slog.Logger) *categorizer.Claude {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return categorizer.NewClaude("test-key", []string{"groceries", "streaming_subscriptions", "fuel"}, logger,
		option.WithBaseURL(srv.URL), option.WithMaxRetries(0))
}

func TestClaude_BatchLoggingContract(t *testing.T) {
	categorizertest.AssertBatchLogging(t, func(logger *slog.Logger) categorizer.Categorizer {
		return fakeAPIWithLogger(t, http.StatusOK, apiResponse("tool_use", validToolInput), logger)
	}, batch)
}

func TestClaude_BatchRecord(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantErr    bool
		wantRecord bool
		wantStop   string
	}{
		{
			name:       "tool use",
			status:     http.StatusOK,
			body:       apiResponse("tool_use", validToolInput),
			wantRecord: true,
			wantStop:   "tool_use",
		},
		{
			name:       "max_tokens logged before the validation error",
			status:     http.StatusOK,
			body:       apiResponse("max_tokens", `{"results": [{"index": 0, "category": "groceries", "confidence": 0.9}]}`),
			wantErr:    true,
			wantRecord: true,
			wantStop:   "max_tokens",
		},
		{
			name:    "no record when the API call fails",
			status:  http.StatusInternalServerError,
			body:    `{"type": "error", "error": {"type": "api_error", "message": "boom"}}`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, buf := logtest.NewLogger(slog.LevelDebug)
			claude := fakeAPIWithLogger(t, tt.status, tt.body, logger)

			_, err := claude.Categorize(t.Context(), batch)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, want error %v", err, tt.wantErr)
			}

			recs := logtest.Records(t, buf)
			if !tt.wantRecord {
				if len(recs) != 0 {
					t.Errorf("want no records, got:\n%s", buf)
				}
				return
			}
			debug := logtest.Filter(recs, "DEBUG", "")
			if len(debug) != 1 {
				t.Fatalf("got %d DEBUG records, want 1:\n%s", len(debug), buf)
			}
			for k, want := range map[string]any{"size": 3.0, "input_tokens": 812.0, "output_tokens": 140.0, "stop_reason": tt.wantStop} {
				if debug[0][k] != want {
					t.Errorf("%s = %v, want %v", k, debug[0][k], want)
				}
			}
			if d, ok := debug[0]["duration_ms"].(float64); !ok || d < 0 {
				t.Errorf("duration_ms = %v, want non-negative", debug[0]["duration_ms"])
			}
			if strings.Contains(buf.String(), "ERROR") {
				t.Errorf("Claude logged an error; errors are logged at the boundary:\n%s", buf)
			}
		})
	}
}

// The SDK's error text carries the API's raw response body, which could echo
// the prompt; only the status, error type and request ID may pass through.
func TestClaude_APIErrorDropsResponseBody(t *testing.T) {
	claude := fakeAPIWithLogger(t, http.StatusBadRequest,
		`{"type": "error", "error": {"type": "invalid_request_error", "message": "bad value: REWE Markt GmbH -23.45"}}`, nil)

	_, err := claude.Categorize(t.Context(), batch)
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"categorizer: create message", "400", "invalid_request_error"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
	for _, v := range []string{"REWE", "23.45", "bad value"} {
		if strings.Contains(err.Error(), v) {
			t.Errorf("error %q contains response text %q", err, v)
		}
	}
}
