package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KrivAlexey/personal_spends/internal/ingest"
	"github.com/KrivAlexey/personal_spends/internal/parser"
)

type fakeImporter struct {
	res ingest.Result
	err error

	called  bool
	gotBank string
	gotBody string
}

func (f *fakeImporter) Import(_ context.Context, bankName string, r io.Reader) (ingest.Result, error) {
	f.called = true
	f.gotBank = bankName
	b, err := io.ReadAll(r)
	if err != nil {
		return ingest.Result{}, err
	}
	f.gotBody = string(b)
	return f.res, f.err
}

func uploadRequest(target, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "text/csv")
	return req
}

const csvBody = "Buchungstag;Betrag\n14.08.26;-12,50\n"

func TestUploadCSV(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		body       string
		imp        *fakeImporter
		wantStatus int
		wantCalled bool
		wantJSON   map[string]int // nil: body not checked as JSON
		bodyLacks  string         // must not appear in the response body
	}{
		{
			name:       "imports and reports counts",
			target:     "/uploads/csv?bank=sparkasse",
			body:       csvBody,
			imp:        &fakeImporter{res: ingest.Result{Imported: 5, AlreadyKnown: 3, PendingSkipped: 2}},
			wantStatus: http.StatusOK,
			wantCalled: true,
			wantJSON:   map[string]int{"imported": 5, "already_known": 3, "pending_skipped": 2},
		},
		{
			name:       "nothing new still reports zeros",
			target:     "/uploads/csv?bank=sparkasse",
			body:       csvBody,
			imp:        &fakeImporter{res: ingest.Result{AlreadyKnown: 1}},
			wantStatus: http.StatusOK,
			wantCalled: true,
			wantJSON:   map[string]int{"imported": 0, "already_known": 1, "pending_skipped": 0},
		},
		{
			name:       "missing bank parameter",
			target:     "/uploads/csv",
			body:       csvBody,
			imp:        &fakeImporter{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty bank parameter",
			target:     "/uploads/csv?bank=",
			body:       csvBody,
			imp:        &fakeImporter{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty body",
			target:     "/uploads/csv?bank=sparkasse",
			body:       "",
			imp:        &fakeImporter{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "unknown bank is a client error",
			target: "/uploads/csv?bank=nobank",
			body:   csvBody,
			imp: &fakeImporter{err: fmt.Errorf("ingest: parse nobank export: parser: csv schema is not found for bank: nobank: %w",
				parser.ErrBankMappingNotFound)},
			wantStatus: http.StatusBadRequest,
			wantCalled: true,
		},
		{
			name:   "import failure hides internal error",
			target: "/uploads/csv?bank=sparkasse",
			body:   csvBody,
			// Import returns the partial result of the batches saved before the failure.
			imp: &fakeImporter{
				res: ingest.Result{Imported: 50},
				err: errors.New("ingest: save: dynamodb: secret-table-name throttled"),
			},
			wantStatus: http.StatusInternalServerError,
			wantCalled: true,
			bodyLacks:  "secret-table-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			New(tt.imp).Routes().ServeHTTP(rec, uploadRequest(tt.target, tt.body))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body: %q)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.imp.called != tt.wantCalled {
				t.Errorf("importer called = %v, want %v", tt.imp.called, tt.wantCalled)
			}
			if tt.bodyLacks != "" && strings.Contains(rec.Body.String(), tt.bodyLacks) {
				t.Errorf("body leaks %q: %q", tt.bodyLacks, rec.Body.String())
			}
			if tt.wantStatus != http.StatusOK && strings.Contains(rec.Body.String(), `"imported"`) {
				t.Errorf("error response carries an import result: %q", rec.Body.String())
			}
			if tt.wantJSON == nil {
				return
			}

			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			var got map[string]int
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode body %q: %v", rec.Body.String(), err)
			}
			for k, want := range tt.wantJSON {
				v, ok := got[k]
				if !ok {
					t.Errorf("body lacks key %q: %v", k, got)
				} else if v != want {
					t.Errorf("%s = %d, want %d", k, v, want)
				}
			}
		})
	}
}

// The body must reach the importer byte for byte: Sparkasse exports are
// ISO-8859-1 and the parser decodes them, not the handler.
func TestUploadCSV_PassesBankAndBodyToImporter(t *testing.T) {
	body := "Verwendungszweck\nGrundpreis f\xfcr Kontof\xfchrung\n"
	imp := &fakeImporter{}
	rec := httptest.NewRecorder()
	New(imp).Routes().ServeHTTP(rec, uploadRequest("/uploads/csv?bank=sparkasse", body))

	if imp.gotBank != "sparkasse" {
		t.Errorf("bank = %q, want %q", imp.gotBank, "sparkasse")
	}
	if imp.gotBody != body {
		t.Errorf("body = %q, want %q", imp.gotBody, body)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// A body that fails to read (client dropped mid-upload) is a client error,
// whether it fails on the first byte or partway through.
func TestUploadCSV_UnreadableBody(t *testing.T) {
	tests := []struct {
		name       string
		body       io.Reader
		wantCalled bool
	}{
		{
			name: "fails before the first byte",
			body: errReader{},
		},
		{
			name:       "fails mid-body",
			body:       io.MultiReader(strings.NewReader("Buchungstag;Be"), errReader{}),
			wantCalled: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imp := &fakeImporter{}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/uploads/csv?bank=sparkasse", tt.body)
			New(imp).Routes().ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d (body: %q)", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
			if imp.called != tt.wantCalled {
				t.Errorf("importer called = %v, want %v", imp.called, tt.wantCalled)
			}
		})
	}
}
