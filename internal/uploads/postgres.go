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
		return fmt.Errorf("postgres: ensure uploads schema: %w", err)
	}
	return nil
}

// claimSQL inserts a processing record, or takes over an existing one that
// failed or whose import started more than $4 seconds (staleAfter) ago; the
// Service ends every import before that, so it can't still be running. The
// conflicting row is locked for the upsert, so concurrent claims of one hash
// have a single winner. It returns no row when the existing record is done or
// still processing.
const claimSQL = `
INSERT INTO uploads (sha256, filename, bank) VALUES ($1, $2, $3)
ON CONFLICT (sha256) DO UPDATE
   SET status = 'processing', filename = EXCLUDED.filename, bank = EXCLUDED.bank,
       started_at = now(), imported = NULL, already_known = NULL, pending_skipped = NULL
 WHERE uploads.status = 'failed'
    OR (uploads.status = 'processing' AND uploads.started_at < now() - make_interval(secs => $4))
RETURNING ` + recordColumns

const recordColumns = `id::text, sha256, filename, bank, uploaded_at, started_at, status, imported, already_known, pending_skipped`

// Claim implements the claim described on recordStore.
func (p *PostgresRecords) Claim(ctx context.Context, sha256, bank, filename string) (Record, bool, error) {
	var rec Record
	// A second attempt covers the owner failing between our upsert and read:
	// the record is then failed, and claimable.
	for range 2 {
		var err error
		rec, err = scanRecord(p.db.QueryRow(ctx, claimSQL, sha256, filename, bank, staleAfter.Seconds()))
		if err == nil {
			return rec, true, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Record{}, false, fmt.Errorf("postgres: claim upsert: %w", err)
		}
		// The upsert only skips a row that exists, and rows are never deleted.
		if rec, err = p.bySHA256(ctx, sha256); err != nil {
			return Record{}, false, err
		}
		if rec.Status != StatusFailed {
			return rec, false, nil
		}
	}
	return rec, false, nil
}

// Finish marks the claimed upload done with counts, or returns ErrClaimLost.
func (p *PostgresRecords) Finish(ctx context.Context, claim Record, counts ingest.Result) error {
	return p.setStatus(ctx, claim, StatusDone, &counts)
}

// Fail marks the claimed upload failed, or returns ErrClaimLost.
func (p *PostgresRecords) Fail(ctx context.Context, claim Record) error {
	return p.setStatus(ctx, claim, StatusFailed, nil)
}

// setStatus sets the status of claim's upload, and its counts when counts
// isn't nil, only while that claim still holds it: still processing, from
// the same start.
func (p *PostgresRecords) setStatus(ctx context.Context, claim Record, status Status, counts *ingest.Result) error {
	var imported, known, pending *int
	if counts != nil {
		imported, known, pending = &counts.Imported, &counts.AlreadyKnown, &counts.PendingSkipped
	}
	tag, err := p.db.Exec(ctx, `
UPDATE uploads SET status = $2, imported = $3, already_known = $4, pending_skipped = $5
 WHERE id = $1 AND status = 'processing' AND started_at = $6`,
		claim.ID, string(status), imported, known, pending, claim.StartedAt)
	if err != nil {
		return fmt.Errorf("postgres: set %s: %w", status, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("postgres: set %s: %w", status, ErrClaimLost)
	}
	return nil
}

// bySHA256 returns the record of the upload with hex SHA-256 sha256.
func (p *PostgresRecords) bySHA256(ctx context.Context, sha256 string) (Record, error) {
	rec, err := scanRecord(p.db.QueryRow(ctx, `SELECT `+recordColumns+` FROM uploads WHERE sha256 = $1`, sha256))
	if err != nil {
		return Record{}, fmt.Errorf("postgres: read upload record: %w", err)
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
