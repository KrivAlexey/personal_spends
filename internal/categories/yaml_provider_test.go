package categories

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeCategoriesFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "categories.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	return path
}

func TestNewYamlCategoryProvider(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			name: "valid categories",
			yaml: `
categories:
  - name: Groceries
    description: Food and household
  - name: Transport
    description: Public transit and fuel
`,
		},
		{
			name: "empty file",
			yaml: ``,
		},
		{
			name:    "invalid yaml",
			yaml:    "categories: [",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeCategoriesFile(t, tt.yaml)

			_, err := NewYamlCategoryProvider(path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewYamlCategoryProvider() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewYamlCategoryProvider_MissingFile(t *testing.T) {
	_, err := NewYamlCategoryProvider(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestYamlCategoryProvider_List(t *testing.T) {
	path := writeCategoriesFile(t, `
categories:
  - name: Transport
    description: Public transit and fuel
  - name: Groceries
    description: Food and household
`)

	provider, err := NewYamlCategoryProvider(path)
	if err != nil {
		t.Fatalf("NewYamlCategoryProvider() error = %v", err)
	}

	got, err := provider.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	want := []Category{
		{Name: "Groceries", Description: "Food and household"},
		{Name: "Transport", Description: "Public transit and fuel"},
	}
	if len(got) != len(want) {
		t.Fatalf("List() returned %d categories, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("List()[%d] = %+v, want %+v (order should be sorted by name)", i, got[i], want[i])
		}
	}
}

func TestYamlCategoryProvider_Get(t *testing.T) {
	path := writeCategoriesFile(t, `
categories:
  - name: Groceries
    description: Food and household
`)

	provider, err := NewYamlCategoryProvider(path)
	if err != nil {
		t.Fatalf("NewYamlCategoryProvider() error = %v", err)
	}

	tests := []struct {
		name    string
		lookup  string
		want    Category
		wantErr error
	}{
		{
			name:   "found",
			lookup: "Groceries",
			want:   Category{Name: "Groceries", Description: "Food and household"},
		},
		{
			name:    "not found",
			lookup:  "Unknown",
			wantErr: ErrCategoryNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := provider.Get(context.Background(), tt.lookup)
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Fatalf("Get() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Get() unexpected error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Get() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
