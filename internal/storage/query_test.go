package storage

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func ptr(f float64) *float64 { return &f }

// page marshals expenses into one Query page, in the given order.
func page(t *testing.T, expenses ...Expense) []map[string]types.AttributeValue {
	t.Helper()
	items := make([]map[string]types.AttributeValue, len(expenses))
	for i, e := range expenses {
		av, err := attributevalue.MarshalMap(newExpenseItem(e))
		if err != nil {
			t.Fatal(err)
		}
		items[i] = av
	}
	return items
}

func exp(id string, date time.Time, category string, amount float64) Expense {
	return Expense{
		ID: id, Date: date, Amount: amount, Currency: "EUR", Merchant: "REWE",
		RawDescription: "Kartenzahlung", Category: category, Confidence: 0.9, Source: "sparkasse",
		CreatedAt: time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC),
	}
}

// keyValues are the string and number values bound in the Query's expressions.
func keyValues(in *dynamodb.QueryInput) (strs []string, nums []string) {
	for _, v := range in.ExpressionAttributeValues {
		switch v := v.(type) {
		case *types.AttributeValueMemberS:
			strs = append(strs, v.Value)
		case *types.AttributeValueMemberN:
			nums = append(nums, v.Value)
		}
	}
	return strs, nums
}

func TestStore_QueryExpenses_KeyConditions(t *testing.T) {
	apr1 := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	apr30 := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		query     ExpenseQuery
		wantIndex string // "" is the base table
		wantStrs  []string
		noStrs    []string
		wantNums  []string
		filter    bool
	}{
		{
			name:     "date range on the base table",
			query:    ExpenseQuery{From: apr1, To: apr30},
			wantStrs: []string{"EXPENSES", "2026-04-01", "2026-04-30#~"},
		},
		{
			name:     "only from leaves the end open",
			query:    ExpenseQuery{From: apr1},
			wantStrs: []string{"EXPENSES", "2026-04-01", "9999-12-31#~"},
		},
		{
			name:     "only to leaves the start open",
			query:    ExpenseQuery{To: apr30},
			wantStrs: []string{"EXPENSES", "0000-01-01", "2026-04-30#~"},
		},
		{
			name:      "category uses GSI1",
			query:     ExpenseQuery{From: apr1, To: apr30, Category: "groceries"},
			wantIndex: "GSI1",
			wantStrs:  []string{"CAT#groceries", "2026-04-01", "2026-04-30#~"},
			noStrs:    []string{"EXPENSES"},
		},
		{
			name:     "amount bounds are a filter expression",
			query:    ExpenseQuery{From: apr1, To: apr30, MinAmount: ptr(-50), MaxAmount: ptr(-10.5)},
			wantStrs: []string{"EXPENSES"},
			wantNums: []string{"-50", "-10.5"},
			filter:   true,
		},
		{
			name:     "min amount only",
			query:    ExpenseQuery{MinAmount: ptr(0)},
			wantStrs: []string{"EXPENSES"},
			wantNums: []string{"0"},
			filter:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeDynamoClient{}
			store := newStore(client, "expenses", nil)

			if _, err := store.QueryExpenses(context.Background(), tt.query); err != nil {
				t.Fatalf("QueryExpenses() error = %v", err)
			}
			if len(client.queryInputs) != 1 {
				t.Fatalf("Query calls = %d, want 1", len(client.queryInputs))
			}
			in := client.queryInputs[0]
			if got := deref(in.IndexName); got != tt.wantIndex {
				t.Errorf("IndexName = %q, want %q", got, tt.wantIndex)
			}
			if deref(in.TableName) != "expenses" {
				t.Errorf("TableName = %q, want expenses", deref(in.TableName))
			}
			if in.KeyConditionExpression == nil {
				t.Error("KeyConditionExpression is nil; a Query needs one")
			}
			if in.ScanIndexForward == nil || *in.ScanIndexForward {
				t.Errorf("ScanIndexForward = %v, want false (newest first)", in.ScanIndexForward)
			}
			if (in.FilterExpression != nil) != tt.filter {
				t.Errorf("FilterExpression = %v, want set = %v", in.FilterExpression, tt.filter)
			}
			strs, nums := keyValues(in)
			for _, want := range tt.wantStrs {
				if !slices.Contains(strs, want) {
					t.Errorf("bound strings %v lack %q", strs, want)
				}
			}
			for _, no := range tt.noStrs {
				if slices.Contains(strs, no) {
					t.Errorf("bound strings %v contain %q", strs, no)
				}
			}
			slices.Sort(nums)
			want := slices.Clone(tt.wantNums)
			slices.Sort(want)
			if !slices.Equal(nums, want) {
				t.Errorf("bound numbers = %v, want %v", nums, want)
			}
		})
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func TestStore_QueryExpenses_MapsItems(t *testing.T) {
	want := exp("3f2a9c1b7d4e8f60-0", day, "groceries", -23.45)
	client := &fakeDynamoClient{queryPages: [][]map[string]types.AttributeValue{page(t, want)}}
	store := newStore(client, "expenses", nil)

	got, err := store.QueryExpenses(context.Background(), ExpenseQuery{})
	if err != nil {
		t.Fatalf("QueryExpenses() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d expenses, want 1", len(got))
	}
	g := got[0]
	if g.ID != want.ID || !g.Date.Equal(want.Date) || g.Amount != want.Amount || g.Currency != want.Currency ||
		g.Merchant != want.Merchant || g.RawDescription != want.RawDescription || g.Category != want.Category ||
		g.Confidence != want.Confidence || g.Source != want.Source || !g.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("got %+v, want %+v", g, want)
	}
}

