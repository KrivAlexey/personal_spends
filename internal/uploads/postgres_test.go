package uploads

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KrivAlexey/personal_spends/internal/ingest"
)

var _ recordStore = (*PostgresRecords)(nil)

// newTestRecords connects to TEST_DATABASE_URL (e.g. the compose Postgres:
// postgres://spends:spends@localhost:5432/spends), ensures the schema and
// empties the table. Tests using it are skipped without the variable and must
// not run in parallel.
func newTestRecords(t *testing.T) (*PostgresRecords, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	p := NewPostgresRecords(pool)
	if err := p.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	if _, err := pool.Exec(ctx, "TRUNCATE uploads"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return p, pool
}

func sum(c byte) string { return strings.Repeat(string(c), 64) }

func TestPostgresRecords_EnsureSchemaTwice(t *testing.T) {
	p, _ := newTestRecords(t)
	if err := p.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("second EnsureSchema() error = %v", err)
	}
}

func TestPostgresRecords_ClaimNew(t *testing.T) {
	p, _ := newTestRecords(t)
	ctx := context.Background()
	before := time.Now().Add(-time.Minute) // DB and host clocks may differ a little

	rec, claimed, err := p.Claim(ctx, sum('a'), "sparkasse", "statement.csv")
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if !claimed || rec.ID == "" {
		t.Fatalf("Claim() = %+v, claimed %v; want a claimed record with an id", rec, claimed)
	}

	got, err := p.bySHA256(ctx, sum('a'))
	if err != nil {
		t.Fatalf("bySHA256() error = %v", err)
	}
	if got.ID != rec.ID || got.Bank != "sparkasse" || got.Filename != "statement.csv" ||
		got.SHA256 != sum('a') || got.Status != StatusProcessing || got.Counts != (ingest.Result{}) {
		t.Errorf("record = %+v, want processing sparkasse statement.csv with id %s", got, rec.ID)
	}
	if got.UploadedAt.Before(before) || got.UploadedAt.Location() != time.UTC {
		t.Errorf("UploadedAt = %v, want a recent UTC time", got.UploadedAt)
	}
}

func TestPostgresRecords_ClaimExisting(t *testing.T) {
	counts := ingest.Result{Imported: 5, AlreadyKnown: 3, PendingSkipped: 2}
	tests := []struct {
		name string
		// setup brings the first claim (id) into the state under test.
		setup       func(t *testing.T, p *PostgresRecords, pool *pgxpool.Pool, id string)
		wantClaimed bool
		wantStatus  Status
		wantCounts  ingest.Result
	}{
		{
			name: "done is not reclaimed and carries counts",
			setup: func(t *testing.T, p *PostgresRecords, _ *pgxpool.Pool, id string) {
				if err := p.Finish(context.Background(), claimOf(t, p), counts); err != nil {
					t.Fatalf("Finish() error = %v", err)
				}
			},
			wantStatus: StatusDone,
			wantCounts: counts,
		},
		{
			name:       "fresh processing is not reclaimed",
			setup:      func(*testing.T, *PostgresRecords, *pgxpool.Pool, string) {},
			wantStatus: StatusProcessing,
		},
		{
			name: "failed is reclaimed",
			setup: func(t *testing.T, p *PostgresRecords, _ *pgxpool.Pool, id string) {
				if err := p.Fail(context.Background(), claimOf(t, p)); err != nil {
					t.Fatalf("Fail() error = %v", err)
				}
			},
			wantClaimed: true,
			wantStatus:  StatusProcessing,
		},
		{
			name: "processing older than 15 minutes is reclaimed",
			setup: func(t *testing.T, _ *PostgresRecords, pool *pgxpool.Pool, id string) {
				ageRecord(t, pool, id, 16*time.Minute)
			},
			wantClaimed: true,
			wantStatus:  StatusProcessing,
		},
		{
			name: "processing younger than 15 minutes is not reclaimed",
			setup: func(t *testing.T, _ *PostgresRecords, pool *pgxpool.Pool, id string) {
				ageRecord(t, pool, id, 14*time.Minute)
			},
			wantStatus: StatusProcessing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, pool := newTestRecords(t)
			ctx := context.Background()
			first, _, err := p.Claim(ctx, sum('b'), "sparkasse", "first.csv")
			if err != nil {
				t.Fatalf("first Claim() error = %v", err)
			}
			tt.setup(t, p, pool, first.ID)

			rec, claimed, err := p.Claim(ctx, sum('b'), "dkb", "second.csv")
			if err != nil {
				t.Fatalf("second Claim() error = %v", err)
			}
			if claimed != tt.wantClaimed || rec.ID != first.ID || rec.Status != tt.wantStatus || rec.Counts != tt.wantCounts {
				t.Errorf("second Claim() = %+v, claimed %v; want id %s, claimed %v, status %s, counts %+v",
					rec, claimed, first.ID, tt.wantClaimed, tt.wantStatus, tt.wantCounts)
			}

			got, err := p.bySHA256(ctx, sum('b'))
			if err != nil {
				t.Fatalf("bySHA256() error = %v", err)
			}
			wantBank, wantFile := "sparkasse", "first.csv"
			if tt.wantClaimed {
				wantBank, wantFile = "dkb", "second.csv"
			}
			if got.Bank != wantBank || got.Filename != wantFile || got.Counts != tt.wantCounts {
				t.Errorf("stored = %+v, want bank %s, file %s, counts %+v", got, wantBank, wantFile, tt.wantCounts)
			}
		})
	}
}

