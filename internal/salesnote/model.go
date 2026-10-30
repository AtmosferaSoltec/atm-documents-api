package salesnote

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg" // registers the JPEG decoder for image.DecodeConfig
	_ "image/png"  // registers the PNG decoder for image.DecodeConfig
	"math"
	"regexp"
	"time"
	"unicode/utf8"
)

// Format is the paper format of the printed note.
type Format string

const (
	// FormatA4 is the full-page note.
	FormatA4 Format = "a4"
	// FormatTicket is the 80 mm thermal strip.
	FormatTicket Format = "ticket"
)

// Limits protect the renderer from oversized input.
const (
	MaxItemsA4     = 500
	MaxItemsTicket = 150
	MaxLogoBytes   = 2 << 20
	// MaxLogoPixels caps the decoded logo size (width x height).
	MaxLogoPixels = 8_000_000
	// MaxAmount is the largest money value; it keeps amounts inside their table columns.
	MaxAmount = 99_999_999.99
)

// Company is the issuer printed in the header.
type Company struct {
	Name      string
	LegalName string
	TaxID     string
	Address   string
	Phone     string
	Email     string
	// Note is the company's closing message; empty means none.
	Note string
	// Logo holds PNG or JPEG bytes; nil means no logo.
	Logo []byte
}

// Customer is the buyer. Document is one text such as "DNI 45872310".
type Customer struct {
	Name     string
	Document string
	Address  string
}

// Item is one line of the note.
type Item struct {
	Name         string
	Presentation string
	Quantity     float64
	UnitPrice    float64
	LineTotal    float64
}

// Note is everything needed to print an internal sales note. The renderer
// queries nothing: it draws exactly what it receives.
type Note struct {
	Format        Format
	Company       Company
	Code          string
	IssuedAt      time.Time
	Customer      Customer
	Items         []Item
	TaxableAmount float64
	TaxAmount     float64
	Total         float64
	SoldBy        string
	// IsVoided marks the note as annulled; it is still printed.
	IsVoided bool
	// IsTest marks a historical test document with the "no value" stamp.
	IsTest bool
}

// ValidationError names the field that failed validation.
type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Reason }

var codePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,31}$`)

// Validate checks the note against the service limits.
func (n Note) Validate() error {
	if n.Format != FormatA4 && n.Format != FormatTicket {
		return invalid("format", "must be a4 or ticket")
	}
	if !codePattern.MatchString(n.Code) {
		return invalid("code", "must be 1-32 characters: letters, digits, '.', '_', '-' or '/'")
	}
	if n.IssuedAt.IsZero() {
		return invalid("issued_at", "is required")
	}
	if err := n.Company.validate(); err != nil {
		return err
	}
	for field, check := range map[string]struct {
		value string
		max   int
	}{
		"customer.name":     {n.Customer.Name, 200},
		"customer.document": {n.Customer.Document, 60},
		"customer.address":  {n.Customer.Address, 300},
		"sold_by":           {n.SoldBy, 120},
	} {
		if err := text(field, check.value, check.max, false); err != nil {
			return err
		}
	}
	maxItems := MaxItemsA4
	if n.Format == FormatTicket {
		maxItems = MaxItemsTicket
	}
	if len(n.Items) == 0 {
		return invalid("items", "at least one item is required")
	}
	if len(n.Items) > maxItems {
		return invalid("items", fmt.Sprintf("at most %d items for format %s", maxItems, n.Format))
	}
	for i, item := range n.Items {
		prefix := fmt.Sprintf("items[%d]", i)
		if err := text(prefix+".name", item.Name, 300, true); err != nil {
			return err
		}
		if err := text(prefix+".presentation", item.Presentation, 100, false); err != nil {
			return err
		}
		if !finite(item.Quantity) || item.Quantity <= 0 || item.Quantity > 10_000_000 {
			return invalid(prefix+".quantity", "must be greater than 0")
		}
		if err := money(prefix+".unit_price", item.UnitPrice); err != nil {
			return err
		}
		if err := money(prefix+".line_total", item.LineTotal); err != nil {
			return err
		}
	}
	for field, value := range map[string]float64{
		"taxable_amount": n.TaxableAmount,
		"tax_amount":     n.TaxAmount,
		"total":          n.Total,
	} {
		if err := money(field, value); err != nil {
			return err
		}
	}
	return nil
}

func (c Company) validate() error {
	for field, check := range map[string]struct {
		value    string
		max      int
		required bool
	}{
		"company.name":       {c.Name, 200, true},
		"company.legal_name": {c.LegalName, 200, true},
		"company.tax_id":     {c.TaxID, 20, true},
		"company.address":    {c.Address, 300, false},
		"company.phone":      {c.Phone, 40, false},
		"company.email":      {c.Email, 120, false},
		"company.note":       {c.Note, 1000, false},
	} {
		if err := text(field, check.value, check.max, check.required); err != nil {
			return err
		}
	}
	if len(c.Logo) == 0 {
		return nil
	}
	if len(c.Logo) > MaxLogoBytes {
		return invalid("company.logo_base64", fmt.Sprintf("must be at most %d bytes", MaxLogoBytes))
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(c.Logo))
	if err != nil || (format != "png" && format != "jpeg") {
		return invalid("company.logo_base64", "must be a PNG or JPEG image")
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width*cfg.Height > MaxLogoPixels {
		return invalid("company.logo_base64", "image dimensions are too large")
	}
	return nil
}

func text(field, value string, max int, required bool) error {
	if required && value == "" {
		return invalid(field, "is required")
	}
	if !utf8.ValidString(value) {
		return invalid(field, "must be valid UTF-8")
	}
	if utf8.RuneCountInString(value) > max {
		return invalid(field, fmt.Sprintf("must be at most %d characters", max))
	}
	return nil
}

func money(field string, value float64) error {
	if !finite(value) || value < 0 || value > MaxAmount {
		return invalid(field, "must be between 0 and 99999999.99")
	}
	return nil
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func invalid(field, reason string) error { return &ValidationError{Field: field, Reason: reason} }
