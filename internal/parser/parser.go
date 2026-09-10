package parser

import (
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

func (parser *Parser) ParseCSV(bankName string, r io.Reader) ([]categorizer.Transaction, error) {
	mapping, err := parser.mappingProvider.GetMapping(bankName)
	if err != nil {
		return nil, err // todo detect and save schema for a new Bank
	}

	csvReader := csv.NewReader(r)
	header, err := csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("parser: unable to read a header row: %w", err)
	}

	fieldByCol := make([]string, len(header))
	for i, col := range header {
		fieldByCol[i] = mapping[col]
	}

	return ParseWithMapping(bankName, fieldByCol, csvReader)
}

func ParseWithMapping(bankName string, fieldByCol []string, r *csv.Reader) ([]categorizer.Transaction, error) {
	var transactions []categorizer.Transaction
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading csv row: %w", err)
		}

		var tx categorizer.Transaction
		for i, value := range row {
			switch fieldByCol[i] {
			case "Date":
				tx.Date, err = time.Parse(time.DateTime, value)
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
				return nil, fmt.Errorf("parsing column %q: %w", fieldByCol[i], err)
			}
		}

		tx.Source = bankName
		transactions = append(transactions, tx)
	}
	return transactions, nil
}