func TestPostgresRecords_ConcurrentClaimHasOneOwner(t *testing.T) {
	p, _ := newTestRecords(t)
	const n = 8
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		owners int
		ids    = map[string]bool{}
	)
	for range n {
		wg.Go(func() {
			rec, claimed, err := p.Claim(context.Background(), sum('c'), "sparkasse", "")
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				t.Errorf("Claim() error = %v", err)
				return
			}
			ids[rec.ID] = true
			if claimed {
				owners++
			}
		})
	}
	wg.Wait()
	if owners != 1 || len(ids) != 1 {
		t.Errorf("owners = %d, distinct ids = %d; want 1 and 1", owners, len(ids))
	}
}

func TestPostgresRecords_FinishAndFailUnknownID(t *testing.T) {
	p, _ := newTestRecords(t)
	ctx := context.Background()
	const missing = "00000000-0000-0000-0000-000000000000"
	if err := p.Finish(ctx, Record{ID: missing}, ingest.Result{}); err == nil {
		t.Error("Finish(unknown id) error = nil, want an error")
	}
	if err := p.Fail(ctx, Record{ID: missing}); err == nil {
		t.Error("Fail(unknown id) error = nil, want an error")
	}
}

// A request whose claim went stale and was taken over must not overwrite the
// new owner's record, whether it finishes or fails.
func TestPostgresRecords_StaleOwnerCannotRecordOutcome(t *testing.T) {
	p, pool := newTestRecords(t)
	ctx := context.Background()
	old, _, err := p.Claim(ctx, sum('d'), "sparkasse", "old.csv")
	if err != nil {
		t.Fatalf("first Claim() error = %v", err)
	}
	ageRecord(t, pool, old.ID, 16*time.Minute)
	cur, claimed, err := p.Claim(ctx, sum('d'), "dkb", "new.csv")
	if err != nil || !claimed {
		t.Fatalf("reclaim = claimed %v, error %v; want claimed", claimed, err)
	}

	if err := p.Finish(ctx, old, ingest.Result{Imported: 9}); !errors.Is(err, ErrClaimLost) {
		t.Errorf("stale Finish() error = %v, want ErrClaimLost", err)
	}
	if err := p.Fail(ctx, old); !errors.Is(err, ErrClaimLost) {
		t.Errorf("stale Fail() error = %v, want ErrClaimLost", err)
	}
	got, err := p.bySHA256(ctx, sum('d'))
	if err != nil {
		t.Fatalf("bySHA256() error = %v", err)
	}
	if got.Status != StatusProcessing || got.Bank != "dkb" || got.Counts != (ingest.Result{}) {
		t.Errorf("record = %+v, want the new owner's processing record", got)
	}

	if err := p.Finish(ctx, cur, ingest.Result{Imported: 1}); err != nil {
		t.Errorf("current Finish() error = %v", err)
	}
}

func TestPostgresRecords_List(t *testing.T) {
	p, pool := newTestRecords(t)
	ctx := context.Background()

	empty, err := p.List(ctx, 10)
	if err != nil {
		t.Fatalf("List() on empty table error = %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("List() on empty table = %#v, want an empty non-nil slice", empty)
	}

	// oldest, middle, newest: received an hour apart.
	var claims []Record
	for i, c := range []byte{'e', 'f', 'g'} {
		rec, _, err := p.Claim(ctx, sum(c), "sparkasse", fmt.Sprintf("%d.csv", i))
		if err != nil {
			t.Fatalf("Claim() error = %v", err)
		}
		if _, err := pool.Exec(ctx, "UPDATE uploads SET uploaded_at = now() - make_interval(hours => $2) WHERE id = $1", rec.ID, 3-i); err != nil {
			t.Fatalf("set uploaded_at: %v", err)
		}
		claims = append(claims, rec)
	}
	done := ingest.Result{Imported: 5, AlreadyKnown: 3, PendingSkipped: 2}
	if err := p.Finish(ctx, claims[1], done); err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	if err := p.Fail(ctx, claims[2]); err != nil {
		t.Fatalf("Fail() error = %v", err)
	}

	got, err := p.List(ctx, 2)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List(2) returned %d records, want 2", len(got))
	}
	if got[0].Filename != "2.csv" || got[0].Status != StatusFailed || got[0].Counts != (ingest.Result{}) {
		t.Errorf("first = %+v, want the newest, 2.csv, failed, no counts", got[0])
	}
	if got[1].Filename != "1.csv" || got[1].Status != StatusDone || got[1].Counts != done {
		t.Errorf("second = %+v, want 1.csv, done, counts %+v", got[1], done)
	}
	if !got[0].UploadedAt.After(got[1].UploadedAt) {
		t.Errorf("uploaded_at not newest first: %v then %v", got[0].UploadedAt, got[1].UploadedAt)
	}
}

// claimOf returns the only record, as Claim returned it to its owner.
func claimOf(t *testing.T, p *PostgresRecords) Record {
	t.Helper()
	rec, err := p.bySHA256(context.Background(), sum('b'))
	if err != nil {
		t.Fatalf("bySHA256() error = %v", err)
	}
	return rec
}

// ageRecord moves the record's start back by age, as if its import began then.
func ageRecord(t *testing.T, pool *pgxpool.Pool, id string, age time.Duration) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		"UPDATE uploads SET started_at = now() - make_interval(secs => $2) WHERE id = $1", id, age.Seconds())
	if err != nil {
		t.Fatalf("age record: %v", err)
	}
}
