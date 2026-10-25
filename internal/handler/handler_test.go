package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/KrivAlexey/personal_spends/internal/categorizer"
	"github.com/KrivAlexey/personal_spends/internal/ingest"
	"github.com/KrivAlexey/personal_spends/internal/logging"
	"github.com/KrivAlexey/personal_spends/internal/logging/logtest"
	"github.com/KrivAlexey/personal_spends/internal/parser"
	"github.com/KrivAlexey/personal_spends/internal/storage"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type fakeImporter struct {
	res ingest.Result
	err error
	// onImport, when set, runs after the body is read and its error replaces err.
	onImport func(ctx context.Context) error

	called  bool
	gotBank string
	gotBody string
}

func (f *fakeImporter) Import(ctx context.Context, bankName string, r io.Reader) (ingest.Result, error) {
	f.called = true
	f.gotBank = bankName
	b, err := io.ReadAll(r)
	if err != nil {
		return ingest.Result{}, err
	}
	f.gotBody = string(b)
	if f.onImport != nil {
		return f.res, f.onImport(ctx)
	}
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
			New(tt.imp, nil).Routes().ServeHTTP(rec, uploadRequest(tt.target, tt.body))

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
	New(imp, nil).Routes().ServeHTTP(rec, uploadRequest("/uploads/csv?bank=sparkasse", body))

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
			New(imp, nil).Routes().ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d (body: %q)", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
			if imp.called != tt.wantCalled {
				t.Errorf("importer called = %v, want %v", imp.called, tt.wantCalled)
			}
		})
	}
}

func bankTarget(bank string) string {
	return "/uploads/csv?" + url.Values{"bank": {bank}}.Encode()
}

func TestUploadCSV_ErrorRecord(t *testing.T) {
	tests := []struct {
		name      string
		ctx       func() (context.Context, context.CancelFunc)
		onImport  func(ctx context.Context, cancel context.CancelFunc) error
		wantLevel string
		wantErr   string
	}{
		{
			name: "storage failure",
			onImport: func(context.Context, context.CancelFunc) error {
				return errors.New("ingest: save: write chunk of expenses: batch write item: throttled")
			},
			wantLevel: "ERROR",
			wantErr:   "ingest: save: write chunk of expenses: batch write item: throttled",
		},
		{
			name: "categorizer failure",
			onImport: func(context.Context, context.CancelFunc) error {
				return errors.New("ingest: categorize: categorizer: create message: 529 overloaded")
			},
			wantLevel: "ERROR",
			wantErr:   "categorizer: create message: 529 overloaded",
		},
		{
			name: "client disconnects during categorization",
			onImport: func(ctx context.Context, cancel context.CancelFunc) error {
				cancel()
				return fmt.Errorf("ingest: categorize: categorizer: create message: %w", ctx.Err())
			},
			wantLevel: "WARN",
			wantErr:   "context canceled",
		},
		{
			name: "timeout stays an error",
			onImport: func(context.Context, context.CancelFunc) error {
				return fmt.Errorf("ingest: categorize: %w", context.DeadlineExceeded)
			},
			wantLevel: "ERROR",
			wantErr:   "context deadline exceeded",
		},
		{
			name: "internal cancellation with the request still live",
			onImport: func(context.Context, context.CancelFunc) error {
				return fmt.Errorf("ingest: categorize: %w", context.Canceled)
			},
			wantLevel: "ERROR",
			wantErr:   "context canceled",
		},
		{
			name: "internal cancellation after the request deadline expired",
			ctx: func() (context.Context, context.CancelFunc) {
				return context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			},
			onImport: func(context.Context, context.CancelFunc) error {
				return fmt.Errorf("ingest: categorize: %w", context.Canceled)
			},
			wantLevel: "ERROR",
			wantErr:   "context canceled",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newCtx := tt.ctx
			if newCtx == nil {
				newCtx = func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) }
			}
			ctx, cancel := newCtx()
			defer cancel()
			imp := &fakeImporter{
				res:      ingest.Result{Imported: 50, AlreadyKnown: 3, PendingSkipped: 2},
				onImport: func(ctx context.Context) error { return tt.onImport(ctx, cancel) },
			}
			h, buf := loggedHandler(imp, slog.LevelInfo)
			rec := httptest.NewRecorder()
			h.Routes().ServeHTTP(rec, uploadRequest("/uploads/csv?bank=sparkasse", csvBody).WithContext(ctx))

			if rec.Code != http.StatusInternalServerError || strings.TrimSpace(rec.Body.String()) != "internal error" {
				t.Errorf("response = %d %q, want 500 internal error", rec.Code, rec.Body.String())
			}
			recs := logtest.Records(t, buf)
			var failures []logtest.Record
			for _, r := range recs {
				if r.Level() == "ERROR" || r.Level() == "WARN" {
					failures = append(failures, r)
				}
			}
			if len(failures) != 1 || failures[0].Level() != tt.wantLevel {
				t.Fatalf("want exactly one %s record, got:\n%s", tt.wantLevel, buf)
			}
			assertFields(t, failures[0], map[string]any{
				"method": "POST", "route": "POST /uploads/csv", "bank": "sparkasse",
				"imported": 50.0, "already_known": 3.0, "pending_skipped": 2.0,
			})
			if e, _ := failures[0]["error"].(string); !strings.Contains(e, tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", e, tt.wantErr)
			}
		})
	}
}

