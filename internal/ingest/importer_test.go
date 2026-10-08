package ingest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/KrivAlexey/personal_spends/internal/categorizer"
	"github.com/KrivAlexey/personal_spends/internal/parser"
	"github.com/KrivAlexey/personal_spends/internal/storage"
)

type fakeParser struct {
	res parser.ParseResult
	err error
}

func (f *fakeParser) ParseCSV(context.Context, string, io.Reader) (parser.ParseResult, error) {
	return f.res, f.err
}

type fakeCategorizer struct {
	batches   [][]categorizer.Transaction
	failBatch int // 1-based batch that fails; 0 = never
}

func (f *fakeCategorizer) Categorize(_ context.Context, batch []categorizer.Transaction) ([]categorizer.CategorizedTransaction, error) {
	f.batches = append(f.batches, batch)
	if len(f.batches) == f.failBatch {
		return nil, errCategorize
	}
	out := make([]categorizer.CategorizedTransaction, len(batch))
	for i, tx := range batch {
		out[i] = categorizer.CategorizedTransaction{Transaction: tx, Category: "groceries", Confidence: 0.9}
	}
	return out, nil
}

type fakeStore struct {
	known     map[string]bool
	knownErr  error
	saveErr   error
	askedKeys []storage.ExpenseKey
	saved     [][]storage.Expense
}

func (f *fakeStore) KnownIDs(_ context.Context, keys []storage.ExpenseKey) (map[string]bool, error) {
	f.askedKeys = keys
	if f.knownErr != nil {
		return nil, f.knownErr
	}
	out := map[string]bool{}
	for _, k := range keys {
		if f.known[k.ID] {
			out[k.ID] = true
		}
	}
	return out, nil
}

func (f *fakeStore) SaveExpenses(_ context.Context, expenses []storage.Expense) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, expenses)
	return nil
}

var (
	errCategorize = errors.New("categorize failed")
	errParse      = errors.New("parse failed")
	errKnown      = errors.New("lookup failed")
	errSave       = errors.New("save failed")
	day           = time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	fixedNow      = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
)

func txs(ids ...string) []categorizer.Transaction {
	out := make([]categorizer.Transaction, len(ids))
	for i, id := range ids {
		out[i] = categorizer.Transaction{ID: id, Date: day, Amount: -1, Merchant: "M " + id, Currency: "EUR", Description: "D " + id, Source: "sparkasse"}
	}
	return out
}

func manyIDs(n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("id%02d-0", i)
	}
	return ids
}

func batchSizes[T any](batches [][]T) []int {
	sizes := make([]int, len(batches))
	for i, b := range batches {
		sizes[i] = len(b)
	}
	return sizes
}

func savedIDs(batches [][]storage.Expense) string {
	var ids []string
	for _, b := range batches {
		for _, e := range b {
			ids = append(ids, e.ID)
		}
	}
	return strings.Join(ids, ",")
}

