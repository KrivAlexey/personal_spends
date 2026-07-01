import (
	"context"
	"fmt"
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
		params *BatchWriteItemInput,
		optFns ...func(*dynamodb.Options),
	) (*BatchWriteItemOutput, error)
}

type Store struct {
	client dynamoDBAPI
	table  string
}

func NewStore(client *dynamodb.Client, table string) *Store {

}