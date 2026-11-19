// Package uploads keeps every uploaded bank export and a record of what
// happened to it: the file goes to a blob store under its SHA-256, the record
// (status, counts) to a record store, and the SHA-256 makes a repeated upload
// of the same bytes idempotent.
package uploads

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/KrivAlexey/personal_spends/internal/ingest"
)

// Status is where an upload's import stands.
type Status string

const (
	StatusProcessing Status = "processing"
	StatusDone       Status = "done"
	StatusFailed     Status = "failed"
)

// ErrInProgress means an upload of the same bytes is being imported right now.
var ErrInProgress = errors.New("uploads: upload in progress")

// Request is one upload: the raw export and what the caller said about it.
type Request struct {
	Bank     string
	Filename string // empty when the caller gave none
	Body     []byte
}

// Result is the outcome of an upload: its id and the import's counts.
type Result struct {
	UploadID string
	ingest.Result
}

// Record is one stored upload.
type Record struct {
	ID         string
	SHA256     string // hex
	Filename   string
	Bank       string
	UploadedAt time.Time
	Status     Status
	Counts     ingest.Result // set only when Status is StatusDone
}

type recordStore interface {
	// Claim records an upload of sha256 as processing and reports claimed if
	// the caller now owns its import. If not, rec is the existing record,
	// either done or still processing.
	Claim(ctx context.Context, sha256, bank, filename string) (rec Record, claimed bool, err error)
	Finish(ctx context.Context, id string, counts ingest.Result) error
	Fail(ctx context.Context, id string) error
}

type blobStore interface {
	Put(ctx context.Context, key string, body []byte) error
}

type importer interface {
	Import(ctx context.Context, bankName string, r io.Reader) (ingest.Result, error)
}

// Service runs an upload: claim, keep the file, import, record the outcome.
type Service struct {
	records recordStore
	blobs   blobStore
	imp     importer
	logger  *slog.Logger
}

// New returns a Service; a nil logger discards.
func New(records recordStore, blobs blobStore, imp importer, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Service{records: records, blobs: blobs, imp: imp, logger: logger}
}

// finalizeTimeout bounds the status write after an import, which runs even
// when the request is gone.
const finalizeTimeout = 5 * time.Second

// Upload keeps req.Body and imports it, unless the same bytes were uploaded
// before: a done upload is answered from its record, one still processing
// gives ErrInProgress. On an import error the partial Result is returned with
// it, and the error keeps the importer's chain (parser.ErrBankMappingNotFound).
func (s *Service) Upload(ctx context.Context, req Request) (Result, error) {
	digest := sha256.Sum256(req.Body)
	sum := hex.EncodeToString(digest[:])

	rec, claimed, err := s.records.Claim(ctx, sum, req.Bank, req.Filename)
	if err != nil {
		return Result{}, fmt.Errorf("uploads: claim: %w", err)
	}
	if !claimed {
		s.logger.DebugContext(ctx, "upload repeated", "upload_id", rec.ID, "status", string(rec.Status))
		if rec.Status == StatusDone {
			return Result{UploadID: rec.ID, Result: rec.Counts}, nil
		}
		return Result{UploadID: rec.ID}, ErrInProgress
	}
	s.logger.DebugContext(ctx, "upload claimed", "upload_id", rec.ID)

	if err := s.blobs.Put(ctx, blobKey(sum), req.Body); err != nil {
		return Result{UploadID: rec.ID}, s.fail(ctx, rec.ID, fmt.Errorf("uploads: keep file: %w", err))
	}

	res, err := s.imp.Import(ctx, req.Bank, bytes.NewReader(req.Body))
	if err != nil {
		return Result{UploadID: rec.ID, Result: res}, s.fail(ctx, rec.ID, fmt.Errorf("uploads: %w", err))
	}

	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalizeTimeout)
	defer cancel()
	if err := s.records.Finish(fctx, rec.ID, res); err != nil {
		return Result{UploadID: rec.ID, Result: res}, fmt.Errorf("uploads: finish: %w", err)
	}
	return Result{UploadID: rec.ID, Result: res}, nil
}

// fail marks the upload failed, on a context that outlives the request so a
// client disconnect can't leave it processing, and returns cause joined with
// any error from doing so.
func (s *Service) fail(ctx context.Context, id string, cause error) error {
	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalizeTimeout)
	defer cancel()
	if err := s.records.Fail(fctx, id); err != nil {
		return errors.Join(cause, fmt.Errorf("uploads: mark failed: %w", err))
	}
	return cause
}

// blobKey is where the file with hex SHA-256 sum is kept.
func blobKey(sum string) string {
	return "csv/" + sum + ".csv"
}