func TestUploadCSV_RejectionRecord(t *testing.T) {
	notFound := fmt.Errorf("ingest: parse: %w", parser.ErrBankMappingNotFound)
	long := strings.Repeat("b", 10_000)
	tests := []struct {
		name       string
		target     string
		body       io.Reader
		err        error
		wantBank   any // nil: no bank field
		wantReason string
		lacks      []string
	}{
		{name: "missing bank", target: "/uploads/csv", body: strings.NewReader(csvBody), wantReason: "missing bank parameter"},
		{name: "empty body", target: bankTarget("sparkasse"), body: strings.NewReader(""), wantBank: "sparkasse", wantReason: "empty body"},
		{name: "unreadable body", target: bankTarget("sparkasse"), body: errReader{}, wantBank: "sparkasse", wantReason: "unreadable body"},
		{
			name: "unknown bank", target: bankTarget("nobank"), body: strings.NewReader(csvBody), err: notFound,
			wantBank: "nobank", wantReason: "bank mapping not found",
		},
		{
			name: "oversized bank", target: bankTarget(long), body: strings.NewReader(csvBody), err: notFound,
			wantBank: strings.Repeat("b", 64) + "…", wantReason: "bank mapping not found", lacks: []string{strings.Repeat("b", 65)},
		},
		{
			name: "oversized multi-byte bank", target: bankTarget(strings.Repeat("ä", 100)), body: strings.NewReader(csvBody), err: notFound,
			wantBank: strings.Repeat("ä", 64) + "…", wantReason: "bank mapping not found", lacks: []string{"�", strings.Repeat("ä", 65)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, buf := loggedHandler(&fakeImporter{err: tt.err}, slog.LevelInfo)
			rec := httptest.NewRecorder()
			h.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, tt.target, tt.body))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
			warns := logtest.Filter(logtest.Records(t, buf), "WARN", "")
			if len(warns) != 1 {
				t.Fatalf("got %d WARN records, want 1:\n%s", len(warns), buf)
			}
			assertFields(t, warns[0], map[string]any{"method": "POST", "route": "POST /uploads/csv", "bank": tt.wantBank, "reason": tt.wantReason})
			if reason, _ := warns[0]["reason"].(string); tt.wantBank != nil && strings.Contains(reason, "nobank") {
				t.Errorf("reason %q contains caller input", reason)
			}
			for _, v := range tt.lacks {
				if strings.Contains(buf.String(), v) {
					t.Errorf("log output contains %q", v)
				}
			}
		})
	}
}

// A bank value can't forge a record: it stays escaped inside one field.
func TestUploadCSV_BankCannotForgeRecord(t *testing.T) {
	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			var buf bytes.Buffer
			logger, err := logging.New(&buf, "info", format)
			if err != nil {
				t.Fatalf("logging.New: %v", err)
			}
			imp := &fakeImporter{err: fmt.Errorf("ingest: %w", parser.ErrBankMappingNotFound)}
			New(imp, logger).Routes().ServeHTTP(httptest.NewRecorder(),
				uploadRequest(bankTarget("x\nlevel=ERROR msg=forged"), csvBody))

			lines := slices.Collect(strings.Lines(buf.String()))
			if len(lines) != 2 {
				t.Fatalf("got %d lines, want the WARN and the access record:\n%s", len(lines), buf.String())
			}
			warns := 0
			for _, l := range lines {
				if strings.HasPrefix(l, "level=ERROR") || strings.HasPrefix(l, "msg=forged") {
					t.Errorf("forged line: %q", l)
				}
				if strings.Contains(l, "level=WARN") || strings.Contains(l, `"level":"WARN"`) {
					warns++
				}
			}
			if warns != 1 {
				t.Errorf("got %d WARN records, want 1:\n%s", warns, buf.String())
			}
		})
	}
}

func TestUploadCSV_DefaultLevelStaysQuiet(t *testing.T) {
	h, buf := loggedHandler(&fakeImporter{res: ingest.Result{Imported: 1}}, slog.LevelInfo)
	h.Routes().ServeHTTP(httptest.NewRecorder(), uploadRequest("/uploads/csv?bank=sparkasse", csvBody))

	recs := logtest.Records(t, buf)
	if len(recs) != 1 || recs[0].Msg() != "request" {
		t.Errorf("want only the access record, got:\n%s", buf)
	}
}

