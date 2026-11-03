package periodreport_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/AtmosferaSoltec/atm-documents-api/internal/periodreport"
)

func day(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := periodreport.ParseDate("d", s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func validReport(t *testing.T) periodreport.Report {
	t.Helper()
	return periodreport.Report{
		From: day(t, "2026-10-01"),
		To:   day(t, "2026-10-31"),
		Currencies: []periodreport.Currency{{
			Code: "PEN", Income: "3200.00", Expense: "1954.20", Balance: "1245.80",
			Expenses: []periodreport.Line{{Name: "Comida", Total: "700.00", Percent: "35.8"}},
			Incomes:  []periodreport.Line{{Name: "Sueldo", Total: "3200.00", Percent: "100.0"}},
		}},
	}
}

func TestValidReportPasses(t *testing.T) {
	if err := validReport(t).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestParseDate(t *testing.T) {
	for _, bad := range []string{"", "2026-13-01", "2026-02-30", "2026-1-5", "01/10/2026", "2026-10-01T00:00:00Z", "1999-01-01", "2101-01-01"} {
		if _, err := periodreport.ParseDate("from", bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestValidateRejects(t *testing.T) {
	lines := func(n int) []periodreport.Line {
		out := make([]periodreport.Line, n)
		for i := range out {
			out[i] = periodreport.Line{Name: fmt.Sprint("c", i), Total: "1.00", Percent: "1.0"}
		}
		return out
	}
	cases := map[string]func(*periodreport.Report){
		"zero dates":        func(r *periodreport.Report) { r.From = time.Time{} },
		"to before from":    func(r *periodreport.Report) { r.To = r.From.AddDate(0, 0, -1) },
		"period too long":   func(r *periodreport.Report) { r.To = r.From.AddDate(0, 0, 366) },
		"no currencies":     func(r *periodreport.Report) { r.Currencies = nil },
		"too many":          func(r *periodreport.Report) { r.Currencies = make([]periodreport.Currency, 13) },
		"lowercase code":    func(r *periodreport.Report) { r.Currencies[0].Code = "pen" },
		"long code":         func(r *periodreport.Report) { r.Currencies[0].Code = "PENS" },
		"duplicate code":    func(r *periodreport.Report) { r.Currencies = append(r.Currencies, r.Currencies[0]) },
		"float income":      func(r *periodreport.Report) { r.Currencies[0].Income = "3200" },
		"one decimal":       func(r *periodreport.Report) { r.Currencies[0].Income = "3200.5" },
		"comma decimal":     func(r *periodreport.Report) { r.Currencies[0].Expense = "1,954.20" },
		"negative expense":  func(r *periodreport.Report) { r.Currencies[0].Expense = "-1.00" },
		"huge amount":       func(r *periodreport.Report) { r.Currencies[0].Income = "123456789.00" },
		"junk balance":      func(r *periodreport.Report) { r.Currencies[0].Balance = "1e3" },
		"51 expenses":       func(r *periodreport.Report) { r.Currencies[0].Expenses = lines(51) },
		"51 incomes":        func(r *periodreport.Report) { r.Currencies[0].Incomes = lines(51) },
		"empty name":        func(r *periodreport.Report) { r.Currencies[0].Expenses[0].Name = "" },
		"long name":         func(r *periodreport.Report) { r.Currencies[0].Expenses[0].Name = strings.Repeat("a", 81) },
		"invalid utf8":      func(r *periodreport.Report) { r.Currencies[0].Expenses[0].Name = "a\xffb" },
		"bad row total":     func(r *periodreport.Report) { r.Currencies[0].Incomes[0].Total = "abc" },
		"percent no dec":    func(r *periodreport.Report) { r.Currencies[0].Incomes[0].Percent = "37" },
		"percent too many":  func(r *periodreport.Report) { r.Currencies[0].Incomes[0].Percent = "37.25" },
		"percent over 100":  func(r *periodreport.Report) { r.Currencies[0].Incomes[0].Percent = "100.1" },
		"percent with sign": func(r *periodreport.Report) { r.Currencies[0].Incomes[0].Percent = "-1.0" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := validReport(t)
			mutate(&r)
			if r.Validate() == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestValidateAcceptsLimits(t *testing.T) {
	r := validReport(t)
	r.Currencies = nil
	for i := 0; i < periodreport.MaxCurrencies; i++ {
		c := validReport(t).Currencies[0]
		c.Code = string(rune('A'+i)) + "XX"
		c.Balance = "-15.00"
		c.Expenses = make([]periodreport.Line, periodreport.MaxRows)
		for j := range c.Expenses {
			c.Expenses[j] = periodreport.Line{Name: strings.Repeat("ñ", periodreport.MaxNameRunes), Total: "0.00", Percent: "0.0"}
		}
		r.Currencies = append(r.Currencies, c)
	}
	r.To = r.From.AddDate(0, 0, 365)
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestMoney(t *testing.T) {
	cases := []struct{ code, in, want string }{
		{"PEN", "1245.80", "S/ 1,245.80"},
		{"USD", "0.50", "US$ 0.50"},
		{"EUR", "1245.80", "EUR 1,245.80"},
		{"PEN", "1234567.89", "S/ 1,234,567.89"},
		{"PEN", "999.00", "S/ 999.00"},
		{"PEN", "-15.00", "-S/ 15.00"},
		{"USD", "-1000.00", "-US$ 1,000.00"},
	}
	for _, c := range cases {
		if got := periodreport.Money(c.code, c.in); got != c.want {
			t.Errorf("Money(%s, %s) = %q, want %q", c.code, c.in, got, c.want)
		}
	}
}

func TestDateAndPeriod(t *testing.T) {
	if got := periodreport.Date(day(t, "2026-10-02")); got != "2 oct 2026" {
		t.Errorf("Date = %q", got)
	}
	if got := periodreport.Date(day(t, "2026-09-30")); got != "30 set 2026" {
		t.Errorf("Date = %q", got)
	}
	if got := periodreport.Period(day(t, "2026-10-01"), day(t, "2026-10-31")); got != "1 oct 2026 - 31 oct 2026" {
		t.Errorf("Period = %q", got)
	}
}

func TestPercentAndFileName(t *testing.T) {
	if got := periodreport.Percent("37.2"); got != "37.2%" {
		t.Errorf("Percent = %q", got)
	}
	if got := periodreport.FileName(day(t, "2026-10-01"), day(t, "2026-10-31")); got != "orbita-2026-10-01_2026-10-31.pdf" {
		t.Errorf("FileName = %q", got)
	}
}
