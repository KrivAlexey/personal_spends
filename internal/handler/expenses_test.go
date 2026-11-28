package handler

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/KrivAlexey/personal_spends/internal/categories"
	"github.com/KrivAlexey/personal_spends/internal/storage"
)

type fakeExpenses struct {
	result   []storage.Expense
	queryErr error
	saveErr  error

	queries []storage.ExpenseQuery
	saved   []storage.Expense
}

func (f *fakeExpenses) QueryExpenses(_ context.Context, q storage.ExpenseQuery) ([]storage.Expense, error) {
	f.queries = append(f.queries, q)
	return f.result, f.queryErr
}

func (f *fakeExpenses) SaveExpenses(_ context.Context, e []storage.Expense) error {
	f.saved = append(f.saved, e...)
	return f.saveErr
}

type fakeCategories struct {
	cats []categories.Category
	err  error
}

func (f fakeCategories) List(context.Context) ([]categories.Category, error) { return f.cats, f.err }

func (f fakeCategories) Get(_ context.Context, name string) (categories.Category, error) {
	if f.err != nil {
		return categories.Category{}, f.err
	}
	for _, c := range f.cats {
		if c.Name == name {
			return c, nil
		}
	}
	return categories.Category{}, categories.ErrCategoryNotFound
}

var testCats = fakeCategories{cats: []categories.Category{
	{Name: "groceries", Description: "Food shopping"},
	{Name: "fuel", Description: "Petrol"},
}}

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func get(t *testing.T, h *Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func ptrF(f float64) *float64 { return &f }

func TestListExpenses_Query(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		wantStatus int
		wantQuery  storage.ExpenseQuery
	}{
		{name: "no parameters", target: "/expenses", wantStatus: 200, wantQuery: storage.ExpenseQuery{Limit: 100}},
		{
			name:       "every parameter",
			target:     "/expenses?from=2026-04-01&to=2026-04-30&category=groceries&min_amount=-50&max_amount=-10.5&limit=7",
			wantStatus: 200,
			wantQuery: storage.ExpenseQuery{
				From: date(2026, 4, 1), To: date(2026, 4, 30), Category: "groceries",
				MinAmount: ptrF(-50), MaxAmount: ptrF(-10.5), Limit: 7,
			},
		},
		{name: "limit at the cap", target: "/expenses?limit=500", wantStatus: 200, wantQuery: storage.ExpenseQuery{Limit: 500}},
		{name: "from equals to", target: "/expenses?from=2026-04-01&to=2026-04-01", wantStatus: 200,
			wantQuery: storage.ExpenseQuery{From: date(2026, 4, 1), To: date(2026, 4, 1), Limit: 100}},
		{name: "min equals max", target: "/expenses?min_amount=-5&max_amount=-5", wantStatus: 200,
			wantQuery: storage.ExpenseQuery{MinAmount: ptrF(-5), MaxAmount: ptrF(-5), Limit: 100}},
		{name: "limit over the cap", target: "/expenses?limit=501", wantStatus: 400},
		{name: "limit zero", target: "/expenses?limit=0", wantStatus: 400},
		{name: "limit negative", target: "/expenses?limit=-1", wantStatus: 400},
		{name: "limit not a number", target: "/expenses?limit=abc", wantStatus: 400},
		{name: "from not a date", target: "/expenses?from=yesterday", wantStatus: 400},
		{name: "to impossible date", target: "/expenses?to=2026-02-30", wantStatus: 400},
		{name: "from after to", target: "/expenses?from=2026-05-01&to=2026-04-01", wantStatus: 400},
		{name: "min not a number", target: "/expenses?min_amount=abc", wantStatus: 400},
		{name: "max NaN", target: "/expenses?max_amount=NaN", wantStatus: 400},
		{name: "min infinite", target: "/expenses?min_amount=Inf", wantStatus: 400},
		{name: "min above max", target: "/expenses?min_amount=5&max_amount=-5", wantStatus: 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeExpenses{}
			rec := get(t, New(&fakeUploader{}, testBanks, store, testCats, nil), tt.target)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body %q", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantStatus != http.StatusOK {
				if len(store.queries) != 0 {
					t.Errorf("storage was queried %d times, want 0", len(store.queries))
				}
				return
			}
			if len(store.queries) != 1 || !reflect.DeepEqual(store.queries[0], tt.wantQuery) {
				t.Errorf("queries = %+v, want [%+v]", store.queries, tt.wantQuery)
			}
		})
	}
}

