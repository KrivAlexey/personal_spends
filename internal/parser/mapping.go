package parser

import (
	"context"
	"errors"
	"fmt"
	"os"

	"go.yaml.in/yaml/v4"
)

var ErrBankMappingNotFound = errors.New("parser: bank mapping not found")

type BankMappingList struct {
	AllMappings []BankMapping `yaml:"allMappings"`
}

type BankMapping struct {
	BankName   string            `yaml:"bankName"`
	DateFormat string            `yaml:"dateFormat"`
	Mappings   map[string]string `yaml:"mappings"`
}

type BankMappingProvider interface {
	GetMapping(ctx context.Context, bankName string) (BankMapping, error)
}

type FileMappingProvider struct {
	mappings map[string]BankMapping
}

var _ BankMappingProvider = (*FileMappingProvider)(nil)

func NewFileMappingProvider(ctx context.Context, filepath string) (*FileMappingProvider, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("reading bank mapping file %s: %w", filepath, err)
	}

	var allMappings BankMappingList
	content, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("reading bank mapping file %s: %w", filepath, err)
	}

	err = yaml.Unmarshal(content, &allMappings)
	if err != nil {
		return nil, fmt.Errorf("parsing bank mapping file %s: %w", filepath, err)
	}

	mappings := make(map[string]BankMapping)
	for _, mapping := range allMappings.AllMappings {
		mappings[mapping.BankName] = mapping
	}

	return &FileMappingProvider{
		mappings: mappings,
	}, nil
}

func (provider *FileMappingProvider) GetMapping(ctx context.Context, bankName string) (BankMapping, error) {
	v, ok := provider.mappings[bankName]
	if ok {
		return v, nil
	}
	return BankMapping{}, ErrBankMappingNotFound
}
