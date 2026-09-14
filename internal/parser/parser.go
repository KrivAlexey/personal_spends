package parser

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/KrivAlexey/personal_spends/internal/categorizer"
)

type Parser struct {
	mappingProvider BankMappingProvider
}

func NewParser(mappingProvider BankMappingProvider) *Parser {
	return &Parser{mappingProvider: mappingProvider}
}

func (parser *Parser) ParseCSV(ctx context.Context, bankName string, r io.Reader) ([]categorizer.Transaction, error) {
	mapping, err := parser.mappingProvider.GetMapping(ctx, bankName)
	if err != nil {
		return nil, fmt.Errorf("parser: csv schema is not found for bank: %s: %w", bankName, err) // todo detect and save schema for a new Bank
	}

	csvReader := csv.NewReader(r)
	header, err := csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("parser: unable to read a header row: %w", err)
	}

	fieldByCol := make([]string, len(header))
	for i, col := range header {
		fieldByCol[i] = mapping.Mappings[col]
	}

	return ParseWithMapping(ctx, bankName, mapping.DateFormat, fieldByCol, csvReader)
}

func ParseWithMapping(ctx context.Context, bankName string, dateFormat string, fieldByCol []string, r *csv.Reader) ([]categorizer.Transaction, error) {
	var transactions []categorizer.Transaction
	for {
		if err := ctx.Err(); err != nil {
			return transactions, fmt.Errorf("reading csv: %w", err)
		}

		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return transactions, fmt.Errorf("reading csv row: %w", err)
		}

		if len(row) != len(fieldByCol) {
			return transactions, fmt.Errorf("row has %d fields, expected %d (bank %s)", len(row), len(fieldByCol), bankName)
		}

		var tx categorizer.Transaction
		for i, value := range row {
			switch fieldByCol[i] {
			case "Date":
				tx.Date, err = time.Parse(dateFormat, value)
			case "Merchant":
				tx.Merchant = value
			case "Amount":
				tx.Amount, err = strconv.ParseFloat(value, 64)
			case "Currency":
				tx.Currency = value
			case "Description":
				tx.Description = value
			}
			if err != nil {
				return transactions, fmt.Errorf("parsing column %q: %w", fieldByCol[i], err)
			}
		}

		tx.Source = bankName
		transactions = append(transactions, tx)
	}
	return transactions, nil
}
