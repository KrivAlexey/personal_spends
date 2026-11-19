package uploads

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/KrivAlexey/personal_spends/internal/ingest"
)

// pgxAPI is the part of *pgxpool.Pool PostgresRecords uses.
type pgxAPI interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// PostgresRecords keeps upload records in the Postgres table "uploads".
type PostgresRecords struct {
	db pgxAPI
}

// NewPostgresRecords returns a PostgresRecords over db, usually a
// *pgxpool.Pool.
func NewPostgresRecords(db pgxAPI) *PostgresRecords {
	return &PostgresRecords{db: db}
}

var errNotImplemented = errors.New("uploads: not implemented")

func (p *PostgresRecords) EnsureSchema(ctx context.Context) error { return errNotImplemented }

func (p *PostgresRecords) Claim(ctx context.Context, sha256, bank, filename string) (Record, bool, error) {
	return Record{}, false, errNotImplemented
}

func (p *PostgresRecords) Finish(ctx context.Context, id string, counts ingest.Result) error {
	return errNotImplemented
}

func (p *PostgresRecords) Fail(ctx context.Context, id string) error { return errNotImplemented }

func (p *PostgresRecords) bySHA256(ctx context.Context, sha256 string) (Record, error) {
	return Record{}, errNotImplemented
}