// testMapping parses testExport: a made-up bank with fake data.
var testMapping = parser.BankMapping{
	BankName:         "testbank",
	DateFormat:       "02.01.2006",
	Delimiter:        ";",
	DecimalSeparator: ",",
	IdentityColumns:  []string{"Datum", "Empfaenger", "Betrag", "Text"},
	Mappings:         map[string]string{"Datum": "Date", "Empfaenger": "Merchant", "Betrag": "Amount", "Waehrung": "Currency", "Text": "Description"},
}

const testExport = "Datum;Empfaenger;Betrag;Waehrung;Text\n12.03.2026;REWE Markt GmbH;-23,45;EUR;Kartenzahlung 12.03\n"

type staticMappings map[string]parser.BankMapping

func (m staticMappings) GetMapping(_ context.Context, bank string) (parser.BankMapping, error) {
	mapping, ok := m[bank]
	if !ok {
		return parser.BankMapping{}, parser.ErrBankMappingNotFound
	}
	return mapping, nil
}

type memStore struct{}

func (memStore) KnownIDs(context.Context, []storage.ExpenseKey) (map[string]bool, error) {
	return map[string]bool{}, nil
}

func (memStore) SaveExpenses(context.Context, []storage.Expense) error { return nil }

// realStack wires the real parser, importer and Claude categorizer, pointed at
// the fake Messages API api, with a fake store; logs go to the returned buffer
// at DEBUG.
func realStack(t *testing.T, apiKey string, api http.HandlerFunc) (http.Handler, *logtest.Buffer) {
	t.Helper()
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	logger, buf := logtest.NewLogger(slog.LevelDebug)
	claude := categorizer.NewClaude(apiKey, []string{"groceries"}, logger, option.WithBaseURL(srv.URL), option.WithMaxRetries(0))
	imp := ingest.NewImporter(parser.NewParser(staticMappings{"testbank": testMapping}), claude, memStore{}, 0, logger)
	return New(imp, logger).Routes(), buf
}

func TestUploadCSV_AnthropicKeyNotLogged(t *testing.T) {
	const key = "sentinel-anthropic-key"
	var gotKey string
	routes, buf := realStack(t, key, func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Api-Key")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"type": "error", "error": {"type": "authentication_error", "message": "invalid x-api-key"}}`))
	})
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, uploadRequest(bankTarget("testbank"), testExport))

	if gotKey != key {
		t.Fatalf("fake API got X-Api-Key %q, want the sentinel (the key wasn't in play)", gotKey)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if n := len(logtest.Filter(logtest.Records(t, buf), "ERROR", "")); n != 1 {
		t.Errorf("got %d ERROR records, want 1:\n%s", n, buf)
	}
	logtest.AssertLacks(t, buf, key)
}

func TestUploadCSV_BankDataNotLogged(t *testing.T) {
	okAPI := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id": "msg_test", "type": "message", "role": "assistant",
			"model": "claude-haiku-4-5-20251001", "stop_reason": "tool_use",
			"usage": {"input_tokens": 812, "output_tokens": 140},
			"content": [{"type": "tool_use", "id": "toolu_test", "name": "categorize_batch",
				"input": {"results": [{"index": 0, "category": "groceries", "confidence": 0.9}]}}]}`))
	}
	tests := []struct {
		name       string
		export     string
		wantStatus int
		lacks      []string
		wantError  string // substring of the ERROR record's error; "" = no ERROR
	}{
		{
			name:       "debug upload of a real-looking export",
			export:     testExport,
			wantStatus: http.StatusOK,
			lacks:      []string{"REWE", "Kartenzahlung", "23,45", "23.45"},
		},
		{
			name:       "malformed cell value",
			export:     strings.Replace(testExport, "-23,45", "-12,3x", 1),
			wantStatus: http.StatusInternalServerError,
			lacks:      []string{"REWE", "Kartenzahlung", "12,3x", "12.3x"},
			wantError:  "Amount",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routes, buf := realStack(t, "test-key", okAPI)
			rec := httptest.NewRecorder()
			routes.ServeHTTP(rec, uploadRequest(bankTarget("testbank"), tt.export))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			recs := logtest.Records(t, buf)
			if len(recs) == 0 {
				t.Fatal("no log records")
			}
			id, _ := recs[0]["request_id"].(string)
			for _, r := range recs {
				if r["request_id"] != id || id == "" {
					t.Errorf("record %q has request_id %v, want %q on every record", r.Msg(), r["request_id"], id)
				}
			}
			if tt.wantError != "" {
				errs := logtest.Filter(recs, "ERROR", "")
				if len(errs) != 1 {
					t.Fatalf("got %d ERROR records, want 1:\n%s", len(errs), buf)
				}
				if e, _ := errs[0]["error"].(string); !strings.Contains(e, tt.wantError) {
					t.Errorf("error = %q, want it to name %q", e, tt.wantError)
				}
			} else if len(logtest.Filter(recs, "DEBUG", "")) == 0 {
				t.Errorf("no DEBUG records at DEBUG level:\n%s", buf)
			}
			logtest.AssertLacks(t, buf, tt.lacks...)
		})
	}
}
