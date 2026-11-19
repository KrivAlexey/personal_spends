// Package uploads keeps every uploaded bank export and a record of what
// happened to it: the file goes to a blob store under its SHA-256, the record
// (status, counts) to a record store, and the SHA-256 makes a repeated upload
// of the same bytes idempotent.
package uploads

import (
	"context"
	"errors"
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

// Upload is not implemented yet.
func (s *Service) Upload(ctx context.Context, req Request) (Result, error) {
	return Result{}, errors.New("uploads: not implemented")
}