func TestListExpenses_Body(t *testing.T) {
	created := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		result []storage.Expense
		want   string
	}{
		{name: "none", want: `{"expenses": []}`},
		{
			name: "two",
			result: []storage.Expense{
				{ID: "h1-0", Date: date(2026, 4, 2), Amount: -12.5, Currency: "EUR", Merchant: "Bakery", RawDescription: "Kartenzahlung", Category: "groceries", Confidence: 0.9, Source: "sparkasse", CreatedAt: created},
				{ID: "h2-0", Date: date(2026, 4, 1), Amount: 100, Currency: "EUR", Merchant: "Employer", Category: "income", Confidence: 1, Source: "manual", CreatedAt: created},
			},
			want: `{"expenses": [
				{"id":"h1-0","date":"2026-04-02","amount":-12.5,"currency":"EUR","merchant":"Bakery","raw_description":"Kartenzahlung","category":"groceries","confidence":0.9,"source":"sparkasse","created_at":"2026-08-15T10:00:00Z"},
				{"id":"h2-0","date":"2026-04-01","amount":100,"currency":"EUR","merchant":"Employer","raw_description":"","category":"income","confidence":1,"source":"manual","created_at":"2026-08-15T10:00:00Z"}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := get(t, New(&fakeUploader{}, testBanks, &fakeExpenses{result: tt.result}, testCats, nil), "/expenses")

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			assertJSONEqual(t, rec.Body.Bytes(), tt.want)
		})
	}
}

func TestExpenseEndpoints_StorageFailureDoesNotLeak(t *testing.T) {
	secret := errors.New("dynamodb: table secret-table-name throttled")
	tests := []struct {
		name   string
		store  *fakeExpenses
		cats   fakeCategories
		method string
		target string
		body   string
	}{
		{name: "list", store: &fakeExpenses{queryErr: secret}, cats: testCats, method: http.MethodGet, target: "/expenses"},
		{name: "summary", store: &fakeExpenses{queryErr: secret}, cats: testCats, method: http.MethodGet, target: "/expenses/summary"},
		{name: "add", store: &fakeExpenses{saveErr: secret}, cats: testCats, method: http.MethodPost, target: "/expenses", body: validExpenseBody},
		{name: "add, category lookup", store: &fakeExpenses{}, cats: fakeCategories{err: secret}, method: http.MethodPost, target: "/expenses", body: validExpenseBody},
		{name: "categories", store: &fakeExpenses{}, cats: fakeCategories{err: secret}, method: http.MethodGet, target: "/categories"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			New(&fakeUploader{}, testBanks, tt.store, tt.cats, nil).Routes().ServeHTTP(rec, req)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500", rec.Code)
			}
			if strings.Contains(rec.Body.String(), "secret-table-name") {
				t.Errorf("body %q leaks the internal error", rec.Body)
			}
		})
	}
}

const validExpenseBody = `{"date":"2026-04-02","amount":-12.5,"merchant":"Bakery","category":"groceries"}`

func post(t *testing.T, h *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/expenses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.Routes().ServeHTTP(rec, req)
	return rec
}

func TestAddExpense_Valid(t *testing.T) {
	store := &fakeExpenses{}
	before := time.Now()
	rec := post(t, New(&fakeUploader{}, testBanks, store, testCats, nil),
		`{"date":"2026-04-02","amount":-12.5,"merchant":"Bakery","raw_description":"Fresh bread","category":"groceries"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body %q", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if len(store.saved) != 1 {
		t.Fatalf("saved %d expenses, want 1", len(store.saved))
	}
	got := store.saved[0]
	if !regexp.MustCompile(`^[0-9a-f]{16}-0$`).MatchString(got.ID) {
		t.Errorf("ID = %q, want <16 hex>-0", got.ID)
	}
	want := storage.Expense{
		ID: got.ID, Date: date(2026, 4, 2), Amount: -12.5, Currency: "EUR", Merchant: "Bakery",
		RawDescription: "Fresh bread", Category: "groceries", Confidence: 1, Source: "manual", CreatedAt: got.CreatedAt,
	}
	if got != want {
		t.Errorf("saved = %+v, want %+v", got, want)
	}
	if got.CreatedAt.Before(before.Add(-time.Minute)) || got.CreatedAt.After(time.Now().Add(time.Minute)) {
		t.Errorf("CreatedAt = %v, want about now", got.CreatedAt)
	}
	assertJSONEqual(t, rec.Body.Bytes(), `{"id":"`+got.ID+`","date":"2026-04-02","amount":-12.5,"currency":"EUR","merchant":"Bakery","raw_description":"Fresh bread","category":"groceries","confidence":1,"source":"manual","created_at":"`+got.CreatedAt.UTC().Format(time.RFC3339Nano)+`"}`)
}

func TestAddExpense_SameBodyTwiceGetsTwoIDs(t *testing.T) {
	store := &fakeExpenses{}
	h := New(&fakeUploader{}, testBanks, store, testCats, nil)
	for range 2 {
		if rec := post(t, h, validExpenseBody); rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201", rec.Code)
		}
	}
	if len(store.saved) != 2 || store.saved[0].ID == store.saved[1].ID {
		t.Errorf("saved %+v, want two expenses with different IDs", store.saved)
	}
}

