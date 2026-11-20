package handler

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KrivAlexey/personal_spends/internal/logging/logtest"
)

// default-src doesn't cover framing; frame-ancestors keeps other sites from
// framing the Upload button (clickjacking).
const wantCSP = "default-src 'self'; frame-ancestors 'none'"

func TestWebAssets(t *testing.T) {
	tests := []struct {
		path        string
		contentType string
		contains    []string
	}{
		{path: "/", contentType: "text/html; charset=utf-8", contains: []string{`src="/app.js"`, `href="/app.css"`}},
		{path: "/app.js", contentType: "text/javascript; charset=utf-8", contains: []string{"/uploads", "/banks"}},
		{path: "/app.css", contentType: "text/css; charset=utf-8"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			New(&fakeUploader{}, testBanks, nil).Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			h := rec.Header()
			if got := h.Get("Content-Type"); got != tt.contentType {
				t.Errorf("Content-Type = %q, want %q", got, tt.contentType)
			}
			if got := h.Get("Content-Security-Policy"); got != wantCSP {
				t.Errorf("Content-Security-Policy = %q, want %q", got, wantCSP)
			}
			if got := h.Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
			}
			if rec.Body.Len() == 0 {
				t.Error("empty body")
			}
			for _, c := range tt.contains {
				if !strings.Contains(rec.Body.String(), c) {
					t.Errorf("body lacks %q", c)
				}
			}
		})
	}
}

// Only the exact root is the page; anything else unknown stays a 404 and is
// logged as unmatched.
func TestWebAssets_UnknownPathIsNotThePage(t *testing.T) {
	h, buf := loggedHandler(&fakeUploader{}, slog.LevelInfo)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nothing-here", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	assertFields(t, accessRecord(t, logtest.Records(t, buf)), map[string]any{"route": "unmatched", "status": 404.0})
}

// File names are caller input: the script must build the DOM from text, never
// from HTML strings (web-ui spec: caller-supplied text is never markup).
func TestWebAssets_ScriptBuildsNoHTMLFromStrings(t *testing.T) {
	script, err := webFiles.ReadFile("web/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	for _, sink := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write", "eval("} {
		if bytes.Contains(script, []byte(sink)) {
			t.Errorf("app.js uses %s", sink)
		}
	}
}
