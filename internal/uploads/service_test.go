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
	"strings"
	"testing"
	"time"

	"github.com/KrivAlexey/personal_spends/internal/ingest"
	"github.com/KrivAlexey/personal_spends/internal/logging/logtest"
	"github.com/KrivAlexey/personal_spends/internal/parser"
)

type fakeRecords struct {
	// existing is returned, unclaimed, when set; otherwise Claim claims "u1".
	existing  *Record
	claimErr  error
	finishErr error
	failErr   error

	claims       []string // "sha256|bank|filename"
	finished     map[string]ingest.Result
	failed       []string
	failCtxErr   error // ctx.Err() seen by Fail
	finishCtxErr error // ctx.Err() seen by Finish
}

func (f *fakeRecords) Claim(_ context.Context, sum, bank, filename string) (Record, bool, error) {
	f.claims = append(f.claims, sum+"|"+bank+"|"+filename)
	if f.claimErr != nil {
		return Record{}, false, f.claimErr
	}
	if f.existing != nil {
		return *f.existing, false, nil
	}
	return Record{ID: "u1", SHA256: sum, Bank: bank, Filename: filename, Status: StatusProcessing}, true, nil
}

func (f *fakeRecords) Finish(ctx context.Context, claim Record, counts ingest.Result) error {
	id := claim.ID
	f.finishCtxErr = ctx.Err()
	if f.finishErr != nil {
		return f.finishErr
	}
	if f.finished == nil {
		f.finished = map[string]ingest.Result{}
	}
	f.finished[id] = counts
	return nil
}

func (f *fakeRecords) Fail(ctx context.Context, claim Record) error {
	f.failCtxErr = ctx.Err()
	f.failed = append(f.failed, claim.ID)
	return f.failErr
}

type fakeBlobs struct {
	putErr error
	delay  time.Duration // how long Put takes, like a slow S3
	puts   map[string][]byte

	gotDeadline time.Time
	hasDeadline bool
}

func (f *fakeBlobs) Put(ctx context.Context, key string, body []byte) error {
	f.gotDeadline, f.hasDeadline = ctx.Deadline()
	time.Sleep(f.delay)
	if f.putErr != nil {
		return f.putErr
	}
	if f.puts == nil {
		f.puts = map[string][]byte{}
	}
	f.puts[key] = bytes.Clone(body)
	return nil
}

type fakeImporter struct {
	res ingest.Result
	err error
	// onImport, when set, runs with the import's context before returning.
	onImport func(ctx context.Context)

	gotDeadline time.Time
	hasDeadline bool

	called  bool
	gotBank string
	gotBody []byte
}

func (f *fakeImporter) Import(ctx context.Context, bankName string, r io.Reader) (ingest.Result, error) {
	f.gotDeadline, f.hasDeadline = ctx.Deadline()
	if f.onImport != nil {
		f.onImport(ctx)
	}
	f.called = true
	f.gotBank = bankName
	b, err := io.ReadAll(r)
	if err != nil {
		return ingest.Result{}, err
	}
	f.gotBody = b
	return f.res, f.err
}

var (
	body    = []byte("Buchungstag;Betrag\n14.08.26;-12,50\n")
	bodySum = func() string { s := sha256.Sum256(body); return hex.EncodeToString(s[:]) }()
	bodyKey = "csv/" + bodySum + ".csv"
	counts  = ingest.Result{Imported: 5, AlreadyKnown: 3, PendingSkipped: 2}

	errClaim  = errors.New("claim failed")
	errPut    = errors.New("put failed")
	errImport = errors.New("import failed")
	errFinish = errors.New("finish failed")
	errFail   = errors.New("fail failed")
)

