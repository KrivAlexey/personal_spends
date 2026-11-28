package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/KrivAlexey/personal_spends/internal/categories"
	"github.com/KrivAlexey/personal_spends/internal/storage"
)

const (
	defaultListLimit = 100
	maxListLimit     = 500
	// maxExpenseBytes caps a POST /expenses body; a real one is a few hundred bytes.
	maxExpenseBytes = 64 << 10
)

// addExpenseRequest is the JSON body of POST /expenses. The pointers tell a
// missing field from a zero one.
type addExpenseRequest struct {
	Date           *string  `json:"date"`
	Amount         *float64 `json:"amount"`
	Merchant       string   `json:"merchant"`
	RawDescription string   `json:"raw_description"`
	Category       string   `json:"category"`
}

func toExpenseEntry(e storage.Expense) expenseEntry {
	return expenseEntry{
		ID: e.ID, Date: e.Date.Format(time.DateOnly), Amount: e.Amount, Currency: e.Currency,
		Merchant: e.Merchant, RawDescription: e.RawDescription, Category: e.Category,
		Confidence: e.Confidence, Source: e.Source, CreatedAt: e.CreatedAt.UTC(),
	}
}

// fail answers 500 with a fixed body and logs what for, with err, once.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, what string, err error) {
	h.logger.Log(r.Context(), failureLevel(r, err), what+" failed",
		"method", r.Method, "route", r.Pattern, "error", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

// parseDateRange reads the optional from and to parameters, or returns the
// text of the 400 answer.
func parseDateRange(v url.Values) (from, to time.Time, problem string) {
	var err error
	if s := v.Get("from"); s != "" {
		if from, err = time.Parse(time.DateOnly, s); err != nil {
			return from, to, "from must be a date, YYYY-MM-DD"
		}
	}
	if s := v.Get("to"); s != "" {
		if to, err = time.Parse(time.DateOnly, s); err != nil {
			return from, to, "to must be a date, YYYY-MM-DD"
		}
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return from, to, "from is after to"
	}
	return from, to, ""
}

// parseAmount reads an optional finite decimal parameter; nil when absent.
func parseAmount(v url.Values, name string) (*float64, string) {
	s := v.Get(name)
	if s == "" {
		return nil, ""
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return nil, name + " must be a number"
	}
	return &f, ""
}

// listExpenses answers the expenses matching the query parameters.
func (h *Handler) listExpenses(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query()
	q := storage.ExpenseQuery{Category: v.Get("category"), Limit: defaultListLimit}
	var problem string
	if q.From, q.To, problem = parseDateRange(v); problem != "" {
		h.reject(w, r, http.StatusBadRequest, "", "invalid date range", problem)
		return
	}
	if q.MinAmount, problem = parseAmount(v, "min_amount"); problem != "" {
		h.reject(w, r, http.StatusBadRequest, "", "invalid min_amount", problem)
		return
	}
	if q.MaxAmount, problem = parseAmount(v, "max_amount"); problem != "" {
		h.reject(w, r, http.StatusBadRequest, "", "invalid max_amount", problem)
		return
	}
	if q.MinAmount != nil && q.MaxAmount != nil && *q.MinAmount > *q.MaxAmount {
		h.reject(w, r, http.StatusBadRequest, "", "min_amount above max_amount", "min_amount is above max_amount")
		return
	}
	if s := v.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > maxListLimit {
			h.reject(w, r, http.StatusBadRequest, "", "invalid limit", "limit must be an integer from 1 to "+strconv.Itoa(maxListLimit))
			return
		}
		q.Limit = n
	}

	found, err := h.store.QueryExpenses(r.Context(), q)
	if err != nil {
		h.fail(w, r, "list expenses", err)
		return
	}
	entries := make([]expenseEntry, len(found))
	for i, e := range found {
		entries[i] = toExpenseEntry(e)
	}
	h.writeJSON(w, r, expensesResponse{Expenses: entries})
}

// round2 rounds to cents, so summing floats leaves no noise like -30.299999999999997.
func round2(f float64) float64 { return math.Round(f*100) / 100 }

