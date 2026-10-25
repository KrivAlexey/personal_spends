package handler

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/KrivAlexey/personal_spends/internal/logging"
)

// withLogging gives each request a request_id, writes its INFO access record
// and turns a panic into an ERROR record (ADR 0013).
func (h *Handler) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		// ServeMux sets Pattern on this copy, so the deferred function reads
		// the route from it, not from the caller's request.
		r = r.WithContext(logging.WithAttrs(r.Context(), slog.String("request_id", newRequestID())))
		sw := &statusWriter{ResponseWriter: w}

		// Deferred, so the access record is written however next exits.
		defer func() {
			p := recover()
			ctx := r.Context()
			route := r.Pattern
			if route == "" {
				route = "unmatched"
			}
			startedBeforePanic := sw.started

			if p != nil && p != http.ErrAbortHandler {
				// The stack, not the panic value: the value is free text that
				// could carry bank data.
				h.logger.ErrorContext(ctx, "handler panicked",
					"method", r.Method, "route", route, "stack", string(debug.Stack()))
				if !sw.started {
					http.Error(sw, "internal error", http.StatusInternalServerError)
				}
			}

			status := sw.status
			if p == nil && !sw.started {
				status = http.StatusOK // net/http sends an implicit 200
			}
			h.logger.InfoContext(ctx, "request",
				"method", r.Method, "route", route, "status", status,
				"duration_ms", time.Since(start).Milliseconds())

			// Last: the re-panic leaves this function at once. net/http treats
			// ErrAbortHandler as a silent abort and closes the connection, so a
			// cut-off response isn't taken as complete.
			if p == http.ErrAbortHandler || (p != nil && startedBeforePanic) {
				panic(http.ErrAbortHandler)
			}
		}()

		next.ServeHTTP(sw, r)
	})
}

func newRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b) // never fails since Go 1.24
	return hex.EncodeToString(b)
}

// statusWriter records the status the client receives. The response counts as
// started on the first final (non-1xx) WriteHeader or on the first Write.
type statusWriter struct {
	http.ResponseWriter
	status  int
	started bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.started && code >= 200 {
		w.status, w.started = code, true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.started {
		w.status, w.started = http.StatusOK, true
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach Flusher and Hijacker.
func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