func TestStore_QueryExpenses_Paging(t *testing.T) {
	d := func(n int) time.Time { return time.Date(2026, 4, n, 0, 0, 0, 0, time.UTC) }
	pages := func(t *testing.T) [][]map[string]types.AttributeValue {
		return [][]map[string]types.AttributeValue{
			page(t, exp("a-0", d(6), "x", -1), exp("b-0", d(5), "x", -1)),
			page(t, exp("c-0", d(4), "x", -1), exp("d-0", d(3), "x", -1)),
			page(t, exp("e-0", d(2), "x", -1), exp("f-0", d(1), "x", -1)),
		}
	}
	tests := []struct {
		name      string
		limit     int
		wantIDs   []string
		wantCalls int
	}{
		{name: "limit inside the first page", limit: 1, wantIDs: []string{"a-0"}, wantCalls: 1},
		{name: "limit across pages stops early", limit: 3, wantIDs: []string{"a-0", "b-0", "c-0"}, wantCalls: 2},
		{name: "no limit reads every page", limit: 0, wantIDs: []string{"a-0", "b-0", "c-0", "d-0", "e-0", "f-0"}, wantCalls: 3},
		{name: "limit above the matches", limit: 100, wantIDs: []string{"a-0", "b-0", "c-0", "d-0", "e-0", "f-0"}, wantCalls: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeDynamoClient{queryPages: pages(t)}
			store := newStore(client, "expenses", nil)

			got, err := store.QueryExpenses(context.Background(), ExpenseQuery{Limit: tt.limit})
			if err != nil {
				t.Fatalf("QueryExpenses() error = %v", err)
			}
			var ids []string
			for _, e := range got {
				ids = append(ids, e.ID)
			}
			if !slices.Equal(ids, tt.wantIDs) {
				t.Errorf("ids = %v, want %v", ids, tt.wantIDs)
			}
			if len(client.queryInputs) != tt.wantCalls {
				t.Errorf("Query calls = %d, want %d", len(client.queryInputs), tt.wantCalls)
			}
			for i, in := range client.queryInputs {
				if (in.ExclusiveStartKey != nil) != (i > 0) {
					t.Errorf("call %d: ExclusiveStartKey = %v, want set only after the first call", i, in.ExclusiveStartKey)
				}
			}
		})
	}
}

func TestStore_QueryExpenses_Errors(t *testing.T) {
	t.Run("negative limit", func(t *testing.T) {
		client := &fakeDynamoClient{}
		_, err := newStore(client, "expenses", nil).QueryExpenses(context.Background(), ExpenseQuery{Limit: -1})
		if err == nil {
			t.Fatal("expected an error")
		}
		if len(client.queryInputs) != 0 {
			t.Errorf("Query calls = %d, want 0", len(client.queryInputs))
		}
	})
	t.Run("context error passes through", func(t *testing.T) {
		client := &fakeDynamoClient{queryErr: context.Canceled}
		_, err := newStore(client, "expenses", nil).QueryExpenses(context.Background(), ExpenseQuery{})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want wrapped context.Canceled", err)
		}
	})
	t.Run("API error drops the message", func(t *testing.T) {
		client := &fakeDynamoClient{queryErr: fakeAPIError{code: "ValidationException", message: "invalid value REWE Markt GmbH"}}
		_, err := newStore(client, "expenses", nil).QueryExpenses(context.Background(), ExpenseQuery{Category: "groceries"})
		if err == nil || !strings.Contains(err.Error(), "ValidationException") {
			t.Fatalf("error = %v, want it to name ValidationException", err)
		}
		if strings.Contains(err.Error(), "REWE") {
			t.Errorf("error %q contains the server message", err)
		}
	})
}
