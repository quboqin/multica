// Package collectionimport reads tabular files without writing application data.
package collectionimport

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
)

const MaxRows = 10000
const MaxColumns = 256

type Column struct {
	Index   int      `json:"index"`
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Skip    bool     `json:"skip"`
	Samples []string `json:"samples"`
}
type Preview struct {
	Name         string     `json:"name"`
	Sheets       []string   `json:"sheets"`
	Sheet        string     `json:"sheet"`
	HeaderRow    int        `json:"header_row"`
	TitleColumn  int        `json:"title_column"`
	Columns      []Column   `json:"columns"`
	RowCount     int        `json:"row_count"`
	FormulaCount int        `json:"formula_count"`
	Rows         [][]string `json:"-"`
	RowNumbers   []int      `json:"-"`
}

// Read keeps displayed values (including leading zeroes and number formats).
// Formulae are never executed. A formula without a saved result stays text.
func Read(ctx context.Context, filename string, input io.Reader, sheet string, headerRow int) (*Preview, error) {
	if headerRow == 0 {
		headerRow = 1
	}
	if headerRow < 1 || headerRow > 100 {
		return nil, fmt.Errorf("header row must be between 1 and 100")
	}
	p := &Preview{Name: strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename)), HeaderRow: headerRow, TitleColumn: 0, Sheets: []string{}, Columns: []Column{}}
	var raw [][]string
	var sourceRows []int
	appendRow := func(row []string, source int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(raw) >= MaxRows+headerRow+100 {
			return fmt.Errorf("worksheet exceeds 10000 rows; remove unused trailing rows")
		}
		if len(row) > MaxColumns {
			return fmt.Errorf("worksheet exceeds 256 source columns")
		}
		for _, s := range row {
			if len(s) > 128*1024 {
				return fmt.Errorf("a cell exceeds 128 KiB")
			}
		}
		raw = append(raw, row)
		sourceRows = append(sourceRows, source)
		return nil
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".csv":
		reader := csv.NewReader(input)
		reader.FieldsPerRecord = -1
		for {
			row, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("invalid CSV: %w", err)
			}
			for _, s := range row {
				if !utf8.ValidString(s) {
					return nil, fmt.Errorf("CSV must use UTF-8")
				}
			}
			line, _ := reader.FieldPos(0)
			if err = appendRow(row, line); err != nil {
				return nil, err
			}
		}
	case ".xlsx":
		book, err := excelize.OpenReader(input, excelize.Options{UnzipSizeLimit: 64 * 1024 * 1024, UnzipXMLSizeLimit: 8 * 1024 * 1024, ShortDatePattern: "yyyy-mm-dd", LongDatePattern: "yyyy-mm-dd"})
		if err != nil {
			return nil, fmt.Errorf("cannot read Excel workbook: %w", err)
		}
		defer book.Close()
		p.Sheets = book.GetSheetList()
		if len(p.Sheets) == 0 {
			return nil, fmt.Errorf("workbook has no worksheets")
		}
		if sheet == "" {
			sheet = p.Sheets[0]
		}
		p.Sheet = sheet
		rows, err := book.Rows(sheet)
		if err != nil {
			return nil, fmt.Errorf("worksheet not found: %s", sheet)
		}
		defer rows.Close()
		rowNumber := 0
		for rows.Next() {
			rowNumber++
			row, err := rows.Columns()
			if err != nil {
				return nil, fmt.Errorf("cannot read row %d: %w", rowNumber, err)
			}
			if len(row) > MaxColumns {
				return nil, fmt.Errorf("worksheet exceeds 256 source columns")
			}
			for col := range row {
				axis, _ := excelize.CoordinatesToCellName(col+1, rowNumber)
				formula, err := book.GetCellFormula(sheet, axis)
				if err != nil {
					return nil, err
				}
				if formula != "" {
					p.FormulaCount++
					if row[col] == "" {
						row[col] = "=" + formula
					}
				}
			}
			if err = appendRow(row, rowNumber); err != nil {
				return nil, err
			}
		}
		if err = rows.Error(); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("choose an .xlsx or UTF-8 .csv file; save older .xls files as .xlsx first")
	}
	if len(raw) < headerRow {
		return p, nil
	}
	header := raw[headerRow-1]
	if len(header) > 0 {
		header[0] = strings.TrimPrefix(header[0], "\ufeff")
	}
	width := len(header)
	for rowIndex, row := range raw[headerRow:] {
		nonempty := false
		for _, v := range row {
			if v != "" {
				nonempty = true
				break
			}
		}
		if !nonempty {
			continue
		}
		if len(p.Rows) >= MaxRows {
			return nil, fmt.Errorf("import supports at most 10000 non-empty records")
		}
		p.Rows = append(p.Rows, row)
		p.RowNumbers = append(p.RowNumbers, sourceRows[rowIndex+headerRow])
		if len(row) > width {
			width = len(row)
		}
	}
	names := map[string]bool{}
	for i := 0; i < width; i++ {
		name := ""
		if i < len(header) {
			name = strings.TrimSpace(header[i])
		}
		if name == "" {
			name = fmt.Sprintf("Column %d", i+1)
		}
		name = truncate(name, 24)
		base := name
		for n := 2; names[strings.ToLower(name)]; n++ {
			name = fmt.Sprintf("%s (%d)", base, n)
		}
		names[strings.ToLower(name)] = true
		values := make([]string, len(p.Rows))
		for n, row := range p.Rows {
			if i < len(row) {
				values[n] = row[i]
			}
		}
		p.Columns = append(p.Columns, Column{Index: i, Name: name, Type: InferType(values), Samples: values[:min(5, len(values))]})
	}
	p.RowCount = len(p.Rows)
	p.Name = truncate(p.Name, 80)
	return p, nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// Conservative inference keeps ambiguous dates, mixed values, long identifiers,
// formatted numbers and leading zeroes as text instead of losing information.
func InferType(values []string) string {
	candidates := map[string]bool{"number": true, "checkbox": true, "date": true, "url": true}
	seen := false
	for _, s := range values {
		if s == "" {
			continue
		}
		seen = true
		for typ, ok := range candidates {
			if ok {
				if _, err := CellValue(typ, s); err != nil {
					candidates[typ] = false
				}
			}
		}
	}
	if seen {
		for _, typ := range []string{"checkbox", "number", "date", "url"} {
			if candidates[typ] {
				return typ
			}
		}
	}
	return "text"
}

func CellValue(typ, s string) (any, error) {
	switch typ {
	case "text", "select":
		return s, nil
	case "number":
		v, err := strconv.ParseFloat(s, 64)
		digits := 0
		for _, c := range s {
			if c >= '0' && c <= '9' {
				digits++
			}
		}
		unsigned := strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
		if err != nil || math.IsInf(v, 0) || math.IsNaN(v) || strings.TrimSpace(s) != s || digits > 15 || (len(unsigned) > 1 && unsigned[0] == '0' && unsigned[1] >= '0' && unsigned[1] <= '9') {
			return nil, fmt.Errorf("not a lossless number; use text to retain this value")
		}
		return v, nil
	case "checkbox":
		if strings.EqualFold(s, "true") {
			return true, nil
		}
		if strings.EqualFold(s, "false") {
			return false, nil
		}
		return nil, fmt.Errorf("expected true or false")
	case "date":
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return nil, fmt.Errorf("expected a YYYY-MM-DD date; use text to keep other formats")
		}
		return s, nil
	case "url":
		u, err := url.Parse(s)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, fmt.Errorf("expected an http or https URL")
		}
		return s, nil
	default:
		return nil, fmt.Errorf("unsupported import field type: %s", typ)
	}
}