// summary answers spending totals per category over the date range. It reads
// every expense in the range: a capped read would give wrong totals.
func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	from, to, problem := parseDateRange(r.URL.Query())
	if problem != "" {
		h.reject(w, r, http.StatusBadRequest, "", "invalid date range", problem)
		return
	}
	found, err := h.store.QueryExpenses(r.Context(), storage.ExpenseQuery{From: from, To: to})
	if err != nil {
		h.fail(w, r, "summarize expenses", err)
		return
	}

	byCategory := map[string]*summaryEntry{}
	var total float64
	for _, e := range found {
		s := byCategory[e.Category]
		if s == nil {
			s = &summaryEntry{Category: e.Category}
			byCategory[e.Category] = s
		}
		s.Total += e.Amount
		s.Count++
		total += e.Amount
	}
	res := summaryResponse{Categories: make([]summaryEntry, 0, len(byCategory)), Total: round2(total), Count: len(found)}
	for _, s := range byCategory {
		s.Total = round2(s.Total)
		res.Categories = append(res.Categories, *s)
	}
	sort.Slice(res.Categories, func(i, j int) bool { return res.Categories[i].Category < res.Categories[j].Category })
	h.writeJSON(w, r, res)
}

// listCategories answers the configured categories.
func (h *Handler) listCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.cats.List(r.Context())
	if err != nil {
		h.fail(w, r, "list categories", err)
		return
	}
	entries := make([]categoryEntry, len(cats))
	for i, c := range cats {
		entries[i] = categoryEntry{Name: c.Name, Description: c.Description}
	}
	h.writeJSON(w, r, categoriesResponse{Categories: entries})
}

// addExpense stores the expense described by the JSON body.
func (h *Handler) addExpense(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxExpenseBytes))
	dec.DisallowUnknownFields()
	var req addExpenseRequest
	err := dec.Decode(&req)
	if err == nil {
		// Anything after the object, including a body that runs past the cap.
		if err = dec.Decode(&struct{}{}); err == nil {
			err = errors.New("trailing data")
		} else if err == io.EOF {
			err = nil
		}
	}
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		h.reject(w, r, http.StatusRequestEntityTooLarge, "", "body too large", "body larger than 64 KiB")
		return
	}
	if err != nil {
		h.reject(w, r, http.StatusBadRequest, "", "invalid body", "body must be one JSON object with known fields")
		return
	}

	var day time.Time
	if req.Date != nil {
		day, err = time.Parse(time.DateOnly, *req.Date)
	}
	switch {
	case req.Date == nil || err != nil:
		h.reject(w, r, http.StatusBadRequest, "", "invalid date", "date must be a date, YYYY-MM-DD")
		return
	case req.Amount == nil || *req.Amount == 0:
		h.reject(w, r, http.StatusBadRequest, "", "invalid amount", "amount must be a number other than 0")
		return
	case strings.TrimSpace(req.Merchant) == "":
		h.reject(w, r, http.StatusBadRequest, "", "missing merchant", "merchant is required")
		return
	}
	if _, err := h.cats.Get(r.Context(), req.Category); errors.Is(err, categories.ErrCategoryNotFound) {
		h.reject(w, r, http.StatusBadRequest, "", "unknown category", "category is not a configured category")
		return
	} else if err != nil {
		h.fail(w, r, "look up category", err)
		return
	}

	var id [8]byte
	_, _ = rand.Read(id[:]) // never fails: it crashes the program instead (Go 1.24)
	e := storage.Expense{
		ID: hex.EncodeToString(id[:]) + "-0", Date: day, Amount: *req.Amount, Currency: "EUR",
		Merchant: req.Merchant, RawDescription: req.RawDescription, Category: req.Category,
		Confidence: 1, Source: "manual", CreatedAt: time.Now().UTC().Truncate(time.Second),
	}
	if err := h.store.SaveExpenses(r.Context(), []storage.Expense{e}); err != nil {
		h.fail(w, r, "add expense", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(toExpenseEntry(e)); err != nil {
		h.logger.WarnContext(r.Context(), "write response failed",
			"method", r.Method, "route", r.Pattern, "error", err)
	}
}
