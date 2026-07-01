package storage

import (
	"fmt"
	"time"
)

// expensesPK is the single partition key shared by every expense item.
// See docs/architecture.md for the single-table design this mirrors.
const expensesPK = "EXPENSES"

// expenseItem is the on-the-wire shape written to DynamoDB. It mirrors the
// table's PK/SK/GSI1 schema (defined in terraform/main.tf) plus the expense
// attributes themselves.
type expenseItem struct {
	PK             string  `dynamodbav:"PK"`
	SK             string  `dynamodbav:"SK"`
	GSI1PK         string  `dynamodbav:"GSI1PK"`
	GSI1SK         string  `dynamodbav:"GSI1SK"`
	Amount         float64 `dynamodbav:"amount"`
	Currency       string  `dynamodbav:"currency"`
	Merchant       string  `dynamodbav:"merchant"`
	RawDescription string  `dynamodbav:"raw_description"`
	Category       string  `dynamodbav:"category"`
	Confidence     float64 `dynamodbav:"confidence"`
	Source         string  `dynamodbav:"source"`
	CreatedAt      string  `dynamodbav:"created_at"`
}

// newExpenseItem maps a domain Expense onto the DynamoDB table schema.
func newExpenseItem(e Expense) expenseItem {
	sk := fmt.Sprintf("%s#%s", e.Date.Format("2006-01-02"), e.ID)

	return expenseItem{
		PK:             expensesPK,
		SK:             sk,
		GSI1PK:         fmt.Sprintf("CAT#%s", e.Category),
		GSI1SK:         sk,
		Amount:         e.Amount,
		Currency:       e.Currency,
		Merchant:       e.Merchant,
		RawDescription: e.RawDescription,
		Category:       e.Category,
		Confidence:     e.Confidence,
		Source:         e.Source,
		CreatedAt:      e.CreatedAt.Format(time.RFC3339),
	}
}