func TestImporter_Import(t *testing.T) {
	tests := []struct {
		name           string
		parsed         parser.ParseResult
		parseErr       error
		known          map[string]bool
		knownErr       error
		saveErr        error
		failBatch      int
		batchSize      int
		want           Result
		wantErr        error
		wantCategorize []int  // batch sizes sent to Categorize
		wantSaved      string // IDs saved, in order
	}{
		{
			name:           "all new, partial last batch",
			parsed:         parser.ParseResult{Transactions: txs("a", "b", "c")},
			batchSize:      2,
			want:           Result{Imported: 3},
			wantCategorize: []int{2, 1},
			wantSaved:      "a,b,c",
		},
		{
			name:           "all known: nothing categorized or saved",
			parsed:         parser.ParseResult{Transactions: txs("a", "b", "c")},
			known:          map[string]bool{"a": true, "b": true, "c": true},
			batchSize:      2,
			want:           Result{AlreadyKnown: 3},
			wantCategorize: []int{},
			wantSaved:      "",
		},
		{
			name:           "mixed: only new rows categorized and saved",
			parsed:         parser.ParseResult{Transactions: txs("a", "b", "c")},
			known:          map[string]bool{"b": true},
			batchSize:      50,
			want:           Result{Imported: 2, AlreadyKnown: 1},
			wantCategorize: []int{2},
			wantSaved:      "a,c",
		},
		{
			name:           "pending count passed through; counts add up",
			parsed:         parser.ParseResult{Transactions: txs("a", "b"), PendingSkipped: 4},
			known:          map[string]bool{"a": true},
			batchSize:      50,
			want:           Result{Imported: 1, AlreadyKnown: 1, PendingSkipped: 4},
			wantCategorize: []int{1},
			wantSaved:      "b",
		},
		{
			name:           "non-positive batch size uses the default",
			parsed:         parser.ParseResult{Transactions: txs(manyIDs(DefaultBatchSize + 10)...)},
			batchSize:      0,
			want:           Result{Imported: DefaultBatchSize + 10},
			wantCategorize: []int{DefaultBatchSize, 10},
			wantSaved:      strings.Join(manyIDs(DefaultBatchSize+10), ","),
		},
		{
			name:           "categorizer fails on batch 2: batch 1 stays saved",
			parsed:         parser.ParseResult{Transactions: txs("a", "b", "c", "d", "e")},
			batchSize:      2,
			failBatch:      2,
			want:           Result{Imported: 2},
			wantErr:        errCategorize,
			wantCategorize: []int{2, 2},
			wantSaved:      "a,b",
		},
		{
			name:           "parser error: nothing looked up",
			parseErr:       errParse,
			batchSize:      2,
			wantErr:        errParse,
			wantCategorize: []int{},
		},
		{
			name:           "lookup error: nothing categorized",
			parsed:         parser.ParseResult{Transactions: txs("a")},
			knownErr:       errKnown,
			batchSize:      2,
			wantErr:        errKnown,
			wantCategorize: []int{},
		},
		{
			name:           "save error is returned",
			parsed:         parser.ParseResult{Transactions: txs("a")},
			saveErr:        errSave,
			batchSize:      2,
			wantErr:        errSave,
			wantCategorize: []int{1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat := &fakeCategorizer{failBatch: tt.failBatch}
			store := &fakeStore{known: tt.known, knownErr: tt.knownErr, saveErr: tt.saveErr}
			im := NewImporter(&fakeParser{res: tt.parsed, err: tt.parseErr}, cat, store, tt.batchSize)

			got, err := im.Import(context.Background(), "sparkasse", strings.NewReader(""))

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("result = %+v, want %+v", got, tt.want)
			}
			if sizes := batchSizes(cat.batches); fmt.Sprint(sizes) != fmt.Sprint(tt.wantCategorize) {
				t.Errorf("Categorize batch sizes = %v, want %v", sizes, tt.wantCategorize)
			}
			if ids := savedIDs(store.saved); ids != tt.wantSaved {
				t.Errorf("saved IDs = %q, want %q", ids, tt.wantSaved)
			}
		})
	}
}

func TestImporter_LooksUpEveryParsedRow(t *testing.T) {
	store := &fakeStore{}
	im := NewImporter(&fakeParser{res: parser.ParseResult{Transactions: txs("a", "b")}}, &fakeCategorizer{}, store, 50)

	if _, err := im.Import(context.Background(), "sparkasse", strings.NewReader("")); err != nil {
		t.Fatalf("Import: %v", err)
	}
	want := []storage.ExpenseKey{{Date: day, ID: "a"}, {Date: day, ID: "b"}}
	if fmt.Sprint(store.askedKeys) != fmt.Sprint(want) {
		t.Errorf("KnownIDs keys = %v, want %v", store.askedKeys, want)
	}
}

func TestImporter_ExpenseMapping(t *testing.T) {
	store := &fakeStore{}
	im := NewImporter(&fakeParser{res: parser.ParseResult{Transactions: txs("a")}}, &fakeCategorizer{}, store, 50)
	im.now = func() time.Time { return fixedNow }

	if _, err := im.Import(context.Background(), "sparkasse", strings.NewReader("")); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(store.saved) != 1 || len(store.saved[0]) != 1 {
		t.Fatalf("saved = %v, want one expense", store.saved)
	}

	want := storage.Expense{
		ID:             "a",
		Date:           day,
		Amount:         -1,
		Currency:       "EUR",
		Merchant:       "M a",
		RawDescription: "D a",
		Category:       "groceries",
		Confidence:     0.9,
		Source:         "sparkasse",
		CreatedAt:      fixedNow,
	}
	if got := store.saved[0][0]; got != want {
		t.Errorf("expense = %+v\nwant      %+v", got, want)
	}
}
