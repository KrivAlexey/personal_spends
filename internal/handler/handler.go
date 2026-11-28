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

	"github.com/KrivAlexey/personal_spends/internal/categories"
	"github.com/KrivAlexey/personal_spends/internal/parser"
	"github.com/KrivAlexey/personal_spends/internal/storage"
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

type expenseStore interface {
	QueryExpenses(ctx context.Context, q storage.ExpenseQuery) ([]storage.Expense, error)
	SaveExpenses(ctx context.Context, expenses []storage.Expense) error
}

// Handler serves the REST API.
type Handler struct {
	up     uploadService
	banks  bankLister
	store  expenseStore
	cats   categories.CategoryProvider
	logger *slog.Logger
}

// New returns a Handler that runs and lists uploads with up, lists bank
// names from banks, reads and adds expenses with store, lists categories
// from cats and logs to logger; a nil logger discards.
func New(up uploadService, banks bankLister, store expenseStore, cats categories.CategoryProvider, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Handler{up: up, banks: banks, store: store, cats: cats, logger: logger}
}

// expenseEntry is one expense in a response.
type expenseEntry struct {
	ID             string    `json:"id"`
	Date           string    `json:"date"`
	Amount         float64   `json:"amount"`
	Currency       string    `json:"currency"`
	Merchant       string    `json:"merchant"`
	RawDescription string    `json:"raw_description"`
	Category       string    `json:"category"`
	Confidence     float64   `json:"confidence"`
	Source         string    `json:"source"`
	CreatedAt      time.Time `json:"created_at"`
}

// expensesResponse is the JSON body of GET /expenses.
type expensesResponse struct {
	Expenses []expenseEntry `json:"expenses"`
}

// summaryEntry is one category's line in GET /expenses/summary.
type summaryEntry struct {
	Category string  `json:"category"`
	Total    float64 `json:"total"`
	Count    int     `json:"count"`
}

// summaryResponse is the JSON body of GET /expenses/summary.
type summaryResponse struct {
	Categories []summaryEntry `json:"categories"`
	Total      float64        `json:"total"`
	Count      int            `json:"count"`
}

// categoryEntry is one category in GET /categories.
type categoryEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// categoriesResponse is the JSON body of GET /categories.
type categoriesResponse struct {
	Categories []categoryEntry `json:"categories"`
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

// uploadsResponse is the JSON body of GET /uploads.
type uploadsResponse struct {
	Uploads []uploadEntry `json:"uploads"`
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
		"POST /uploads/csv":     h.uploadCSV,
		"GET /uploads":          h.listUploads,
		"GET /banks":            h.listBanks,
		"GET /openapi.yaml":     h.openAPI,
		"GET /expenses":         h.listExpenses,
		"POST /expenses":        h.addExpense,
		"GET /expenses/summary": h.summary,
		"GET /categories":       h.listCategories,
		// {$}: only the root itself is the page, other paths stay 404.
		"GET /{$}":     h.webFile("web/index.html", "text/html; charset=utf-8"),
		"GET /app.js":  h.webFile("web/app.js", "text/javascript; charset=utf-8"),
		"GET /app.css": h.webFile("web/app.css", "text/css; charset=utf-8"),
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

// maxListedUploads is how many upload records GET /uploads returns.
const maxListedUploads = 100

// listUploads answers the most recent upload records, newest first.
func (h *Handler) listUploads(w http.ResponseWriter, r *http.Request) {
	recs, err := h.up.List(r.Context(), maxListedUploads)
	if err != nil {
		h.logger.Log(r.Context(), failureLevel(r, err), "list uploads failed",
			"method", r.Method, "route", r.Pattern, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	entries := make([]uploadEntry, 0, len(recs))
	for _, rec := range recs {
		entries = append(entries, toUploadEntry(rec))
	}
	h.writeJSON(w, r, uploadsResponse{Uploads: entries})
}

func toUploadEntry(rec uploads.Record) uploadEntry {
	e := uploadEntry{
		UploadID:   rec.ID,
		Filename:   rec.Filename,
		Bank:       rec.Bank,
		UploadedAt: rec.UploadedAt.UTC(),
		Status:     string(rec.Status),
	}
	if rec.Status == uploads.StatusDone {
		c := rec.Counts
		e.Imported, e.AlreadyKnown, e.PendingSkipped = &c.Imported, &c.AlreadyKnown, &c.PendingSkipped
	}
	return e
}

// listBanks answers the configured bank mapping names.
func (h *Handler) listBanks(w http.ResponseWriter, r *http.Request) {
	banks := h.banks.Banks()
	if banks == nil {
		banks = []string{}
	}
	h.writeJSON(w, r, banksResponse{Banks: banks})
}

// writeJSON answers 200 with v as JSON.
func (h *Handler) writeJSON(w http.ResponseWriter, r *http.Request, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
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
		h.reject(w, r, http.StatusConflict, bank, "upload in progress", "the same file is being imported, or its last attempt just failed; retry shortly")
		return
	}
	if errors.Is(err, parser.ErrBankMappingNotFound) {
		h.reject(w, r, http.StatusBadRequest, bank,
			"bank mapping not found", fmt.Sprintf("bank mapping %q not found", bank))
		return
	}
	if err != nil {
		level := failureLevel(r, err)
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

// failureLevel is WARN when err comes from the client disconnecting, ERROR
// otherwise. A client that disconnected isn't a server fault. Only the
// request's own cancellation counts: a deadline, or a Canceled from a context
// we cancelled ourselves, is ours to fix.
func failureLevel(r *http.Request, err error) slog.Level {
	if errors.Is(err, context.Canceled) && errors.Is(r.Context().Err(), context.Canceled) {
		return slog.LevelWarn
	}
	return slog.LevelError
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
