package handler

import (
	"bytes"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/KrivAlexey/personal_spends/internal/uploads"
)

// openAPIDoc is the part of an OpenAPI document the tests check.
type openAPIDoc struct {
	OpenAPI string `yaml:"openapi"`
	Info    struct {
		Title   string `yaml:"title"`
		Version string `yaml:"version"`
	} `yaml:"info"`
	Paths      map[string]map[string]openAPIOperation `yaml:"paths"`
	Components struct {
		Schemas map[string]openAPISchema `yaml:"schemas"`
	} `yaml:"components"`
}

type openAPIOperation struct {
	Responses map[string]struct {
		Content map[string]struct {
			Schema openAPISchema `yaml:"schema"`
		} `yaml:"content"`
	} `yaml:"responses"`
}

type openAPISchema struct {
	Ref        string                   `yaml:"$ref"`
	Type       any                      `yaml:"type"` // a string, or a list such as [integer, "null"]
	Required   []string                 `yaml:"required"`
	Properties map[string]openAPISchema `yaml:"properties"`
	Items      *openAPISchema           `yaml:"items"`
}

var openAPIMethods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}

func parseOpenAPI(t *testing.T) openAPIDoc {
	t.Helper()
	var doc openAPIDoc
	if err := yaml.Unmarshal(openAPISpec, &doc); err != nil {
		t.Fatalf("openapi.yaml does not parse: %v", err)
	}
	return doc
}

func TestOpenAPIDocument(t *testing.T) {
	doc := parseOpenAPI(t)
	if !strings.HasPrefix(doc.OpenAPI, "3.1.") {
		t.Errorf("openapi = %q, want 3.1.x", doc.OpenAPI)
	}
	if doc.Info.Title == "" || doc.Info.Version == "" {
		t.Errorf("info.title and info.version must be set, got %+v", doc.Info)
	}
}

func TestOpenAPIEndpoint(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		wantStatus int
	}{
		{name: "GET serves the document", method: http.MethodGet, wantStatus: http.StatusOK},
		{name: "POST is not allowed", method: http.MethodPost, wantStatus: http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			New(&fakeUploader{}, testBanks, &fakeExpenses{}, fakeCategories{}, nil).Routes().ServeHTTP(rec, httptest.NewRequest(tt.method, "/openapi.yaml", nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			if got := rec.Header().Get("Content-Type"); got != "application/yaml" {
				t.Errorf("Content-Type = %q, want application/yaml", got)
			}
			if !bytes.Equal(rec.Body.Bytes(), openAPISpec) {
				t.Errorf("body is not the embedded openapi.yaml")
			}
		})
	}
}

// TestOpenAPICoversRoutes fails when a route is added to the server without
// the spec, or to the spec without the server.
func TestOpenAPICoversRoutes(t *testing.T) {
	doc := parseOpenAPI(t)
	var documented []string
	for path, item := range doc.Paths {
		for _, m := range openAPIMethods {
			if _, ok := item[m]; ok {
				documented = append(documented, strings.ToUpper(m)+" "+path)
			}
		}
	}
	slices.Sort(documented)
	var registered []string
	for pattern := range New(&fakeUploader{}, testBanks, &fakeExpenses{}, fakeCategories{}, nil).routes() {
		// "{$}" is ServeMux syntax for "this path exactly"; OpenAPI paths are exact anyway.
		registered = append(registered, strings.TrimSuffix(pattern, "{$}"))
	}
	slices.Sort(registered)

	if !slices.Equal(documented, registered) {
		t.Errorf("spec documents %v, server registers %v", documented, registered)
	}
}

