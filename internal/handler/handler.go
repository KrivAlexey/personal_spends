// Package handler exposes the REST API over net/http.
package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	imp importer
}

// New returns a Handler that imports uploads with imp.
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
	bank := r.URL.Query().Get("bank")
	if bank == "" {
		http.Error(w, "missing bank query parameter", http.StatusBadRequest)
		return
	}

	br := bufio.NewReader(r.Body)
	if _, err := br.Peek(1); err != nil {
		if err == io.EOF {
			http.Error(w, "empty bank CSV", http.StatusBadRequest)
			return
		}
		http.Error(w, "reading request body failed", http.StatusBadRequest)
		return
	}

	res, err := h.imp.Import(r.Context(), bank, br)
	if errors.Is(err, parser.ErrBankMappingNotFound) {
		http.Error(w, fmt.Sprintf("bank mapping %q not found", bank), http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(toUploadResponse(res))
}

func toUploadResponse(result ingest.Result) uploadResponse {
	return uploadResponse{
		Imported:       result.Imported,
		AlreadyKnown:   result.AlreadyKnown,
		PendingSkipped: result.PendingSkipped,
	}
}
