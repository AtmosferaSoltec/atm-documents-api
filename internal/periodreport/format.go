package periodreport

import (
	"strings"
	"time"
)

var months = [...]string{"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "set", "oct", "nov", "dic"}

// Date formats a calendar date as "2 oct 2026".
func Date(t time.Time) string {
	return itoa(t.Day()) + " " + months[t.Month()-1] + " " + itoa(t.Year())
}

// Period formats the header range: "1 oct 2026 - 31 oct 2026".
func Period(from, to time.Time) string { return Date(from) + " - " + Date(to) }

// Symbol is the printed prefix of a currency: "S/" for PEN, "US$" for USD and
// the ISO code for anything else.
func Symbol(code string) string {
	switch code {
	case "PEN":
		return "S/"
	case "USD":
		return "US$"
	default:
		return code
	}
}

// Money formats a validated decimal string as "S/ 1,245.80" ("-S/ 15.00" when negative).
func Money(code, amount string) string {
	sign := ""
	if strings.HasPrefix(amount, "-") {
		sign, amount = "-", amount[1:]
	}
	whole, cents, _ := strings.Cut(amount, ".")
	var grouped strings.Builder
	for i, digit := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(digit)
	}
	return sign + Symbol(code) + " " + grouped.String() + "." + cents
}

// Percent formats a validated percentage string as "37.2%".
func Percent(value string) string { return value + "%" }

// FileName is the download name; it is built only from validated dates.
func FileName(from, to time.Time) string {
	return "orbita-" + from.Format(DateLayout) + "_" + to.Format(DateLayout) + ".pdf"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for ; n > 0; n /= 10 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
	}
	return string(digits)
}
