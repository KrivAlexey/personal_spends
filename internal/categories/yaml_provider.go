package categories

import (
	"context"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

type yamlCategory struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func (c yamlCategory) toCategory() Category {
	return Category{
		Name:        c.Name,
		Description: c.Description,
	}
}

type yamlCategories struct {
	Categories []yamlCategory `yaml:"categories"`
}

type YamlCategoryProvider struct {
	filePath   string
	categories map[string]Category
}

var _ CategoryProvider = (*YamlCategoryProvider)(nil)

func NewYamlCategoryProvider(filePath string) (*YamlCategoryProvider, error) {
	yamlProvider := &YamlCategoryProvider{
		filePath:   filePath,
		categories: make(map[string]Category),
	}

	err := yamlProvider.loadAllCategories()
	if err != nil {
		return nil, fmt.Errorf("YamlCategoryProvider initialization err: %w", err)
	}

	return yamlProvider, nil
}

func (provider *YamlCategoryProvider) List(ctx context.Context) ([]Category, error) {
	categories := slices.Collect(maps.Values(provider.categories))
	slices.SortFunc(categories, func(a, b Category) int {
		return strings.Compare(a.Name, b.Name)
	})
	return categories, nil
}

func (provider *YamlCategoryProvider) Get(ctx context.Context, name string) (Category, error) {
	category, ok := provider.categories[name]
	if !ok {
		return Category{}, ErrCategoryNotFound
	}

	return category, nil
}

func (provider *YamlCategoryProvider) loadAllCategories() error {
	var parsed yamlCategories

	data, err := os.ReadFile(provider.filePath)
	if err != nil {
		return fmt.Errorf("could not read categories file: %v. error:%w", provider.filePath, err)
	}

	err = yaml.Unmarshal(data, &parsed)
	if err != nil {
		return fmt.Errorf("can't parse input yaml categories file: %v. error: %w", provider.filePath, err)
	}

	for _, cat := range parsed.Categories {
		boCategory := cat.toCategory()
		provider.categories[boCategory.Name] = boCategory
	}

	return nil
}
