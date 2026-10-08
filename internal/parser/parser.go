package parser

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/KrivAlexey/personal_spends/internal/categorizer"
)

type Parser struct {
	mappingProvider BankMappingProvider
}

func NewParser(mappingProvider BankMappingProvider) *Parser {
	return &Parser{mappingProvider: mappingProvider}
}

// ParseResult is what one export yields: its booked transactions and how many
// pending rows were skipped.
type ParseResult struct {
	Transactions   []categorizer.Transaction
	PendingSkipped int
}

func (parser *Parser) ParseCSV(ctx context.Context, bankName string, r io.Reader) (ParseResult, error) {
	mapping, err := parser.mappingProvider.GetMapping(ctx, bankName)
	if err != nil {
		return ParseResult{}, fmt.Errorf("parser: csv schema is not found for bank: %s: %w", bankName, err) // todo detect and save schema for a new Bank
	}

	if mapping.Encoding == "iso-8859-1" {
		r, err = decodeLatin1(r)
		if err != nil {
			return ParseResult{}, fmt.Errorf("parser: reading export: %w", err)
		}
	}

	csvReader := csv.NewReader(r)
	csvReader.Comma = mapping.Comma()
	header, err := csvReader.Read()
	if err != nil {
		return ParseResult{}, fmt.Errorf("parser: unable to read a header row: %w", err)
	}

	layout, err := newLayout(mapping, header)
	if err != nil {
		return ParseResult{}, err
	}

	return parseRows(ctx, mapping, layout, csvReader)
}

// decodeLatin1 converts ISO-8859-1 to UTF-8: each byte is the code point of
// the same value.
// ponytail: reads the whole export into memory (fine for monthly exports);
// 0x80–0x9F decode as C1 controls, not Windows-1252's € and quotes — switch to
// golang.org/x/text/encoding/charmap if a bank sends those.
func decodeLatin1(r io.Reader) (io.Reader, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	runes := make([]rune, len(raw))
	for i, b := range raw {
		runes[i] = rune(b)
	}
	return strings.NewReader(string(runes)), nil
}

// layout resolves a mapping against one export's header row.
type layout struct {
	fieldByCol  []string // Transaction field per column, "" if unmapped
	pendingCol  int      // -1 if the bank has no pending marker
	identityCol []int    // column indexes hashed into the transaction ID
}

func newLayout(mapping BankMapping, header []string) (layout, error) {
	colIndex := make(map[string]int, len(header))
	fieldByCol := make([]string, len(header))
	for i, col := range header {
		colIndex[col] = i
		fieldByCol[i] = mapping.Mappings[col]
	}

	l := layout{fieldByCol: fieldByCol, pendingCol: -1}
	if mapping.PendingColumn != "" {
		i, ok := colIndex[mapping.PendingColumn]
		if !ok {
			return layout{}, fmt.Errorf("parser: bank %s: header has no pending column %q", mapping.BankName, mapping.PendingColumn)
		}
		l.pendingCol = i
	}
	for _, col := range mapping.IdentityColumns {
		i, ok := colIndex[col]
		if !ok {
			return layout{}, fmt.Errorf("parser: bank %s: header has no identity column %q", mapping.BankName, col)
		}
		l.identityCol = append(l.identityCol, i)
	}
	return l, nil
}

// rowHash hashes the trimmed raw text of the identity columns, joined by the
// unit separator so that cell boundaries can't shift (ADR 0011).
func rowHash(row []string, identityCol []int) string {
	h := sha256.New()
	for i, col := range identityCol {
		if i > 0 {
			h.Write([]byte{0x1f})
		}
		h.Write([]byte(strings.TrimSpace(row[col])))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func parseRows(ctx context.Context, mapping BankMapping, l layout, r *csv.Reader) (ParseResult, error) {
	bankName := mapping.BankName
	var res ParseResult
	occurrences := make(map[string]int) // per file: identical rows get -0, -1, …
	for {
		if err := ctx.Err(); err != nil {
			return res, fmt.Errorf("reading csv: %w", err)
		}

		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return res, fmt.Errorf("reading csv row: %w", err)
		}

		if len(row) != len(l.fieldByCol) {
			return res, fmt.Errorf("row has %d fields, expected %d (bank %s)", len(row), len(l.fieldByCol), bankName)
		}

		if l.pendingCol >= 0 && strings.TrimSpace(row[l.pendingCol]) == mapping.PendingValue {
			res.PendingSkipped++
			continue
		}

		base := rowHash(row, l.identityCol)
		tx := categorizer.Transaction{ID: fmt.Sprintf("%s-%d", base, occurrences[base])}
		occurrences[base]++

		fieldByCol := l.fieldByCol
		for i, value := range row {
			switch fieldByCol[i] {
			case "Date":
				tx.Date, err = time.Parse(mapping.DateFormat, value)
			case "Merchant":
				tx.Merchant = value
			case "Amount":
				tx.Amount, err = parseAmount(value, mapping.DecimalSeparator)
			case "Currency":
				tx.Currency = value
			case "Description":
				tx.Description = value
			}
			if err != nil {
				return res, fmt.Errorf("parsing column %q: %w", fieldByCol[i], err)
			}
		}

		tx.Source = bankName
		res.Transactions = append(res.Transactions, tx)
	}
	return res, nil
}

// parseAmount reads a decimal number written with the bank's separator.
// With a comma separator the dots are thousands grouping ("1.234,56").
func parseAmount(value string, decimalSeparator string) (float64, error) {
	value = strings.TrimSpace(value)
	if decimalSeparator == "," {
		value = strings.ReplaceAll(value, ".", "")
		value = strings.Replace(value, ",", ".", 1)
	}
	return strconv.ParseFloat(value, 64)
}
