package storage

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type fakeDynamoClient struct {
	putItemErr error

	batchWriteCalls int
	unprocessedLeft int // number of BatchWriteItem calls that should still report unprocessed items
	batchWriteErr   error

	stored             map[string]bool // SKs that BatchGetItem finds
	batchGetCalls      int
	maxKeysPerGet      int
	unprocessedGetLeft int // number of BatchGetItem calls that return every key as unprocessed
	batchGetErr        error
	lastProjection     *string
}

func (f *fakeDynamoClient) BatchGetItem(ctx context.Context, params *dynamodb.BatchGetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error) {
	f.batchGetCalls++
	if f.batchGetErr != nil {
		return nil, f.batchGetErr
	}

	out := &dynamodb.BatchGetItemOutput{Responses: map[string][]map[string]types.AttributeValue{}}
	for table, req := range params.RequestItems {
		f.maxKeysPerGet = max(f.maxKeysPerGet, len(req.Keys))
		f.lastProjection = req.ProjectionExpression
		if f.unprocessedGetLeft > 0 {
			f.unprocessedGetLeft--
			out.UnprocessedKeys = params.RequestItems
			return out, nil
		}
		for _, key := range req.Keys {
			sk := key["SK"].(*types.AttributeValueMemberS).Value
			if f.stored[sk] {
				out.Responses[table] = append(out.Responses[table], map[string]types.AttributeValue{
					"SK": &types.AttributeValueMemberS{Value: sk},
				})
			}
		}
	}
	return out, nil
}

func (f *fakeDynamoClient) PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	if f.putItemErr != nil {
		return nil, f.putItemErr
	}
	return &dynamodb.PutItemOutput{}, nil
}

func (f *fakeDynamoClient) BatchWriteItem(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error) {
	f.batchWriteCalls++

	if f.batchWriteErr != nil {
		return nil, f.batchWriteErr
	}

	if f.unprocessedLeft > 0 {
		f.unprocessedLeft--
		return &dynamodb.BatchWriteItemOutput{UnprocessedItems: params.RequestItems}, nil
	}

	return &dynamodb.BatchWriteItemOutput{}, nil
}

func TestStore_PutExpense(t *testing.T) {
	expense := Expense{ID: "1", Date: time.Now(), Category: "Groceries"}

	t.Run("success", func(t *testing.T) {
		client := &fakeDynamoClient{}
		store := &Store{client: client, table: "expenses"}

		if err := store.PutExpense(context.Background(), expense); err != nil {
			t.Fatalf("PutExpense() error = %v", err)
		}
	})

	t.Run("client error", func(t *testing.T) {
		wantErr := errors.New("boom")
		client := &fakeDynamoClient{putItemErr: wantErr}
		store := &Store{client: client, table: "expenses"}

		err := store.PutExpense(context.Background(), expense)
		if !errors.Is(err, wantErr) {
			t.Fatalf("PutExpense() error = %v, want wrapped %v", err, wantErr)
		}
	})
}

func TestStore_SaveExpenses_Empty(t *testing.T) {
	client := &fakeDynamoClient{}
	store := &Store{client: client, table: "expenses"}

	if err := store.SaveExpenses(context.Background(), nil); err != nil {
		t.Fatalf("SaveExpenses() error = %v", err)
	}
	if client.batchWriteCalls != 0 {
		t.Errorf("batchWriteCalls = %d, want 0", client.batchWriteCalls)
	}
}

func TestStore_SaveExpenses_Chunking(t *testing.T) {
	client := &fakeDynamoClient{}
	store := &Store{client: client, table: "expenses"}

	expenses := make([]Expense, MaxBatchItems+5) // forces two chunks
	for i := range expenses {
		expenses[i] = Expense{ID: "id", Date: time.Now(), Category: "Groceries"}
	}

	if err := store.SaveExpenses(context.Background(), expenses); err != nil {
		t.Fatalf("SaveExpenses() error = %v", err)
	}
	if client.batchWriteCalls != 2 {
		t.Errorf("batchWriteCalls = %d, want 2", client.batchWriteCalls)
	}
}

func TestStore_SaveExpenses_RetriesUnprocessed(t *testing.T) {
	client := &fakeDynamoClient{unprocessedLeft: 1} // fails once, then succeeds
	store := &Store{client: client, table: "expenses"}

	expenses := []Expense{{ID: "1", Date: time.Now(), Category: "Groceries"}}

	if err := store.SaveExpenses(context.Background(), expenses); err != nil {
		t.Fatalf("SaveExpenses() error = %v", err)
	}
	if client.batchWriteCalls != 2 {
		t.Errorf("batchWriteCalls = %d, want 2 (one retry)", client.batchWriteCalls)
	}
}

func TestStore_SaveExpenses_GivesUpAfterMaxRetries(t *testing.T) {
	client := &fakeDynamoClient{unprocessedLeft: MaxRetryAttempts + 1} // never clears
	store := &Store{client: client, table: "expenses"}

	expenses := []Expense{{ID: "1", Date: time.Now(), Category: "Groceries"}}

	err := store.SaveExpenses(context.Background(), expenses)
	if err == nil {
		t.Fatal("expected error after exhausting retries, got nil")
	}
	if client.batchWriteCalls != MaxRetryAttempts {
		t.Errorf("batchWriteCalls = %d, want %d", client.batchWriteCalls, MaxRetryAttempts)
	}
}

