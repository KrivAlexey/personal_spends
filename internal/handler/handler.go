// Package handler exposes the REST API over net/http.
package handler

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/KrivAlexey/personal_spends/internal/parser"
	"github.com/KrivAlexey/personal_spends/internal/uploads"
)

// uploadResponse is the JSON body of a successful POST /uploads/csv.
type uploadResponse struct {
	UploadID       string `json:"upload_id"`
	Imported       int    `json:"imported"`
	AlreadyKnown   int    `json:"already_known"`
	PendingSkipped int    `json:"pending_skipped"`
}

type uploadService interface {
	Upload(ctx context.Context, req uploads.Request) (uploads.Result, error)
	List(ctx context.Context, limit int) ([]uploads.Record, error)
}

type bankLister interface {
	Banks() []string
}

// Handler serves the REST API.
type Handler struct {
	up     uploadService
	banks  bankLister
	logger *slog.Logger
}

// New returns a Handler that runs and lists uploads with up, lists bank
// names from banks and logs to logger; a nil logger discards.
func New(up uploadService, banks bankLister, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Handler{up: up, banks: banks, logger: logger}
}

// uploadEntry is one element of the GET /uploads list; counts are null
// unless the upload is done.
type uploadEntry struct {
	UploadID       string    `json:"upload_id"`
	Filename       string    `json:"filename"`
	Bank           string    `json:"bank"`
	UploadedAt     time.Time `json:"uploaded_at"`
	Status         string    `json:"status"`
	Imported       *int      `json:"imported"`
	AlreadyKnown   *int      `json:"already_known"`
	PendingSkipped *int      `json:"pending_skipped"`
}

// banksResponse is the JSON body of GET /banks.
type banksResponse struct {
	Banks []string `json:"banks"`
}

// openAPISpec is the OpenAPI document for every route in routes.
//
//go:embed openapi.yaml
var openAPISpec []byte

// Routes returns the API's router, wrapped in the logging middleware.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	for pattern, fn := range h.routes() {
		mux.HandleFunc(pattern, fn)
	}
	return h.withLogging(mux)
}

// routes maps each ServeMux pattern to its handler; openapi.yaml documents
// exactly these.
func (h *Handler) routes() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"POST /uploads/csv": h.uploadCSV,
		"GET /openapi.yaml": h.openAPI,
	}
}

// openAPI serves the embedded OpenAPI document.
func (h *Handler) openAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	if _, err := w.Write(openAPISpec); err != nil {
		h.logger.WarnContext(r.Context(), "write response failed",
			"method", r.Method, "route", r.Pattern, "error", err)
	}
}

// maxUploadBytes caps an upload body, which is held in memory to hash it.
// API Gateway caps payloads at 10 MB anyway.
const maxUploadBytes = 10 << 20

// uploadCSV uploads the export sent as the raw request body; the query
// parameter "bank" names the bank mapping (ADR 0012), the optional
// "filename" is recorded with it.
func (h *Handler) uploadCSV(w http.ResponseWriter, r *http.Request) {
	bank := r.URL.Query().Get("bank")
	if bank == "" {
		h.reject(w, r, http.StatusBadRequest, bank, "missing bank parameter", "missing bank query parameter")
		return
	}

	// Read the whole body first: the upload is keyed by its hash, and a body
	// cut off mid-upload must fail here, before anything is stored.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxUploadBytes))
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		h.reject(w, r, http.StatusRequestEntityTooLarge, bank, "body too large", "body larger than 10 MiB")
		return
	}
	if err != nil {
		h.reject(w, r, http.StatusBadRequest, bank, "unreadable body", "reading request body failed")
		return
	}
	if len(body) == 0 {
		h.reject(w, r, http.StatusBadRequest, bank, "empty body", "empty bank CSV")
		return
	}

	res, err := h.up.Upload(r.Context(), uploads.Request{
		Bank:     bank,
		Filename: r.URL.Query().Get("filename"),
		Body:     body,
	})
	if errors.Is(err, uploads.ErrInProgress) {
		h.reject(w, r, http.StatusConflict, bank, "upload in progress", "the same file is being imported")
		return
	}
	if errors.Is(err, parser.ErrBankMappingNotFound) {
		h.reject(w, r, http.StatusBadRequest, bank,
			"bank mapping not found", fmt.Sprintf("bank mapping %q not found", bank))
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
			"method", r.Method, "route", r.Pattern, "bank", capBank(bank), "upload_id", res.UploadID,
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

// reject answers status with body and logs reason, which must be fixed text
// without caller input.
func (h *Handler) reject(w http.ResponseWriter, r *http.Request, status int, bank, reason, body string) {
	attrs := []any{"method", r.Method, "route", r.Pattern, "reason", reason}
	if bank != "" {
		attrs = append(attrs, "bank", capBank(bank))
	}
	h.logger.WarnContext(r.Context(), "request rejected", attrs...)
	http.Error(w, body, status)
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

func toUploadResponse(result uploads.Result) uploadResponse {
	return uploadResponse{
		UploadID:       result.UploadID,
		Imported:       result.Imported,
		AlreadyKnown:   result.AlreadyKnown,
		PendingSkipped: result.PendingSkipped,
	}
}
