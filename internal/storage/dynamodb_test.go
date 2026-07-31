package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

type fakeDynamoClient struct {
	putItemErr error

	batchWriteCalls int
	unprocessedLeft int // number of BatchWriteItem calls that should still report unprocessed items
	batchWriteErr   error
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