// TestOpenAPIUploadDocumentsStatuses fails when a status the upload route
// answers, or its filename parameter, is missing from the spec.
func TestOpenAPIUploadDocumentsStatuses(t *testing.T) {
	var doc struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name string `yaml:"name"`
			} `yaml:"parameters"`
			Responses map[string]any `yaml:"responses"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(openAPISpec, &doc); err != nil {
		t.Fatalf("openapi.yaml does not parse: %v", err)
	}
	op := doc.Paths["/uploads/csv"]["post"]
	for _, code := range []string{"200", "400", "409", "413", "500"} {
		if _, ok := op.Responses[code]; !ok {
			t.Errorf("POST /uploads/csv lacks response %s", code)
		}
	}
	var params []string
	for _, p := range op.Parameters {
		params = append(params, p.Name)
	}
	if !slices.Contains(params, "filename") {
		t.Errorf("POST /uploads/csv parameters = %v, want filename among them", params)
	}
}

// TestOpenAPIUploadResponseSchema fails when uploadResponse and the spec's
// 200 schema for POST /uploads/csv list different fields.
func TestOpenAPIUploadResponseSchema(t *testing.T) {
	doc := parseOpenAPI(t)
	schema := doc.Paths["/uploads/csv"]["post"].Responses["200"].Content["application/json"].Schema
	if name, ok := strings.CutPrefix(schema.Ref, "#/components/schemas/"); ok {
		schema = doc.Components.Schemas[name]
	}

	var want []string
	wantType := map[string]string{}
	typ := reflect.TypeFor[uploadResponse]()
	for i := range typ.NumField() {
		name := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		want = append(want, name)
		wantType[name] = map[reflect.Kind]string{reflect.Int: "integer", reflect.String: "string"}[typ.Field(i).Type.Kind()]
	}
	slices.Sort(want)

	if got := slices.Sorted(maps.Keys(schema.Properties)); !slices.Equal(got, want) {
		t.Errorf("schema properties = %v, want %v", got, want)
	}
	if got := slices.Sorted(slices.Values(schema.Required)); !slices.Equal(got, want) {
		t.Errorf("schema required = %v, want %v (all fields are always present)", got, want)
	}
	for name, p := range schema.Properties {
		if p.Type != wantType[name] {
			t.Errorf("property %s type = %q, want %q", name, p.Type, wantType[name])
		}
	}
}

// TestOpenAPIContentTypes fails when a documented GET route's 200 response
// lists a media type other than the one the server sends.
func TestOpenAPIContentTypes(t *testing.T) {
	doc := parseOpenAPI(t)
	routes := New(&fakeUploader{list: []uploads.Record{}}, testBanks, &fakeExpenses{}, fakeCategories{}, nil).Routes()
	for path, item := range doc.Paths {
		op, ok := item["get"]
		if !ok {
			continue
		}
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			routes.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			got, _, _ := strings.Cut(rec.Header().Get("Content-Type"), ";")
			documented := slices.Sorted(maps.Keys(op.Responses["200"].Content))
			if !slices.Equal(documented, []string{got}) {
				t.Errorf("documented 200 content %v, server sends %q", documented, got)
			}
		})
	}
}

// TestOpenAPIListSchemas fails when the GET /uploads entry or GET /banks body
// schema and the Go response types list different fields.
func TestOpenAPIListSchemas(t *testing.T) {
	doc := parseOpenAPI(t)
	resolve := func(s openAPISchema) openAPISchema {
		if name, ok := strings.CutPrefix(s.Ref, "#/components/schemas/"); ok {
			return doc.Components.Schemas[name]
		}
		return s
	}
	jsonFields := func(typ reflect.Type) []string {
		var out []string
		for i := range typ.NumField() {
			out = append(out, strings.Split(typ.Field(i).Tag.Get("json"), ",")[0])
		}
		return slices.Sorted(slices.Values(out))
	}

	uploadsBody := resolve(doc.Paths["/uploads"]["get"].Responses["200"].Content["application/json"].Schema)
	if got, want := slices.Sorted(maps.Keys(uploadsBody.Properties)), jsonFields(reflect.TypeFor[uploadsResponse]()); !slices.Equal(got, want) {
		t.Errorf("uploads body properties = %v, want %v", got, want)
	}
	items := uploadsBody.Properties["uploads"].Items
	if items == nil {
		t.Fatal("GET /uploads 200 schema has no uploads array with items")
	}
	entry := resolve(*items)
	if got, want := slices.Sorted(maps.Keys(entry.Properties)), jsonFields(reflect.TypeFor[uploadEntry]()); !slices.Equal(got, want) {
		t.Errorf("upload entry properties = %v, want %v", got, want)
	}
	if got, want := slices.Sorted(slices.Values(entry.Required)), jsonFields(reflect.TypeFor[uploadEntry]()); !slices.Equal(got, want) {
		t.Errorf("upload entry required = %v, want %v", got, want)
	}
	// Each Go field's JSON type, as OpenAPI 3.1 writes it: a pointer can be null.
	typ := reflect.TypeFor[uploadEntry]()
	for i := range typ.NumField() {
		f := typ.Field(i)
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		want := map[reflect.Kind]string{reflect.String: "string", reflect.Int: "integer", reflect.Struct: "string"}
		var wantType any = want[f.Type.Kind()]
		if f.Type.Kind() == reflect.Pointer {
			wantType = []any{want[f.Type.Elem().Kind()], "null"}
		}
		if got := entry.Properties[name].Type; !reflect.DeepEqual(got, wantType) {
			t.Errorf("upload entry %s type = %#v, want %#v", name, got, wantType)
		}
	}

	banksBody := resolve(doc.Paths["/banks"]["get"].Responses["200"].Content["application/json"].Schema)
	if got, want := slices.Sorted(maps.Keys(banksBody.Properties)), jsonFields(reflect.TypeFor[banksResponse]()); !slices.Equal(got, want) {
		t.Errorf("banks body properties = %v, want %v", got, want)
	}
}
