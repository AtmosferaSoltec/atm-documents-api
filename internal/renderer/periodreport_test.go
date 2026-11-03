package renderer

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AtmosferaSoltec/atm-documents-api/internal/periodreport"
)

func ranking(n int, name string) []periodreport.Line {
	out := make([]periodreport.Line, n)
	for i := range out {
		out[i] = periodreport.Line{Name: fmt.Sprintf("%s %d", name, i+1), Total: fmt.Sprintf("%d.50", 1000-i*10), Percent: fmt.Sprintf("%d.%d", 40-i%40, i%10)}
	}
	return out
}

func sampleReport(currencies, rows int) periodreport.Report {
	r := periodreport.Report{
		From: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC),
	}
	codes := []string{"PEN", "USD", "EUR", "CLP", "BRL", "MXN", "COP", "ARS", "CAD", "GBP", "JPY", "CHF"}
	for i := 0; i < currencies; i++ {
		r.Currencies = append(r.Currencies, periodreport.Currency{
			Code: codes[i], Income: "3200.00", Expense: "1954.20", Balance: "1245.80",
			Expenses: ranking(rows, "Categoría"), Incomes: ranking(rows, "Fuente"),
		})
	}
	return r
}

func TestPeriodReportRenders(t *testing.T) {
	out, err := NewPeriodReport().Render(sampleReport(2, 5))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Fatal("not a PDF")
	}
	if len(out) < 2_000 || len(out) > 200_000 {
		t.Fatalf("implausible size %d", len(out))
	}
	if n := pages(out); n != 1 {
		t.Fatalf("pages = %d, want 1", n)
	}
}

func TestPeriodReportPaginatesAndKeepsLimits(t *testing.T) {
	out, err := NewPeriodReport().Render(sampleReport(12, 50))
	if err != nil {
		t.Fatal(err)
	}
	if n := pages(out); n < 2 || n > maxReportPages {
		t.Fatalf("pages = %d", n)
	}
}

func TestPeriodReportEmptyRankingsAndNegativeBalance(t *testing.T) {
	r := sampleReport(1, 0)
	r.Currencies[0].Balance = "-15.00"
	if _, err := NewPeriodReport().Render(r); err != nil {
		t.Fatal(err)
	}
}

func TestPeriodReportSurvivesHostileText(t *testing.T) {
	r := sampleReport(1, 2)
	r.Currencies[0].Expenses[0].Name = strings.Repeat("W", 80) + "\x00\x07 日本語"
	r.Currencies[0].Incomes[0].Name = strings.Repeat("palabra ", 10)
	if _, err := NewPeriodReport().Render(r); err != nil {
		t.Fatal(err)
	}
}

func TestPeriodReportPreview(t *testing.T) {
	dir := os.Getenv("PREVIEW_DIR")
	if dir == "" {
		t.Skip("set PREVIEW_DIR to write sample PDFs")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, r := range map[string]periodreport.Report{
		"report-short": sampleReport(2, 6), "report-long": sampleReport(3, 50),
	} {
		out, err := NewPeriodReport().Render(r)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".pdf"), out, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