func TestService_Upload(t *testing.T) {
	tests := []struct {
		name    string
		records *fakeRecords
		blobs   *fakeBlobs
		imp     *fakeImporter

		want         Result
		wantErrs     []error // each must match errors.Is; nil: no error
		wantPut      bool
		wantImport   bool
		wantFinished bool
		wantFailed   bool
	}{
		{
			name:         "new upload is kept, imported and finished",
			records:      &fakeRecords{},
			blobs:        &fakeBlobs{},
			imp:          &fakeImporter{res: counts},
			want:         Result{UploadID: "u1", Result: counts},
			wantPut:      true,
			wantImport:   true,
			wantFinished: true,
		},
		{
			name: "done repeat is answered from the record",
			records: &fakeRecords{existing: &Record{
				ID: "u0", SHA256: bodySum, Status: StatusDone, Counts: ingest.Result{Imported: 7, PendingSkipped: 1},
			}},
			blobs: &fakeBlobs{},
			imp:   &fakeImporter{res: counts},
			want:  Result{UploadID: "u0", Result: ingest.Result{Imported: 7, PendingSkipped: 1}},
		},
		{
			name:     "repeat while processing is in progress",
			records:  &fakeRecords{existing: &Record{ID: "u0", SHA256: bodySum, Status: StatusProcessing}},
			blobs:    &fakeBlobs{},
			imp:      &fakeImporter{res: counts},
			want:     Result{UploadID: "u0"},
			wantErrs: []error{ErrInProgress},
		},
		{
			name:     "claim error stops everything",
			records:  &fakeRecords{claimErr: errClaim},
			blobs:    &fakeBlobs{},
			imp:      &fakeImporter{res: counts},
			wantErrs: []error{errClaim},
		},
		{
			name:       "put error marks failed and skips import",
			records:    &fakeRecords{},
			blobs:      &fakeBlobs{putErr: errPut},
			imp:        &fakeImporter{res: counts},
			want:       Result{UploadID: "u1"},
			wantErrs:   []error{errPut},
			wantFailed: true,
		},
		{
			name:    "import error marks failed and keeps partial counts",
			records: &fakeRecords{},
			blobs:   &fakeBlobs{},
			// Import returns the batches saved before the failure.
			imp:        &fakeImporter{res: ingest.Result{Imported: 50}, err: errImport},
			want:       Result{UploadID: "u1", Result: ingest.Result{Imported: 50}},
			wantErrs:   []error{errImport},
			wantPut:    true,
			wantImport: true,
			wantFailed: true,
		},
		{
			name:    "unknown bank marks failed and stays detectable",
			records: &fakeRecords{},
			blobs:   &fakeBlobs{},
			imp: &fakeImporter{err: fmt.Errorf("ingest: parse export: %w",
				parser.ErrBankMappingNotFound)},
			want:       Result{UploadID: "u1"},
			wantErrs:   []error{parser.ErrBankMappingNotFound},
			wantPut:    true,
			wantImport: true,
			wantFailed: true,
		},
		{
			name:       "fail error is joined to the import error",
			records:    &fakeRecords{failErr: errFail},
			blobs:      &fakeBlobs{},
			imp:        &fakeImporter{err: errImport},
			want:       Result{UploadID: "u1"},
			wantErrs:   []error{errImport, errFail},
			wantPut:    true,
			wantImport: true,
			wantFailed: true,
		},
		{
			name:       "finish error is returned",
			records:    &fakeRecords{finishErr: errFinish},
			blobs:      &fakeBlobs{},
			imp:        &fakeImporter{res: counts},
			want:       Result{UploadID: "u1", Result: counts},
			wantErrs:   []error{errFinish},
			wantPut:    true,
			wantImport: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := New(tt.records, tt.blobs, tt.imp, nil)
			got, err := svc.Upload(context.Background(), Request{Bank: "sparkasse", Filename: "statement.csv", Body: body})

			if len(tt.wantErrs) == 0 && err != nil {
				t.Fatalf("Upload() error = %v, want nil", err)
			}
			for _, want := range tt.wantErrs {
				if !errors.Is(err, want) {
					t.Errorf("Upload() error = %v, want errors.Is %v", err, want)
				}
			}
			if got != tt.want {
				t.Errorf("Upload() = %+v, want %+v", got, tt.want)
			}

			wantClaim := bodySum + "|sparkasse|statement.csv"
			if len(tt.records.claims) != 1 || tt.records.claims[0] != wantClaim {
				t.Errorf("claims = %q, want [%q]", tt.records.claims, wantClaim)
			}

			put, isPut := tt.blobs.puts[bodyKey]
			if isPut != tt.wantPut || len(tt.blobs.puts) > 1 {
				t.Errorf("puts = %v keys, want put of %q: %v", len(tt.blobs.puts), bodyKey, tt.wantPut)
			}
			if isPut && !bytes.Equal(put, body) {
				t.Errorf("put body = %q, want %q", put, body)
			}

			if tt.imp.called != tt.wantImport {
				t.Errorf("import called = %v, want %v", tt.imp.called, tt.wantImport)
			}
			if tt.imp.called && (tt.imp.gotBank != "sparkasse" || !bytes.Equal(tt.imp.gotBody, body)) {
				t.Errorf("import got bank %q body %q, want sparkasse and the request body", tt.imp.gotBank, tt.imp.gotBody)
			}

			fin, isFinished := tt.records.finished["u1"]
			if isFinished != tt.wantFinished {
				t.Errorf("finished = %v, want %v", tt.records.finished, tt.wantFinished)
			}
			if isFinished && fin != counts {
				t.Errorf("finished counts = %+v, want %+v", fin, counts)
			}

			wantFailed := []string(nil)
			if tt.wantFailed {
				wantFailed = []string{"u1"}
			}
			if fmt.Sprint(tt.records.failed) != fmt.Sprint(wantFailed) {
				t.Errorf("failed = %v, want %v", tt.records.failed, wantFailed)
			}
		})
	}
}

// A client that disconnects mid-import cancels the request context; the
// record must still be marked failed, or the hash answers 409 for 15 minutes.
func TestService_Upload_FailOutlivesCancelledRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	records := &fakeRecords{}
	svc := New(records, &fakeBlobs{}, &fakeImporter{err: context.Canceled}, nil)

	_, err := svc.Upload(ctx, Request{Bank: "sparkasse", Body: body})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Upload() error = %v, want context.Canceled", err)
	}
	if len(records.failed) != 1 {
		t.Fatalf("failed = %v, want [u1]", records.failed)
	}
	if records.failCtxErr != nil {
		t.Errorf("Fail saw ctx.Err() = %v, want a live context", records.failCtxErr)
	}
}

