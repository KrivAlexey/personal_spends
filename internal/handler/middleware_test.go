package handler

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/KrivAlexey/personal_spends/internal/logging/logtest"
)

// loggedHandler returns a Handler over imp that logs JSON at level into the
// returned buffer.
func loggedHandler(imp importer, level slog.Level) (*Handler, *logtest.Buffer) {
	logger, buf := logtest.NewLogger(level)
	return New(imp, logger), buf
}

// accessRecord returns the only INFO access record, failing otherwise.
func accessRecord(t *testing.T, recs []logtest.Record) logtest.Record {
	t.Helper()
	access := logtest.Filter(recs, "INFO", "request")
	if len(access) != 1 {
		t.Fatalf("got %d access records, want 1: %v", len(access), recs)
	}
	return access[0]
}

func assertFields(t *testing.T, rec logtest.Record, want map[string]any) {
	t.Helper()
	for k, v := range want {
		if rec[k] != v {
			t.Errorf("%s = %v, want %v", k, rec[k], v)
		}
	}
}

func TestAccessRecord(t *testing.T) {
	tests := []struct {
		name string
		req  *http.Request
		imp  *fakeImporter
		want map[string]any
	}{
		{
			name: "successful upload",
			req:  uploadRequest("/uploads/csv?bank=sparkasse", csvBody),
			imp:  &fakeImporter{},
			want: map[string]any{"method": "POST", "route": "POST /uploads/csv", "status": 200.0},
		},
		{
			name: "failed upload",
			req:  uploadRequest("/uploads/csv?bank=sparkasse", csvBody),
			imp:  &fakeImporter{err: io.ErrClosedPipe},
			want: map[string]any{"method": "POST", "route": "POST /uploads/csv", "status": 500.0},
		},
		{
			name: "rejected upload",
			req:  uploadRequest("/uploads/csv", csvBody),
			imp:  &fakeImporter{},
			want: map[string]any{"method": "POST", "route": "POST /uploads/csv", "status": 400.0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, buf := loggedHandler(tt.imp, slog.LevelInfo)
			h.Routes().ServeHTTP(httptest.NewRecorder(), tt.req)

			access := accessRecord(t, logtest.Records(t, buf))
			assertFields(t, access, tt.want)
			if d, ok := access["duration_ms"].(float64); !ok || d < 0 {
				t.Errorf("duration_ms = %v, want non-negative", access["duration_ms"])
			}
			logtest.AssertLacks(t, buf, "bank=sparkasse", "?bank", "Buchungstag")
		})
	}
}

func TestAccessRecord_UnknownPathIsNotLogged(t *testing.T) {
	path := "/REWE-23,45" + strings.Repeat("A", 100*1024)
	h, buf := loggedHandler(&fakeImporter{}, slog.LevelDebug)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	assertFields(t, accessRecord(t, logtest.Records(t, buf)), map[string]any{"method": "GET", "route": "unmatched", "status": 404.0})
	logtest.AssertLacks(t, buf, "REWE", "23,45", "AAAA")
}

var requestIDFormat = regexp.MustCompile(`^[0-9a-f]{16}$`)

func TestRequestID(t *testing.T) {
	imp := &fakeImporter{err: io.ErrClosedPipe}
	h, buf := loggedHandler(imp, slog.LevelInfo)
	routes := h.Routes()
	routes.ServeHTTP(httptest.NewRecorder(), uploadRequest("/uploads/csv?bank=sparkasse", csvBody))
	routes.ServeHTTP(httptest.NewRecorder(), uploadRequest("/uploads/csv?bank=sparkasse", csvBody))

	recs := logtest.Records(t, buf)
	if len(recs) != 4 {
		t.Fatalf("got %d records, want 2 per request:\n%s", len(recs), buf)
	}
	ids := make([]string, len(recs))
	for i, r := range recs {
		ids[i], _ = r["request_id"].(string)
		if !requestIDFormat.MatchString(ids[i]) {
			t.Errorf("request_id = %q, want 16 hex characters", ids[i])
		}
	}
	if ids[0] != ids[1] || ids[2] != ids[3] {
		t.Errorf("records of one request have different IDs: %v", ids)
	}
	if ids[0] == ids[2] {
		t.Errorf("two requests share request_id %q", ids[0])
	}
}

// panicServer serves h's middleware around a mux with test routes, plus the
// real upload route over imp.
func panicServer(t *testing.T, imp importer) (*httptest.Server, *logtest.Buffer) {
	t.Helper()
	h, buf := loggedHandler(imp, slog.LevelInfo)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /uploads/csv", h.uploadCSV)
	mux.HandleFunc("GET /after-write", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("partial"))
		_ = http.NewResponseController(w).Flush()
		panic("boom REWE")
	})
	mux.HandleFunc("GET /abort", func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	})
	srv := httptest.NewServer(h.withLogging(mux))
	t.Cleanup(srv.Close)
	return srv, buf
}

func TestPanic_BeforeResponse(t *testing.T) {
	imp := &fakeImporter{onImport: func(context.Context) error { panic("boom REWE") }}
	srv, buf := panicServer(t, imp)

	resp, err := http.Post(srv.URL+"/uploads/csv?bank=sparkasse", "text/csv", strings.NewReader(csvBody))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError || strings.TrimSpace(string(body)) != "internal error" {
		t.Errorf("response = %d %q, want 500 internal error", resp.StatusCode, body)
	}

	recs := logtest.Records(t, buf)
	errs := logtest.Filter(recs, "ERROR", "")
	if len(errs) != 1 {
		t.Fatalf("got %d ERROR records, want 1:\n%s", len(errs), buf)
	}
	assertFields(t, errs[0], map[string]any{"method": "POST", "route": "POST /uploads/csv"})
	if s, _ := errs[0]["stack"].(string); s == "" {
		t.Errorf("ERROR record has no stack")
	}
	access := accessRecord(t, recs)
	assertFields(t, access, map[string]any{"status": 500.0})
	if id, _ := errs[0]["request_id"].(string); id == "" || id != access["request_id"] {
		t.Errorf("ERROR request_id = %v, access request_id = %v; want the same, non-empty", errs[0]["request_id"], access["request_id"])
	}
	logtest.AssertLacks(t, buf, "boom REWE")
}

func TestPanic_AfterResponseStarted(t *testing.T) {
	srv, buf := panicServer(t, &fakeImporter{})

	resp, err := http.Get(srv.URL + "/after-write")
	if err == nil {
		_, err = io.ReadAll(resp.Body)
		resp.Body.Close()
	}
	if err == nil {
		t.Error("response completed; want the connection closed mid-response")
	}

	recs := logtest.Records(t, buf)
	if n := len(logtest.Filter(recs, "ERROR", "")); n != 1 {
		t.Errorf("got %d ERROR records, want 1:\n%s", n, buf)
	}
	assertFields(t, accessRecord(t, recs), map[string]any{"route": "GET /after-write", "status": 200.0})
	logtest.AssertLacks(t, buf, "boom REWE")
}

func TestPanic_DeliberateAbort(t *testing.T) {
	srv, buf := panicServer(t, &fakeImporter{})

	resp, err := http.Get(srv.URL + "/abort")
	if err == nil {
		resp.Body.Close()
		t.Errorf("got response %d; want the connection closed", resp.StatusCode)
	}

	recs := logtest.Records(t, buf)
	if n := len(logtest.Filter(recs, "ERROR", "")); n != 0 {
		t.Errorf("got %d ERROR records, want none:\n%s", n, buf)
	}
	assertFields(t, accessRecord(t, recs), map[string]any{"route": "GET /abort", "status": 0.0})
}
