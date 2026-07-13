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

		backoff := time.Duration(attempt+1) * 100 * time.Millisecond
		select {
		case <-ctx.Done():
			return fmt.Errorf("batch write cancelled: %w", ctx.Err())
		case <-time.After(backoff):
		}
	}

	return fmt.Errorf("batch write: %d items still unprocessed after %d attempts", len(requestItems[store.table]), MaxRetryAttempts)
}
