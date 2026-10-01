package parser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"unicode/utf8"

	"go.yaml.in/yaml/v4"
)

var ErrBankMappingNotFound = errors.New("parser: bank mapping not found")

type BankMappingList struct {
	AllMappings []BankMapping `yaml:"allMappings"`
}

type BankMapping struct {
	BankName   string `yaml:"bankName"`
	DateFormat string `yaml:"dateFormat"`
	// Delimiter is the CSV field separator, defaults to ",".
	Delimiter string `yaml:"delimiter"`
	// DecimalSeparator is "." or ",". With ",", amounts are read as German
	// style ("1.234,56"): dots are thousands separators and get stripped.
	DecimalSeparator string            `yaml:"decimalSeparator"`
	Mappings         map[string]string `yaml:"mappings"`
}

// Comma returns the delimiter as a rune for csv.Reader. Valid only after
// applyDefaults has accepted the mapping.
func (mapping BankMapping) Comma() rune {
	r, _ := utf8.DecodeRuneInString(mapping.Delimiter)
	return r
}

func (mapping *BankMapping) applyDefaults() error {
	if mapping.Delimiter == "" {
		mapping.Delimiter = ","
	}
	if mapping.DecimalSeparator == "" {
		mapping.DecimalSeparator = "."
	}
	if utf8.RuneCountInString(mapping.Delimiter) != 1 {
		return fmt.Errorf("bank %s: delimiter must be a single character, got %q", mapping.BankName, mapping.Delimiter)
	}
	if mapping.DecimalSeparator != "." && mapping.DecimalSeparator != "," {
		return fmt.Errorf("bank %s: decimalSeparator must be \".\" or \",\", got %q", mapping.BankName, mapping.DecimalSeparator)
	}
	return nil
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
		if err := mapping.applyDefaults(); err != nil {
			return nil, fmt.Errorf("parsing bank mapping file %s: %w", filepath, err)
		}
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
