// Package catalog describes the documents this service can render and serves
// them to the console. Templates are code in this repository, so the catalog is
// a fixed list, not something callers can create or edit.
package catalog

import (
	"time"

	"github.com/AtmosferaSoltec/atm-documents-api/internal/salesnote"
)

// Format is one paper format a template supports.
type Format struct {
	Code  string `json:"code"`
	Label string `json:"label"`
	Paper string `json:"paper"`
}

// Limits are the caps the service enforces for a template.
type Limits struct {
	MaxItems     map[string]int `json:"max_items"`
	MaxBodyBytes int            `json:"max_body_bytes"`
	MaxLogoBytes int            `json:"max_logo_bytes"`
	MaxAmount    float64        `json:"max_amount"`
}

// Template is one document type.
type Template struct {
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Endpoint    string   `json:"endpoint"`
	Formats     []Format `json:"formats"`
	Limits      Limits   `json:"limits"`
}

// Service is how the running service is configured.
type Service struct {
	Environment          string   `json:"environment"`
	AllowedSystems       []string `json:"allowed_systems"`
	MaxConcurrentRenders int      `json:"max_concurrent_renders"`
	RenderTimeoutSeconds int      `json:"render_timeout_seconds"`
}

// Listing is the body of the catalog endpoint.
type Listing struct {
	Service   Service    `json:"service"`
	Templates []Template `json:"templates"`
}

// Settings are the configuration values the catalog reports.
type Settings struct {
	Environment          string
	AllowedSystems       []string
	MaxConcurrentRenders int
	RenderTimeout        time.Duration
}

// Templates lists every document type, in display order.
func Templates() []Template {
	return []Template{{
		Code:        salesnote.TemplateCode,
		Name:        "Nota de venta",
		Description: "Documento interno de venta. No es un comprobante de pago: las boletas, facturas y guías las emite Nubefact.",
		Endpoint:    "POST /v1/documents/sales-note",
		Formats: []Format{
			{Code: string(salesnote.FormatA4), Label: "A4", Paper: "Hoja A4, con paginación"},
			{Code: string(salesnote.FormatTicket), Label: "Ticket", Paper: "Tira térmica de 80 mm"},
		},
		Limits: Limits{
			MaxItems: map[string]int{
				string(salesnote.FormatA4):     salesnote.MaxItemsA4,
				string(salesnote.FormatTicket): salesnote.MaxItemsTicket,
			},
			MaxBodyBytes: salesnote.MaxBodyBytes,
			MaxLogoBytes: salesnote.MaxLogoBytes,
			MaxAmount:    salesnote.MaxAmount,
		},
	}}
}

// Find returns the template with the given code.
func Find(code string) (Template, bool) {
	for _, t := range Templates() {
		if t.Code == code {
			return t, true
		}
	}
	return Template{}, false
}

// Supports reports whether the template offers a format.
func (t Template) Supports(format string) bool {
	for _, f := range t.Formats {
		if f.Code == format {
			return true
		}
	}
	return false
}
