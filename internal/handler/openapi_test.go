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
	Type       string                   `yaml:"type"`
	Required   []string                 `yaml:"required"`
	Properties map[string]openAPISchema `yaml:"properties"`
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
			New(&fakeUploader{}, nil).Routes().ServeHTTP(rec, httptest.NewRequest(tt.method, "/openapi.yaml", nil))

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
	registered := slices.Sorted(maps.Keys(New(&fakeUploader{}, nil).routes()))

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
