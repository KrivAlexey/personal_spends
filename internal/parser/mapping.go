package parser

import (
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
	BankName string            `yaml:"bankName"`
	Mappings map[string]string `yaml:"mappings"`
}

type BankMappingProvider interface {
	GetMapping(bankName string) (map[string]string, error)
}

type FileMappingProvider struct {
	mappings map[string]BankMapping
}

var _ BankMappingProvider = (*FileMappingProvider)(nil)

func NewFileMappingProvider(filepath string) (*FileMappingProvider, error) {
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

func (provider *FileMappingProvider) GetMapping(bankName string) (map[string]string, error) {
	v, ok := provider.mappings[bankName]
	if ok {
		return v.Mappings, nil
	}
	return nil, ErrBankMappingNotFound
}
