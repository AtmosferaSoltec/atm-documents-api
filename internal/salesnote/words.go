package salesnote

import (
	"fmt"
	"math"
	"strings"
)

// The total in words, as Peruvian sales documents carry it:
// "CIENTO VEINTE CON 00/100 SOLES". The rules are short and fixed, so they are
// written here instead of pulling in a dependency.

var units = [...]string{
	"", "UNO", "DOS", "TRES", "CUATRO", "CINCO", "SEIS", "SIETE", "OCHO", "NUEVE",
	"DIEZ", "ONCE", "DOCE", "TRECE", "CATORCE", "QUINCE", "DIECISÉIS", "DIECISIETE",
	"DIECIOCHO", "DIECINUEVE", "VEINTE", "VEINTIUNO", "VEINTIDÓS", "VEINTITRÉS",
	"VEINTICUATRO", "VEINTICINCO", "VEINTISÉIS", "VEINTISIETE", "VEINTIOCHO", "VEINTINUEVE",
}

var tens = [...]string{"", "", "", "TREINTA", "CUARENTA", "CINCUENTA", "SESENTA", "SETENTA", "OCHENTA", "NOVENTA"}

var hundreds = [...]string{
	"", "CIENTO", "DOSCIENTOS", "TRESCIENTOS", "CUATROCIENTOS", "QUINIENTOS",
	"SEISCIENTOS", "SETECIENTOS", "OCHOCIENTOS", "NOVECIENTOS",
}

// underThousand writes 0-999.
func underThousand(n int) string {
	if n == 0 {
		return ""
	}
	if n == 100 {
		return "CIEN"
	}
	var parts []string
	if h := n / 100; h > 0 {
		parts = append(parts, hundreds[h])
	}
	rest := n % 100
	if rest < 30 {
		if rest > 0 {
			parts = append(parts, units[rest])
		}
	} else if u := rest % 10; u > 0 {
		parts = append(parts, tens[rest/10]+" Y "+units[u])
	} else {
		parts = append(parts, tens[rest/10])
	}
	return strings.Join(parts, " ")
}

// apocope shortens a trailing "UNO" (and "VEINTIUNO") before "MIL" and "MILLONES".
func apocope(words string) string {
	if strings.HasSuffix(words, "UNO") {
		return strings.TrimSuffix(words, "UNO") + "UN"
	}
	return words
}

// IntegerInWords writes a whole number up to 999,999,999.
func IntegerInWords(n int) string {
	if n == 0 {
		return "CERO"
	}
	millions := n / 1_000_000
	thousands := n % 1_000_000 / 1000
	rest := n % 1000
	var parts []string
	if millions > 0 {
		if millions == 1 {
			parts = append(parts, "UN MILLÓN")
		} else {
			parts = append(parts, apocope(underThousand(millions))+" MILLONES")
		}
	}
	if thousands > 0 {
		if thousands == 1 {
			parts = append(parts, "MIL")
		} else {
			parts = append(parts, apocope(underThousand(thousands))+" MIL")
		}
	}
	if rest > 0 {
		parts = append(parts, underThousand(rest))
	}
	return strings.Join(parts, " ")
}

// AmountInWords writes 120 as "CIENTO VEINTE CON 00/100 SOLES".
func AmountInWords(amount float64) string {
	cents := int(math.Round(amount * 100))
	return fmt.Sprintf("%s CON %02d/100 SOLES", IntegerInWords(cents/100), cents%100)
}
