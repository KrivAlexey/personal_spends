package storage

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const (
	MaxBatchItems    = 25
	MaxBatchGetKeys  = 100
	MaxRetryAttempts = 3
)

type dynamoDBAPI interface {
	PutItem(
		ctx context.Context,
		params *dynamodb.PutItemInput,
		optFns ...func(*dynamodb.Options),
	) (*dynamodb.PutItemOutput, error)

	BatchWriteItem(
		ctx context.Context,
		params *dynamodb.BatchWriteItemInput,
		optFns ...func(*dynamodb.Options),
	) (*dynamodb.BatchWriteItemOutput, error)

	BatchGetItem(
		ctx context.Context,
		params *dynamodb.BatchGetItemInput,
		optFns ...func(*dynamodb.Options),
	) (*dynamodb.BatchGetItemOutput, error)
}

// ExpenseKey addresses one stored expense: SK = "<date>#<ID>".
type ExpenseKey struct {
	Date time.Time
	ID   string
}

// KnownIDs reports which of keys are already stored, keyed by ID. It reads
// only the sort key of each item, in BatchGetItem chunks of MaxBatchGetKeys.
func (store *Store) KnownIDs(ctx context.Context, keys []ExpenseKey) (map[string]bool, error) {
	known := make(map[string]bool)
	for chunk := range slices.Chunk(keys, MaxBatchGetKeys) {
		idBySK := make(map[string]string, len(chunk))
		reqKeys := make([]map[string]types.AttributeValue, len(chunk))
		for i, k := range chunk {
			sk := sortKey(k.Date, k.ID)
			idBySK[sk] = k.ID
			reqKeys[i] = map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: expensesPK},
				"SK": &types.AttributeValueMemberS{Value: sk},
			}
		}

		err := store.getChunkWithRetries(ctx, reqKeys, func(sk string) { known[idBySK[sk]] = true })
		if err != nil {
			return nil, fmt.Errorf("look up known expenses: %w", err)
		}
	}
	return known, nil
}

type Store struct {
	client dynamoDBAPI
	table  string
}

func NewStore(client *dynamodb.Client, table string) *Store {
	return &Store{
		client: client,
		table:  table,
	}
}

func (store *Store) PutExpense(ctx context.Context, expense Expense) error {
	item := newExpenseItem(expense)

	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return fmt.Errorf("can't marshal map Expense:%v err:%w", expense, err)
	}

	putItemInput := &dynamodb.PutItemInput{
		TableName: aws.String(store.table),
		Item:      av,
	}

	_, err = store.client.PutItem(ctx, putItemInput)
	if err != nil {
		return fmt.Errorf("can't write Expense:%v into the DB. err: %w", expense, err)
	}

	return nil
}

func (store *Store) SaveExpenses(ctx context.Context, expenses []Expense) error {
	if len(expenses) == 0 {
		return nil
	}

	for chunk := range slices.Chunk(expenses, MaxBatchItems) {
		writeRequests := make([]types.WriteRequest, len(chunk))

		for i, e := range chunk {
			item := newExpenseItem(e)
			av, err := attributevalue.MarshalMap(item)
			if err != nil {
				return fmt.Errorf("can't marshal map expenses chunk err:%w", err)
			}
			writeRequests[i] = types.WriteRequest{
				PutRequest: &types.PutRequest{
					Item: av,
				},
			}
		}

		err := store.writeChunkWithRetries(ctx, writeRequests)
		if err != nil {
			return fmt.Errorf("write chunk of expenses error. err:%w", err)
		}
	}
	return nil
}

func (store *Store) writeChunkWithRetries(ctx context.Context, requests []types.WriteRequest) error {
	requestItems := map[string][]types.WriteRequest{
		store.table: requests,
	}

	for attempt := 0; attempt < MaxRetryAttempts; attempt++ {
		output, err := store.client.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
			RequestItems: requestItems,
		})
		if err != nil {
			return fmt.Errorf("batch write item: %w", err)
		}
		if len(output.UnprocessedItems) == 0 {
			return nil
		}
		requestItems = output.UnprocessedItems

		if err := waitBackoff(ctx, attempt); err != nil {
			return fmt.Errorf("batch write cancelled: %w", err)
		}
	}

	return fmt.Errorf("batch write: %d items still unprocessed after %d attempts", len(requestItems[store.table]), MaxRetryAttempts)
}

// waitBackoff sleeps before retry attempt+1, or returns ctx's error if it ends first.
func waitBackoff(ctx context.Context, attempt int) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Duration(attempt+1) * 100 * time.Millisecond):
		return nil
	}
}

func (store *Store) getChunkWithRetries(ctx context.Context, keys []map[string]types.AttributeValue, found func(sk string)) error {
	requestItems := map[string]types.KeysAndAttributes{
		store.table: {Keys: keys, ProjectionExpression: aws.String("SK")},
	}

	for attempt := 0; attempt < MaxRetryAttempts; attempt++ {
		output, err := store.client.BatchGetItem(ctx, &dynamodb.BatchGetItemInput{
			RequestItems: requestItems,
		})
		if err != nil {
			return fmt.Errorf("batch get item: %w", err)
		}
		for _, item := range output.Responses[store.table] {
			if sk, ok := item["SK"].(*types.AttributeValueMemberS); ok {
				found(sk.Value)
			}
		}
		if len(output.UnprocessedKeys) == 0 {
			return nil
		}
		requestItems = output.UnprocessedKeys

		if err := waitBackoff(ctx, attempt); err != nil {
			return fmt.Errorf("batch get cancelled: %w", err)
		}
	}

	return fmt.Errorf("batch get: %d keys still unprocessed after %d attempts", len(requestItems[store.table].Keys), MaxRetryAttempts)
}
