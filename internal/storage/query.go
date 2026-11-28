package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Bounds for an open end of the date range. "#~" sorts after every sort key
// of that day, whatever the ID.
const (
	earliestDate = "0000-01-01"
	latestDate   = "9999-12-31"
	afterDay     = "#~"
)

// ExpenseQuery selects stored expenses. The zero value matches all of them.
type ExpenseQuery struct {
	From, To  time.Time // booking dates, inclusive; zero leaves that end open
	Category  string    // "" matches every category
	MinAmount *float64  // inclusive; nil leaves that end open
	MaxAmount *float64
	Limit     int // 0 returns every match
}

// QueryExpenses returns the expenses matching q, newest booking date first.
// It reads the base table by date, or GSI1 when q names a category, and never
// scans. The amount bounds are a filter expression, applied by DynamoDB after
// the read, so it follows result pages until Limit matches are found.
func (store *Store) QueryExpenses(ctx context.Context, q ExpenseQuery) ([]Expense, error) {
	if q.Limit < 0 {
		return nil, fmt.Errorf("storage: query limit %d is negative", q.Limit)
	}
	in, err := store.queryInput(q)
	if err != nil {
		return nil, err
	}

	var out []Expense
	for {
		res, err := store.client.Query(ctx, in)
		if err != nil {
			return nil, fmt.Errorf("storage: query expenses: %w", safeAPIError(err))
		}
		for _, av := range res.Items {
			e, err := expenseFromItem(av)
			if err != nil {
				return nil, err
			}
			out = append(out, e)
			if q.Limit > 0 && len(out) == q.Limit {
				return out, nil
			}
		}
		if len(res.LastEvaluatedKey) == 0 {
			return out, nil
		}
		next := *in // the caller's fake or logger may keep the previous input
		next.ExclusiveStartKey = res.LastEvaluatedKey
		in = &next
	}
}

func (store *Store) queryInput(q ExpenseQuery) (*dynamodb.QueryInput, error) {
	lo, hi := earliestDate, latestDate+afterDay
	if !q.From.IsZero() {
		lo = q.From.Format(time.DateOnly)
	}
	if !q.To.IsZero() {
		hi = q.To.Format(time.DateOnly) + afterDay
	}

	in := &dynamodb.QueryInput{
		TableName:                aws.String(store.table),
		KeyConditionExpression:   aws.String("#pk = :pk AND #sk BETWEEN :lo AND :hi"),
		ExpressionAttributeNames: map[string]string{"#pk": "PK", "#sk": "SK"},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: expensesPK},
			":lo": &types.AttributeValueMemberS{Value: lo},
			":hi": &types.AttributeValueMemberS{Value: hi},
		},
		ScanIndexForward: aws.Bool(false),
	}
	if q.Category != "" {
		in.IndexName = aws.String("GSI1")
		in.ExpressionAttributeNames = map[string]string{"#pk": "GSI1PK", "#sk": "GSI1SK"}
		in.ExpressionAttributeValues[":pk"] = &types.AttributeValueMemberS{Value: categoryKey(q.Category)}
	}

	var conds []string
	for _, b := range []struct {
		bound *float64
		cond  string
		name  string
	}{
		{q.MinAmount, "#amount >= :min", ":min"},
		{q.MaxAmount, "#amount <= :max", ":max"},
	} {
		if b.bound == nil {
			continue
		}
		av, err := attributevalue.Marshal(*b.bound)
		if err != nil {
			return nil, fmt.Errorf("storage: marshal amount bound: %w", err)
		}
		in.ExpressionAttributeValues[b.name] = av
		conds = append(conds, b.cond)
	}
	if len(conds) > 0 {
		in.FilterExpression = aws.String(strings.Join(conds, " AND "))
		in.ExpressionAttributeNames["#amount"] = "amount"
	}
	return in, nil
}

// expenseFromItem maps a stored item back to an Expense. Its errors are fixed
// text: the item holds bank data (ADR 0013).
func expenseFromItem(av map[string]types.AttributeValue) (Expense, error) {
	var item expenseItem
	if err := attributevalue.UnmarshalMap(av, &item); err != nil {
		return Expense{}, errors.New("storage: stored expense does not unmarshal")
	}
	day, id, ok := strings.Cut(item.SK, "#")
	date, dateErr := time.Parse(time.DateOnly, day)
	created, createdErr := time.Parse(time.RFC3339, item.CreatedAt)
	if !ok || dateErr != nil || createdErr != nil {
		return Expense{}, errors.New("storage: stored expense has a malformed sort key or created_at")
	}
	return Expense{
		ID: id, Date: date, Amount: item.Amount, Currency: item.Currency, Merchant: item.Merchant,
		RawDescription: item.RawDescription, Category: item.Category, Confidence: item.Confidence,
		Source: item.Source, CreatedAt: created,
	}, nil
}