// The import finished, then the client went away: the outcome must still be
// recorded, or the hash answers 409 until the claim goes stale.
func TestService_Upload_FinishOutlivesCancelledRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	records := &fakeRecords{}
	imp := &fakeImporter{res: counts, onImport: func(context.Context) { cancel() }}

	if _, err := New(records, &fakeBlobs{}, imp, nil).Upload(ctx, Request{Bank: "sparkasse", Body: body}); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if _, ok := records.finished["u1"]; !ok {
		t.Fatal("Finish not called")
	}
	if records.finishCtxErr != nil {
		t.Errorf("Finish saw ctx.Err() = %v, want a live context", records.finishCtxErr)
	}
}

// The import must end before its claim can go stale, or a repeat could
// reclaim the upload while it still runs.
func TestService_Upload_ImportEndsBeforeClaimIsStale(t *testing.T) {
	imp := &fakeImporter{res: counts}
	start := time.Now()

	if _, err := New(&fakeRecords{}, &fakeBlobs{}, imp, nil).Upload(context.Background(), Request{Bank: "sparkasse", Body: body}); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if !imp.hasDeadline {
		t.Fatal("import context has no deadline")
	}
	if limit := start.Add(staleAfter - time.Minute); imp.gotDeadline.After(limit) {
		t.Errorf("import deadline %v after start, want at least a minute before staleAfter (%v)",
			imp.gotDeadline.Sub(start).Round(time.Second), staleAfter)
	}
}

// The claim goes stale staleAfter after it starts, so the S3 put and the
// import together must end before then: one deadline for both, set before the
// put, not a fresh import deadline once a slow put returns.
func TestService_Upload_PutAndImportShareOneDeadline(t *testing.T) {
	blobs := &fakeBlobs{delay: 20 * time.Millisecond}
	imp := &fakeImporter{res: counts}
	start := time.Now()

	if _, err := New(&fakeRecords{}, blobs, imp, nil).Upload(context.Background(), Request{Bank: "sparkasse", Body: body}); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if !blobs.hasDeadline {
		t.Fatal("put context has no deadline")
	}
	if limit := start.Add(staleAfter - time.Minute); blobs.gotDeadline.After(limit) {
		t.Errorf("put deadline %v after start, want at least a minute before staleAfter (%v)",
			blobs.gotDeadline.Sub(start).Round(time.Second), staleAfter)
	}
	if !imp.gotDeadline.Equal(blobs.gotDeadline) {
		t.Errorf("import deadline is %v after the put's, want the same deadline",
			imp.gotDeadline.Sub(blobs.gotDeadline))
	}
}

// A failed S3 put is logged at ERROR by the handler with its whole chain,
// so the chain must not carry the hash (design decision 9).
func TestService_Upload_ErrorOmitsHash(t *testing.T) {
	_, err := New(&fakeRecords{}, NewS3Blobs(&fakeS3{err: errors.New("access denied")}, "bucket"), &fakeImporter{}, nil).
		Upload(context.Background(), Request{Bank: "sparkasse", Body: body})

	if err == nil {
		t.Fatal("Upload() error = nil, want the put error")
	}
	if strings.Contains(err.Error(), bodySum) {
		t.Errorf("error %q carries the body hash", err)
	}
}

func TestService_Upload_DebugRecords(t *testing.T) {
	tests := []struct {
		name    string
		records *fakeRecords
		msg     string
		fields  map[string]any
	}{
		{
			name:    "claimed",
			records: &fakeRecords{},
			msg:     "upload claimed",
			fields:  map[string]any{"upload_id": "u1"},
		},
		{
			name:    "repeated",
			records: &fakeRecords{existing: &Record{ID: "u0", Status: StatusDone}},
			msg:     "upload repeated",
			fields:  map[string]any{"upload_id": "u0", "status": "done"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, buf := logtest.NewLogger(slog.LevelDebug)
			svc := New(tt.records, &fakeBlobs{}, &fakeImporter{}, logger)

			if _, err := svc.Upload(context.Background(), Request{Bank: "sparkasse", Filename: "secret-name.csv", Body: body}); err != nil {
				t.Fatalf("Upload() error = %v", err)
			}

			recs := logtest.Filter(logtest.Records(t, buf), "DEBUG", tt.msg)
			if len(recs) != 1 {
				t.Fatalf("got %d %q records, want 1; log:\n%s", len(recs), tt.msg, buf.String())
			}
			for k, v := range tt.fields {
				if recs[0][k] != v {
					t.Errorf("record[%q] = %v, want %v", k, recs[0][k], v)
				}
			}
			if bytes.Contains([]byte(buf.String()), []byte("secret-name")) || bytes.Contains([]byte(buf.String()), []byte(bodySum)) {
				t.Errorf("log carries the file name or hash:\n%s", buf.String())
			}
		})
	}
}
