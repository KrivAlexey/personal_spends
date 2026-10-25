// Package ingest imports a bank CSV export: it parses it, skips transactions
// already stored, categorizes the rest in batches and saves them.
package ingest

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"time"

	"github.com/KrivAlexey/personal_spends/internal/categorizer"
	"github.com/KrivAlexey/personal_spends/internal/logging"
	"github.com/KrivAlexey/personal_spends/internal/parser"
	"github.com/KrivAlexey/personal_spends/internal/storage"
)

// DefaultBatchSize is the number of transactions per Categorize call.
const DefaultBatchSize = 50

type csvParser interface {
	ParseCSV(ctx context.Context, bankName string, r io.Reader) (parser.ParseResult, error)
}

type expenseStore interface {
	KnownIDs(ctx context.Context, keys []storage.ExpenseKey) (map[string]bool, error)
	SaveExpenses(ctx context.Context, expenses []storage.Expense) error
}

// Result reports what one import did. The three counts add up to the
// export's data rows.
type Result struct {
	Imported       int
	AlreadyKnown   int
	PendingSkipped int
}

type Importer struct {
	parser      csvParser
	categorizer categorizer.Categorizer
	store       expenseStore
	batchSize   int
	logger      *slog.Logger
	now         func() time.Time
}

// NewImporter returns an Importer that categorizes batchSize transactions per
// call; a non-positive batchSize means DefaultBatchSize. A nil logger
// discards.
func NewImporter(p csvParser, c categorizer.Categorizer, s expenseStore, batchSize int, logger *slog.Logger) *Importer {
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Importer{parser: p, categorizer: c, store: s, batchSize: batchSize, logger: logger, now: time.Now}
}

// Import stores the export's booked transactions that aren't stored yet.
// Batches are saved as they're categorized (ADR 0007, serial loop): if a batch
// fails, earlier batches stay stored and the partial Result is returned with
// the error. Re-running the import is safe, because stored rows are skipped
// (ADR 0011).
func (im *Importer) Import(ctx context.Context, bankName string, r io.Reader) (Result, error) {
	start := time.Now()
	parsed, err := im.parser.ParseCSV(ctx, bankName, r)
	if err != nil {
		// No bankName: it comes from the caller, and the handler logs this error.
		return Result{}, fmt.Errorf("ingest: parse export: %w", err)
	}
	res := Result{PendingSkipped: parsed.PendingSkipped}
	// bankName named an existing mapping, so it is ours, not arbitrary input.
	im.logger.DebugContext(ctx, "parsed export",
		"bank", bankName, "rows", len(parsed.Transactions), "pending_skipped", parsed.PendingSkipped)

	keys := make([]storage.ExpenseKey, len(parsed.Transactions))
	for i, tx := range parsed.Transactions {
		keys[i] = storage.ExpenseKey{Date: tx.Date, ID: tx.ID}
	}
	known, err := im.store.KnownIDs(ctx, keys)
	if err != nil {
		return res, fmt.Errorf("ingest: %w", err)
	}

	var fresh []categorizer.Transaction
	for _, tx := range parsed.Transactions {
		if known[tx.ID] {
			res.AlreadyKnown++
			continue
		}
		fresh = append(fresh, tx)
	}
	im.logger.DebugContext(ctx, "deduplicated", "already_known", res.AlreadyKnown, "fresh", len(fresh))

	batches := (len(fresh) + im.batchSize - 1) / im.batchSize
	n := 0
	for batch := range slices.Chunk(fresh, im.batchSize) {
		n++
		batchCtx := logging.WithAttrs(ctx, slog.Int("batch", n), slog.Int("batches", batches))
		categorized, err := im.categorizer.Categorize(batchCtx, batch)
		if err != nil {
			return res, fmt.Errorf("ingest: categorize: %w", err)
		}

		expenses := make([]storage.Expense, len(categorized))
		for i, ct := range categorized {
			expenses[i] = im.toExpense(ct)
		}
		if err := im.store.SaveExpenses(batchCtx, expenses); err != nil {
			return res, fmt.Errorf("ingest: save: %w", err)
		}
		res.Imported += len(expenses)
		im.logger.DebugContext(batchCtx, "saved batch", "saved", len(expenses))
	}
	im.logger.DebugContext(ctx, "import done",
		"imported", res.Imported, "already_known", res.AlreadyKnown, "pending_skipped", res.PendingSkipped,
		"duration_ms", time.Since(start).Milliseconds())
	return res, nil
}

func (im *Importer) toExpense(ct categorizer.CategorizedTransaction) storage.Expense {
	return storage.Expense{
		ID:             ct.ID,
		Date:           ct.Date,
		Amount:         ct.Amount,
		Currency:       ct.Currency,
		Merchant:       ct.Merchant,
		RawDescription: ct.Description,
		Category:       ct.Category,
		Confidence:     ct.Confidence,
		Source:         ct.Source,
		CreatedAt:      im.now(),
	}
}
