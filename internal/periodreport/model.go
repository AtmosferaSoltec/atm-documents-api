// Package periodreport is the Orbita period report: the summary PDF of a date
// range with, per currency, the totals and the two rankings. The caller sends
// every figure already computed; this package only validates and formats.
package periodreport

import (
	"fmt"
	"regexp"
	"time"
	"unicode/utf8"
)

// TemplateCode identifies this document type in routes and in the activity log.
const TemplateCode = "period-report"

// Limits protect the renderer from oversized input.
const (
	MaxCurrencies = 12
	MaxRows       = 50
	MaxNameRunes  = 80
	// MaxPeriodDays bounds the period (inclusive) to a little over a year.
	MaxPeriodDays = 366
	// MaxBodyBytes bounds the request: 12 currencies x 100 rows is about 150 KiB.
	MaxBodyBytes = 512 << 10
)

// DateLayout is the only accepted date format.
const DateLayout = "2006-01-02"

var (
	nonNegativeMoney = regexp.MustCompile(`^\d{1,8}\.\d{2}$`)
	signedMoney      = regexp.MustCompile(`^-?\d{1,8}\.\d{2}$`)
	percentPattern   = regexp.MustCompile(`^(100\.0|\d{1,2}\.\d)$`)
	currencyPattern  = regexp.MustCompile(`^[A-Z]{3}$`)
)

// Line is one ranking row. Total and Percent are decimal strings ("350.00", "37.2").
type Line struct {
	Name    string
	Total   string
	Percent string
}

// Currency is the block of one currency. Money values are decimal strings with
// two decimals; Balance may be negative.
type Currency struct {
	Code     string
	Income   string
	Expense  string
	Balance  string
	Expenses []Line
	Incomes  []Line
}

// Report is everything needed to print the period report.
type Report struct {
	// From and To are calendar dates (midnight UTC), both inclusive.
	From       time.Time
	To         time.Time
	Currencies []Currency
}

// ValidationError names the field that failed validation.
type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Reason }

func invalid(field, reason string) error { return &ValidationError{Field: field, Reason: reason} }

// ParseDate parses a strict "YYYY-MM-DD" calendar date.
func ParseDate(field, value string) (time.Time, error) {
	t, err := time.Parse(DateLayout, value)
	if err != nil || t.Format(DateLayout) != value {
		return time.Time{}, invalid(field, "must be a valid date as YYYY-MM-DD")
	}
	if t.Year() < 2000 || t.Year() > 2100 {
		return time.Time{}, invalid(field, "must be between years 2000 and 2100")
	}
	return t, nil
}

// Validate checks the report against the service limits.
func (r Report) Validate() error {
	if r.From.IsZero() || r.To.IsZero() {
		return invalid("from", "from and to are required")
	}
	if r.To.Before(r.From) {
		return invalid("to", "must not be before from")
	}
	if days := int(r.To.Sub(r.From).Hours()/24) + 1; days > MaxPeriodDays {
		return invalid("to", fmt.Sprintf("the period must be at most %d days", MaxPeriodDays))
	}
	if len(r.Currencies) == 0 {
		return invalid("currencies", "at least one currency is required")
	}
	if len(r.Currencies) > MaxCurrencies {
		return invalid("currencies", fmt.Sprintf("at most %d currencies", MaxCurrencies))
	}
	seen := map[string]bool{}
	for i, c := range r.Currencies {
		prefix := fmt.Sprintf("currencies[%d]", i)
		if !currencyPattern.MatchString(c.Code) {
			return invalid(prefix+".currency", "must be a 3-letter uppercase code")
		}
		if seen[c.Code] {
			return invalid(prefix+".currency", "is repeated")
		}
		seen[c.Code] = true
		if !nonNegativeMoney.MatchString(c.Income) {
			return invalid(prefix+".income_total", "must be a decimal string like 1245.80")
		}
		if !nonNegativeMoney.MatchString(c.Expense) {
			return invalid(prefix+".expense_total", "must be a decimal string like 1245.80")
		}
		if !signedMoney.MatchString(c.Balance) {
			return invalid(prefix+".balance", "must be a decimal string like 1245.80 or -15.00")
		}
		if err := validateLines(prefix+".expenses", c.Expenses); err != nil {
			return err
		}
		if err := validateLines(prefix+".incomes", c.Incomes); err != nil {
			return err
		}
	}
	return nil
}

func validateLines(field string, lines []Line) error {
	if len(lines) > MaxRows {
		return invalid(field, fmt.Sprintf("at most %d rows", MaxRows))
	}
	for i, l := range lines {
		prefix := fmt.Sprintf("%s[%d]", field, i)
		if l.Name == "" {
			return invalid(prefix+".name", "is required")
		}
		if !utf8.ValidString(l.Name) {
			return invalid(prefix+".name", "must be valid UTF-8")
		}
		if utf8.RuneCountInString(l.Name) > MaxNameRunes {
			return invalid(prefix+".name", fmt.Sprintf("must be at most %d characters", MaxNameRunes))
		}
		if !nonNegativeMoney.MatchString(l.Total) {
			return invalid(prefix+".total", "must be a decimal string like 350.00")
		}
		if !percentPattern.MatchString(l.Percent) {
			return invalid(prefix+".percent", "must be a string with one decimal between 0.0 and 100.0")
		}
	}
	return nil
}

// Rows is the number of ranking rows in the report, for the activity log.
func (r Report) Rows() int {
	n := 0
	for _, c := range r.Currencies {
		n += len(c.Expenses) + len(c.Incomes)
	}
	return n
}