func TestStore_SaveExpenses_ContextCancelled(t *testing.T) {
	client := &fakeDynamoClient{unprocessedLeft: MaxRetryAttempts} // always unprocessed, forces the backoff wait
	store := &Store{client: client, table: "expenses"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	expenses := []Expense{{ID: "1", Date: time.Now(), Category: "Groceries"}}

	err := store.SaveExpenses(ctx, expenses)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SaveExpenses() error = %v, want wrapped %v", err, context.Canceled)
	}
	if client.batchWriteCalls != 1 {
		t.Errorf("batchWriteCalls = %d, want 1 (cancelled during backoff, before a retry)", client.batchWriteCalls)
	}
}

func TestStore_SaveExpenses_ClientError(t *testing.T) {
	wantErr := errors.New("throttled")
	client := &fakeDynamoClient{batchWriteErr: wantErr}
	store := &Store{client: client, table: "expenses"}

	expenses := []Expense{{ID: "1", Date: time.Now(), Category: "Groceries"}}

	err := store.SaveExpenses(context.Background(), expenses)
	if !errors.Is(err, wantErr) {
		t.Fatalf("SaveExpenses() error = %v, want wrapped %v", err, wantErr)
	}
	if client.batchWriteCalls != 1 {
		t.Errorf("batchWriteCalls = %d, want 1 (no retry on hard error)", client.batchWriteCalls)
	}
}

func keysFor(n int, date time.Time) []ExpenseKey {
	keys := make([]ExpenseKey, n)
	for i := range keys {
		keys[i] = ExpenseKey{Date: date, ID: fmt.Sprintf("id%03d-0", i)}
	}
	return keys
}

func TestStore_KnownIDs(t *testing.T) {
	day := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		client    *fakeDynamoClient
		keys      []ExpenseKey
		wantKnown []string
		wantCalls int
		wantErr   error // matched with errors.Is; nil means success
		cancelled bool
	}{
		{
			name:      "no keys, no call",
			client:    &fakeDynamoClient{},
			keys:      nil,
			wantCalls: 0,
		},
		{
			name:      "mix of known and unknown",
			client:    &fakeDynamoClient{stored: map[string]bool{"2026-08-14#id001-0": true, "2026-08-14#id003-0": true}},
			keys:      keysFor(5, day),
			wantKnown: []string{"id001-0", "id003-0"},
			wantCalls: 1,
		},
		{
			name:      "more than 100 keys are chunked",
			client:    &fakeDynamoClient{stored: map[string]bool{"2026-08-14#id150-0": true}},
			keys:      keysFor(250, day),
			wantKnown: []string{"id150-0"},
			wantCalls: 3,
		},
		{
			name:      "unprocessed keys are retried",
			client:    &fakeDynamoClient{stored: map[string]bool{"2026-08-14#id000-0": true}, unprocessedGetLeft: 1},
			keys:      keysFor(2, day),
			wantKnown: []string{"id000-0"},
			wantCalls: 2,
		},
		{
			name:      "gives up after max retries",
			client:    &fakeDynamoClient{unprocessedGetLeft: MaxRetryAttempts + 1},
			keys:      keysFor(2, day),
			wantCalls: MaxRetryAttempts,
			wantErr:   errAny,
		},
		{
			name:      "client error is wrapped",
			client:    &fakeDynamoClient{batchGetErr: errBoom},
			keys:      keysFor(2, day),
			wantCalls: 1,
			wantErr:   errBoom,
		},
		{
			name:      "cancelled during backoff",
			client:    &fakeDynamoClient{unprocessedGetLeft: MaxRetryAttempts},
			keys:      keysFor(2, day),
			wantCalls: 1,
			wantErr:   context.Canceled,
			cancelled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.cancelled {
				cancel()
			}
			store := &Store{client: tt.client, table: "expenses"}

			known, err := store.KnownIDs(ctx, tt.keys)

			switch {
			case tt.wantErr == errAny && err == nil:
				t.Fatal("expected an error, got nil")
			case tt.wantErr != nil && tt.wantErr != errAny && !errors.Is(err, tt.wantErr):
				t.Fatalf("error = %v, want wrapped %v", err, tt.wantErr)
			case tt.wantErr == nil && err != nil:
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.client.batchGetCalls != tt.wantCalls {
				t.Errorf("BatchGetItem calls = %d, want %d", tt.client.batchGetCalls, tt.wantCalls)
			}
			if tt.client.maxKeysPerGet > 100 {
				t.Errorf("a BatchGetItem call had %d keys, DynamoDB allows 100", tt.client.maxKeysPerGet)
			}
			if tt.wantErr != nil {
				return
			}
			if len(known) != len(tt.wantKnown) {
				t.Errorf("known = %v, want %v", known, tt.wantKnown)
			}
			for _, id := range tt.wantKnown {
				if !known[id] {
					t.Errorf("known lacks %q (got %v)", id, known)
				}
			}
			if tt.wantCalls > 0 && (tt.client.lastProjection == nil || *tt.client.lastProjection != "SK") {
				t.Errorf("ProjectionExpression = %v, want SK only", tt.client.lastProjection)
			}
		})
	}
}

var (
	errBoom = errors.New("boom")
	errAny  = errors.New("any error")
)
