// Package categorizertest holds the contract tests every Categorizer
// implementation's tests must call.
package categorizertest

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"testing"

	"github.com/KrivAlexey/personal_spends/internal/categorizer"
	"github.com/KrivAlexey/personal_spends/internal/logging"
	"github.com/KrivAlexey/personal_spends/internal/logging/logtest"
)

// AssertBatchLogging checks the categorizer spec's batch record: build gets a
// DEBUG logger, and one successful Categorize of batch must write exactly one
// DEBUG record with size, duration_ms, input_tokens, output_tokens and
// stop_reason, carry request_id/batch/batches from the context, and log none of
// the batch's merchants, descriptions or amounts. Give batch distinctive values
// (amounts like -23.45, not -1) so the bank-data check can't match a timestamp.
func AssertBatchLogging(t *testing.T, build func(*slog.Logger) categorizer.Categorizer, batch []categorizer.Transaction) {
	t.Helper()
	logger, buf := logtest.NewLogger(slog.LevelDebug)
	ctx := logging.WithAttrs(context.Background(),
		slog.String("request_id", "contract-req"), slog.Int("batch", 2), slog.Int("batches", 3))

	if _, err := build(logger).Categorize(ctx, batch); err != nil {
		t.Fatalf("Categorize: %v", err)
	}

	var recs []logtest.Record
	for _, r := range logtest.Filter(logtest.Records(t, buf), "DEBUG", "") {
		if _, ok := r["stop_reason"]; ok {
			recs = append(recs, r)
		}
	}
	if len(recs) != 1 {
		t.Fatalf("got %d batch records (DEBUG with stop_reason), want 1:\n%s", len(recs), buf)
	}
	rec := recs[0]
	if rec["size"] != float64(len(batch)) {
		t.Errorf("size = %v, want %d", rec["size"], len(batch))
	}
	for _, k := range []string{"duration_ms", "input_tokens", "output_tokens"} {
		if v, ok := rec[k].(float64); !ok || v < 0 {
			t.Errorf("%s = %v, want a non-negative number", k, rec[k])
		}
	}
	if s, _ := rec["stop_reason"].(string); s == "" {
		t.Errorf("stop_reason = %v, want non-empty text", rec["stop_reason"])
	}
	for k, want := range map[string]any{"request_id": "contract-req", "batch": 2.0, "batches": 3.0} {
		if rec[k] != want {
			t.Errorf("%s = %v, want %v (from the context)", k, rec[k], want)
		}
	}

	var bankData []string
	for _, tx := range batch {
		amount := strconv.FormatFloat(tx.Amount, 'f', -1, 64)
		bankData = append(bankData, tx.Merchant, tx.Description, amount, strings.Replace(amount, ".", ",", 1))
	}
	logtest.AssertLacks(t, buf, bankData...)
}
