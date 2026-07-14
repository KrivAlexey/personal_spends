package categories

import (
	"context"
	"errors"
)

var ErrCategoryNotFound = errors.New("category not found")

type CategoryProvider interface {
	List(ctx context.Context) ([]Category, error)
	Get(ctx context.Context, name string) (Category, error)
}
