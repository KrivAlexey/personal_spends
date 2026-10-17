// Package handler exposes the REST API over net/http.
package handler

import (
	"context"
	"io"
	"net/http"

	"github.com/KrivAlexey/personal_spends/internal/ingest"
)

type importer interface {
	Import(ctx context.Context, bankName string, r io.Reader) (ingest.Result, error)
}

type Handler struct {
	imp importer
}

func New(imp importer) *Handler {
	return &Handler{imp: imp}
}

// Routes returns the API's router.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /uploads/csv", h.uploadCSV)
	return mux
}

// uploadCSV imports the export sent as the raw request body; the query
// parameter "bank" names the bank mapping (ADR 0012).
func (h *Handler) uploadCSV(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
