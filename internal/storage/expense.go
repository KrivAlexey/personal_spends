package storage

import "time"

// Expense is the domain model for a single categorized transaction.
// It has no DynamoDB-specific concerns - see item.go for the mapping
// to the table's PK/SK/GSI schema.
type Expense struct {
	ID             string
	Date           time.Time
	Amount         float64
	Currency       string
	Merchant       string
	RawDescription string
	Category       string
	Confidence     float64
	Source         string
	CreatedAt      time.Time
}
