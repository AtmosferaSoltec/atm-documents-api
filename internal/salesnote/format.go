package salesnote

import (
	"math"
	"strconv"
	"strings"
	"time"
)

// lima is fixed at UTC-5: Peru has no daylight saving time, and a fixed zone
// avoids depending on tzdata being present in the container image.
var lima = time.FixedZone("America/Lima", -5*60*60)

// Amount formats a money value as "1,234.50", without the currency.
func Amount(value float64) string {
	cents := int64(math.Round(math.Abs(value) * 100))
	whole := strconv.FormatInt(cents/100, 10)
	var grouped strings.Builder
	for i, digit := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(digit)
	}
	sign := ""
	if value < 0 && cents > 0 {
		sign = "-"
	}
	return sign + grouped.String() + "." + strconv.FormatInt(cents%100/10, 10) + strconv.FormatInt(cents%10, 10)
}

// Money formats a money value as "S/ 1,234.50".
func Money(value float64) string { return "S/ " + Amount(value) }

// Quantity prints a quantity without trailing zeros: 3 -> "3", 1.5 -> "1.5".
func Quantity(value float64) string { return strconv.FormatFloat(value, 'f', -1, 64) }

// IssueDate formats the date as "13/10/2026", in Lima time.
func IssueDate(t time.Time) string { return t.In(lima).Format("02/01/2006") }

// IssueTime formats the time as "10:45 a. m.", in Lima time.
func IssueTime(t time.Time) string {
	local := t.In(lima)
	hour := local.Hour() % 12
	if hour == 0 {
		hour = 12
	}
	meridiem := "a. m."
	if local.Hour() >= 12 {
		meridiem = "p. m."
	}
	return strconv.Itoa(hour/10) + strconv.Itoa(hour%10) + ":" + local.Format("04") + " " + meridiem
}

// SplitDocument turns "DNI 45872310" into ("DNI", "45872310"). A document
// with no label comes back as ("DOC.", number); an empty one as ("DOC.", "").
func SplitDocument(document string) (label, number string) {
	parts := strings.Fields(document)
	switch len(parts) {
	case 0:
		return "DOC.", ""
	case 1:
		return "DOC.", parts[0]
	default:
		return strings.ToUpper(parts[0]), strings.Join(parts[1:], " ")
	}
}

// NoteLines splits the company message into trimmed, non-empty lines. The
// first one is usually a title, so renderers print it in bold.
func NoteLines(note string) []string {
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(note, "\r\n", "\n"), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// Credits is the footer line naming the system that generated the document.
func Credits(isTest bool) string {
	line := "Generado con Atmósfera · atmosferast.com"
	if isTest {
		return line + " · Documento de prueba"
	}
	return line
}
