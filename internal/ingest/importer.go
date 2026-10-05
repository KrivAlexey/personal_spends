// Package ingest imports a bank CSV export: it parses it, skips transactions
// already stored, categorizes the rest in batches and saves them.
package ingest

import (
	"context"
	"io"
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

func NewImporter(p csvParser, c categorizer.Categorizer, s expenseStore, batchSize int) *Importer {
	return &Importer{parser: p, categorizer: c, store: s, batchSize: batchSize, now: time.Now}
}

func (im *Importer) Import(ctx context.Context, bankName string, r io.Reader) (Result, error) {
	return Result{}, nil
}