func TestAddExpense_Rejected(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{name: "not JSON", body: "hello", wantStatus: 400},
		{name: "empty body", body: "", wantStatus: 400},
		{name: "JSON array", body: `[]`, wantStatus: 400},
		{name: "unknown field", body: `{"date":"2026-04-02","amount":-1,"merchant":"B","category":"groceries","id":"x"}`, wantStatus: 400},
		{name: "missing date", body: `{"amount":-1,"merchant":"B","category":"groceries"}`, wantStatus: 400},
		{name: "missing amount", body: `{"date":"2026-04-02","merchant":"B","category":"groceries"}`, wantStatus: 400},
		{name: "missing merchant", body: `{"date":"2026-04-02","amount":-1,"category":"groceries"}`, wantStatus: 400},
		{name: "missing category", body: `{"date":"2026-04-02","amount":-1,"merchant":"B"}`, wantStatus: 400},
		{name: "bad date", body: `{"date":"2026-02-30","amount":-1,"merchant":"B","category":"groceries"}`, wantStatus: 400},
		{name: "zero amount", body: `{"date":"2026-04-02","amount":0,"merchant":"B","category":"groceries"}`, wantStatus: 400},
		{name: "amount as string", body: `{"date":"2026-04-02","amount":"-1","merchant":"B","category":"groceries"}`, wantStatus: 400},
		{name: "empty merchant", body: `{"date":"2026-04-02","amount":-1,"merchant":"","category":"groceries"}`, wantStatus: 400},
		{name: "unknown category", body: `{"date":"2026-04-02","amount":-1,"merchant":"B","category":"nonsense"}`, wantStatus: 400},
		{name: "body over 64 KiB", body: `{"merchant":"` + strings.Repeat("a", 64<<10) + `"}`, wantStatus: 413},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeExpenses{}
			rec := post(t, New(&fakeUploader{}, testBanks, store, testCats, nil), tt.body)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if len(store.saved) != 0 {
				t.Errorf("saved %+v, want nothing", store.saved)
			}
		})
	}
}

func TestSummary(t *testing.T) {
	result := []storage.Expense{
		{Category: "groceries", Amount: -10.10},
		{Category: "fuel", Amount: -5},
		{Category: "groceries", Amount: -20.20},
	}
	tests := []struct {
		name       string
		target     string
		result     []storage.Expense
		wantStatus int
		want       string
		wantQuery  storage.ExpenseQuery
	}{
		{
			name: "two categories, sorted and rounded", target: "/expenses/summary", result: result, wantStatus: 200,
			want: `{"categories":[{"category":"fuel","total":-5,"count":1},{"category":"groceries","total":-30.3,"count":2}],"total":-35.3,"count":3}`,
		},
		{
			name: "range is passed on and nothing is capped", target: "/expenses/summary?from=2026-04-01&to=2026-04-30", result: result, wantStatus: 200,
			want:      `{"categories":[{"category":"fuel","total":-5,"count":1},{"category":"groceries","total":-30.3,"count":2}],"total":-35.3,"count":3}`,
			wantQuery: storage.ExpenseQuery{From: date(2026, 4, 1), To: date(2026, 4, 30)},
		},
		{name: "empty", target: "/expenses/summary", wantStatus: 200, want: `{"categories":[],"total":0,"count":0}`},
		{name: "bad from", target: "/expenses/summary?from=yesterday", wantStatus: 400},
		{name: "from after to", target: "/expenses/summary?from=2026-05-01&to=2026-04-01", wantStatus: 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeExpenses{result: tt.result}
			rec := get(t, New(&fakeUploader{}, testBanks, store, testCats, nil), tt.target)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantStatus != http.StatusOK {
				if len(store.queries) != 0 {
					t.Errorf("storage was queried, want not")
				}
				return
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			assertJSONEqual(t, rec.Body.Bytes(), tt.want)
			if len(store.queries) != 1 || !reflect.DeepEqual(store.queries[0], tt.wantQuery) {
				t.Errorf("queries = %+v, want [%+v]", store.queries, tt.wantQuery)
			}
		})
	}
}

