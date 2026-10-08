// Package ingest imports a bank CSV export: it parses it, skips transactions
// already stored, categorizes the rest in batches and saves them.
package ingest

import (
	"context"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/KrivAlexey/personal_spends/internal/categorizer"
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
	now         func() time.Time
}

// NewImporter returns an Importer that categorizes batchSize transactions per
// call; a non-positive batchSize means DefaultBatchSize.
func NewImporter(p csvParser, c categorizer.Categorizer, s expenseStore, batchSize int) *Importer {
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	return &Importer{parser: p, categorizer: c, store: s, batchSize: batchSize, now: time.Now}
}

// Import stores the export's booked transactions that aren't stored yet.
// Batches are saved as they're categorized (ADR 0007, serial loop): if a batch
// fails, earlier batches stay stored and the partial Result is returned with
// the error. Re-running the import is safe, because stored rows are skipped
// (ADR 0011).
func (im *Importer) Import(ctx context.Context, bankName string, r io.Reader) (Result, error) {
	parsed, err := im.parser.ParseCSV(ctx, bankName, r)
	if err != nil {
		return Result{}, fmt.Errorf("ingest: parse %s export: %w", bankName, err)
	}
	res := Result{PendingSkipped: parsed.PendingSkipped}

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

	for batch := range slices.Chunk(fresh, im.batchSize) {
		categorized, err := im.categorizer.Categorize(ctx, batch)
		if err != nil {
			return res, fmt.Errorf("ingest: categorize: %w", err)
		}

		expenses := make([]storage.Expense, len(categorized))
		for i, ct := range categorized {
			expenses[i] = im.toExpense(ct)
		}
		if err := im.store.SaveExpenses(ctx, expenses); err != nil {
			return res, fmt.Errorf("ingest: save: %w", err)
		}
		res.Imported += len(expenses)
	}
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
