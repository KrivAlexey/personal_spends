// Package handler exposes the REST API over net/http.
package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/KrivAlexey/personal_spends/internal/ingest"
	"github.com/KrivAlexey/personal_spends/internal/parser"
)

// uploadResponse is the JSON body of a successful POST /uploads/csv.
type uploadResponse struct {
	Imported       int `json:"imported"`
	AlreadyKnown   int `json:"already_known"`
	PendingSkipped int `json:"pending_skipped"`
}

type importer interface {
	Import(ctx context.Context, bankName string, r io.Reader) (ingest.Result, error)
}

// Handler serves the REST API.
type Handler struct {
	imp    importer
	logger *slog.Logger
}

// New returns a Handler that imports uploads with imp and logs to logger; a
// nil logger discards.
func New(imp importer, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Handler{imp: imp, logger: logger}
}

// Routes returns the API's router, wrapped in the logging middleware.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /uploads/csv", h.uploadCSV)
	return h.withLogging(mux)
}

// uploadCSV imports the export sent as the raw request body; the query
// parameter "bank" names the bank mapping (ADR 0012).
func (h *Handler) uploadCSV(w http.ResponseWriter, r *http.Request) {
	bank := r.URL.Query().Get("bank")
	if bank == "" {
		h.reject(w, r, bank, "missing bank parameter", "missing bank query parameter")
		return
	}

	body := &readErrRecorder{r: r.Body}
	br := bufio.NewReader(body)
	if _, err := br.Peek(1); err != nil {
		if err == io.EOF {
			h.reject(w, r, bank, "empty body", "empty bank CSV")
			return
		}
		h.reject(w, r, bank, "unreadable body", "reading request body failed")
		return
	}

	res, err := h.imp.Import(r.Context(), bank, br)
	if err != nil && body.err != nil {
		h.reject(w, r, bank, "unreadable body", "reading request body failed")
		return
	}
	if errors.Is(err, parser.ErrBankMappingNotFound) {
		h.reject(w, r, bank, "bank mapping not found", fmt.Sprintf("bank mapping %q not found", bank))
		return
	}
	if err != nil {
		// A client that disconnected isn't a server fault. Only the request's
		// own cancellation counts: a deadline, or a Canceled from a context we
		// cancelled ourselves, is ours to fix.
		level := slog.LevelError
		if errors.Is(err, context.Canceled) && errors.Is(r.Context().Err(), context.Canceled) {
			level = slog.LevelWarn
		}
		// The counts tell the operator how much was saved before the failure;
		// a re-run is safe (ADR 0011).
		h.logger.Log(r.Context(), level, "upload failed",
			"method", r.Method, "route", r.Pattern, "bank", capBank(bank),
			"imported", res.Imported, "already_known", res.AlreadyKnown, "pending_skipped", res.PendingSkipped,
			"error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(toUploadResponse(res)); err != nil {
		// The 200 is already sent; logging is all that's left to do.
		h.logger.WarnContext(r.Context(), "write response failed",
			"method", r.Method, "route", r.Pattern, "error", err)
	}
}

// reject answers 400 with body and logs reason, which must be fixed text
// without caller input.
func (h *Handler) reject(w http.ResponseWriter, r *http.Request, bank, reason, body string) {
	attrs := []any{"method", r.Method, "route", r.Pattern, "reason", reason}
	if bank != "" {
		attrs = append(attrs, "bank", capBank(bank))
	}
	h.logger.WarnContext(r.Context(), "request rejected", attrs...)
	http.Error(w, body, http.StatusBadRequest)
}

const maxLoggedBankRunes = 64

// capBank cuts a caller's bank value to maxLoggedBankRunes runes plus "…", so
// one request can't write a megabyte into every record.
func capBank(bank string) string {
	n := 0
	for i := range bank {
		if n == maxLoggedBankRunes {
			return bank[:i] + "…"
		}
		n++
	}
	return bank
}

func toUploadResponse(result ingest.Result) uploadResponse {
	return uploadResponse{
		Imported:       result.Imported,
		AlreadyKnown:   result.AlreadyKnown,
		PendingSkipped: result.PendingSkipped,
	}
}

// readErrRecorder remembers the first read error other than io.EOF, so a body
// cut off mid-upload is told apart from a server-side import failure.
type readErrRecorder struct {
	r   io.Reader
	err error
}

func (b *readErrRecorder) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	if err != nil && err != io.EOF && b.err == nil {
		b.err = err
	}
	return n, err
}