func TestListCategories(t *testing.T) {
	tests := []struct {
		name string
		cats fakeCategories
		want string
	}{
		{name: "two", cats: testCats, want: `{"categories":[{"name":"groceries","description":"Food shopping"},{"name":"fuel","description":"Petrol"}]}`},
		{name: "none", cats: fakeCategories{}, want: `{"categories":[]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := get(t, New(&fakeUploader{}, testBanks, &fakeExpenses{}, tt.cats, nil), "/categories")

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			assertJSONEqual(t, rec.Body.Bytes(), tt.want)
		})
	}
}

// TestOpenAPIExpenseSchemas fails when the Go response types and the spec's
// schemas for the expense endpoints list different fields.
func TestOpenAPIExpenseSchemas(t *testing.T) {
	doc := parseOpenAPI(t)
	resolve := func(s openAPISchema) openAPISchema {
		if name, ok := strings.CutPrefix(s.Ref, "#/components/schemas/"); ok {
			return doc.Components.Schemas[name]
		}
		return s
	}
	fields := func(typ reflect.Type) []string {
		var out []string
		for i := range typ.NumField() {
			out = append(out, strings.Split(typ.Field(i).Tag.Get("json"), ",")[0])
		}
		slices.Sort(out)
		return out
	}
	body := func(path, method, code string) openAPISchema {
		return resolve(doc.Paths[path][method].Responses[code].Content["application/json"].Schema)
	}
	check := func(what string, s openAPISchema, typ reflect.Type) {
		t.Helper()
		if got, want := slices.Sorted(maps.Keys(s.Properties)), fields(typ); !slices.Equal(got, want) {
			t.Errorf("%s properties = %v, want %v", what, got, want)
		}
		if got, want := slices.Sorted(slices.Values(s.Required)), fields(typ); !slices.Equal(got, want) {
			t.Errorf("%s required = %v, want %v", what, got, want)
		}
	}
	items := func(what string, s openAPISchema, prop string) openAPISchema {
		t.Helper()
		it := s.Properties[prop].Items
		if it == nil {
			t.Fatalf("%s has no %s array with items", what, prop)
		}
		return resolve(*it)
	}

	list := body("/expenses", "get", "200")
	check("GET /expenses body", list, reflect.TypeFor[expensesResponse]())
	check("expense", items("GET /expenses body", list, "expenses"), reflect.TypeFor[expenseEntry]())
	check("POST /expenses body", body("/expenses", "post", "201"), reflect.TypeFor[expenseEntry]())

	sum := body("/expenses/summary", "get", "200")
	check("summary body", sum, reflect.TypeFor[summaryResponse]())
	check("summary entry", items("summary body", sum, "categories"), reflect.TypeFor[summaryEntry]())

	cats := body("/categories", "get", "200")
	check("categories body", cats, reflect.TypeFor[categoriesResponse]())
	check("category", items("categories body", cats, "categories"), reflect.TypeFor[categoryEntry]())
}

// TestOpenAPIExpenseStatuses fails when a status an expense route answers is
// missing from the spec.
func TestOpenAPIExpenseStatuses(t *testing.T) {
	doc := parseOpenAPI(t)
	tests := []struct {
		path, method string
		codes        []string
	}{
		{"/expenses", "get", []string{"200", "400", "500"}},
		{"/expenses", "post", []string{"201", "400", "413", "500"}},
		{"/expenses/summary", "get", []string{"200", "400", "500"}},
		{"/categories", "get", []string{"200", "500"}},
	}
	for _, tt := range tests {
		for _, code := range tt.codes {
			if _, ok := doc.Paths[tt.path][tt.method].Responses[code]; !ok {
				t.Errorf("%s %s lacks response %s", strings.ToUpper(tt.method), tt.path, code)
			}
		}
	}
}
