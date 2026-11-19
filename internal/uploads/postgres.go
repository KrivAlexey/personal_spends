package uploads

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

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

//go:embed schema.sql
var schemaSQL string

// EnsureSchema creates the uploads table if it doesn't exist.
func (p *PostgresRecords) EnsureSchema(ctx context.Context) error {
	if _, err := p.db.Exec(ctx, schemaSQL); err != nil {
		return fmt.Errorf("uploads: ensure schema: %w", err)
	}
	return nil
}

// claimSQL inserts a processing record, or takes over an existing one that
// failed or whose import started more than 15 minutes ago (Lambda's maximum
// run time, so it can't still be running). The conflicting row is locked for
// the upsert, so concurrent claims of one hash have a single winner. It
// returns no row when the existing record is done or still processing.
const claimSQL = `
INSERT INTO uploads (sha256, filename, bank) VALUES ($1, $2, $3)
ON CONFLICT (sha256) DO UPDATE
   SET status = 'processing', filename = EXCLUDED.filename, bank = EXCLUDED.bank,
       started_at = now(), imported = NULL, already_known = NULL, pending_skipped = NULL
 WHERE uploads.status = 'failed'
    OR (uploads.status = 'processing' AND uploads.started_at < now() - interval '15 minutes')
RETURNING ` + recordColumns

const recordColumns = `id::text, sha256, filename, bank, uploaded_at, started_at, status, imported, already_known, pending_skipped`

// Claim implements the claim described on recordStore.
func (p *PostgresRecords) Claim(ctx context.Context, sha256, bank, filename string) (Record, bool, error) {
	rec, err := scanRecord(p.db.QueryRow(ctx, claimSQL, sha256, filename, bank))
	if err == nil {
		return rec, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Record{}, false, fmt.Errorf("uploads: claim: %w", err)
	}
	// The upsert only skips a row that exists, and rows are never deleted.
	rec, err = p.bySHA256(ctx, sha256)
	if err != nil {
		return Record{}, false, err
	}
	return rec, false, nil
}

// Finish marks the upload done with counts.
func (p *PostgresRecords) Finish(ctx context.Context, claim Record, counts ingest.Result) error {
	return p.setStatus(ctx, claim.ID, StatusDone, &counts)
}

// Fail marks the upload failed.
func (p *PostgresRecords) Fail(ctx context.Context, claim Record) error {
	return p.setStatus(ctx, claim.ID, StatusFailed, nil)
}

// setStatus sets id's status, and its counts when counts isn't nil.
func (p *PostgresRecords) setStatus(ctx context.Context, id string, status Status, counts *ingest.Result) error {
	var imported, known, pending *int
	if counts != nil {
		imported, known, pending = &counts.Imported, &counts.AlreadyKnown, &counts.PendingSkipped
	}
	tag, err := p.db.Exec(ctx,
		`UPDATE uploads SET status = $2, imported = $3, already_known = $4, pending_skipped = $5 WHERE id = $1`,
		id, string(status), imported, known, pending)
	if err != nil {
		return fmt.Errorf("uploads: set %s: %w", status, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("uploads: set %s: no upload %s", status, id)
	}
	return nil
}

// bySHA256 returns the record of the upload with hex SHA-256 sha256.
func (p *PostgresRecords) bySHA256(ctx context.Context, sha256 string) (Record, error) {
	rec, err := scanRecord(p.db.QueryRow(ctx, `SELECT `+recordColumns+` FROM uploads WHERE sha256 = $1`, sha256))
	if err != nil {
		return Record{}, fmt.Errorf("uploads: read record: %w", err)
	}
	return rec, nil
}

func scanRecord(row pgx.Row) (Record, error) {
	var (
		rec                      Record
		status                   string
		imported, known, pending *int
	)
	err := row.Scan(&rec.ID, &rec.SHA256, &rec.Filename, &rec.Bank, &rec.UploadedAt, &rec.StartedAt, &status, &imported, &known, &pending)
	if err != nil {
		return Record{}, err
	}
	rec.Status = Status(status)
	rec.UploadedAt = rec.UploadedAt.In(time.UTC)
	if imported != nil && known != nil && pending != nil {
		rec.Counts = ingest.Result{Imported: *imported, AlreadyKnown: *known, PendingSkipped: *pending}
	}
	return rec, nil
}
