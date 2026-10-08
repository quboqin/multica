package collectionimport

import (
	"bytes"
	"context"
	"fmt"
	"github.com/xuri/excelize/v2"
	"strings"
	"testing"
	"time"
)

func TestExcelPreviewKeepsWorkbookValues(t *testing.T) {
	book := excelize.NewFile()
	defer book.Close()
	_, _ = book.NewSheet("Customers")
	header := []any{"Customer", "Seats", "Date", "Signed", "Website", "Code", "Formula", "Mixed", "Mixed"}
	_ = book.SetSheetRow("Customers", "A1", &header)
	row := []any{"客户, 一", 25, time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC), true, "https://example.com", "000123", "", "12", "a"}
	_ = book.SetSheetRow("Customers", "A2", &row)
	style, _ := book.NewStyle(&excelize.Style{NumFmt: 14})
	_ = book.SetCellStyle("Customers", "C2", "C2", style)
	_ = book.SetCellDefault("Customers", "G2", "")
	_ = book.SetCellFormula("Customers", "G2", "SUM(B2,1)")
	row = []any{"Second", 26, "2026-09-22", false, "https://example.com/b", "123456789012345678", "", "not a number", "b"}
	_ = book.SetSheetRow("Customers", "A3", &row)
	data, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	empty, err := Read(t.Context(), "customers.xlsx", bytes.NewReader(data.Bytes()), "", 1)
	if err != nil || len(empty.Sheets) != 2 || empty.RowCount != 0 {
		t.Fatalf("empty first sheet must remain selectable: %+v %v", empty, err)
	}
	p, err := Read(t.Context(), "customers.xlsx", bytes.NewReader(data.Bytes()), "Customers", 1)
	if err != nil {
		t.Fatal(err)
	}
	if p.RowCount != 2 || p.FormulaCount != 1 || p.Rows[0][2] != "2026-09-21" || p.Rows[0][6] != "=SUM(B2,1)" {
		t.Fatalf("lost values: %+v", p)
	}
	for i, want := range []string{"text", "number", "date", "checkbox", "url", "text", "text", "text", "text"} {
		if p.Columns[i].Type != want {
			t.Fatalf("column %d: %s want %s", i, p.Columns[i].Type, want)
		}
	}
	if p.Columns[8].Name != "Mixed (2)" {
		t.Fatal(p.Columns)
	}
}

func TestCSVInferenceBoundsAndHeaderSelection(t *testing.T) {
	p, err := Read(t.Context(), "data.csv", strings.NewReader("report\nName,Name,,Number\n\nfirst,00123,,7\nsecond,99999,,8\n"), "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if p.RowCount != 2 || p.Columns[1].Name != "Name (2)" || p.Columns[2].Name != "Column 3" || p.Columns[1].Type != "text" || p.RowNumbers[0] != 4 {
		t.Fatalf("%+v", p)
	}
	for _, value := range []string{"00042", "123456789012345678", "2026/01/02", "2026-01-02 12:00", "1,200", "$12.50", " 42 "} {
		if InferType([]string{value}) != "text" {
			t.Fatalf("coerced %q", value)
		}
	}
	data := "Name,Number\n" + strings.Repeat("row,42\n", 10000)
	p, err = Read(t.Context(), "data.csv", strings.NewReader(data), "", 1)
	if err != nil || p.RowCount != 10000 {
		t.Fatalf("10000 rows: %v", err)
	}
	if _, err = Read(t.Context(), "data.csv", strings.NewReader(data+"extra,43\n"), "", 1); err == nil {
		t.Fatal("accepted 10001 rows")
	}
	if _, err = Read(t.Context(), "data.xls", strings.NewReader("bad"), "", 1); err == nil {
		t.Fatal("accepted unsupported workbook")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = Read(ctx, "data.csv", strings.NewReader(data), "", 1); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestExcel10000Rows20Fields(t *testing.T) {
	book := excelize.NewFile()
	defer book.Close()
	header := []any{"Customer"}
	for i := 0; i < 20; i++ {
		header = append(header, fmt.Sprintf("Field %d", i))
	}
	_ = book.SetSheetRow("Sheet1", "A1", &header)
	for n := 0; n < 10000; n++ {
		row := []any{fmt.Sprintf("Customer %d", n)}
		for i := 0; i < 20; i++ {
			row = append(row, n+i)
		}
		_ = book.SetSheetRow("Sheet1", fmt.Sprintf("A%d", n+2), &row)
	}
	data, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	p, err := Read(t.Context(), "customers.xlsx", data, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if p.RowCount != 10000 || len(p.Columns) != 21 || p.Rows[9999][20] != "10018" {
		t.Fatalf("incomplete Excel import: rows %d columns %d", p.RowCount, len(p.Columns))
	}
}
