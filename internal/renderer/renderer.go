package renderer

import (
	"fmt"
	"strings"

	"github.com/AtmosferaSoltec/atm-documents-api/internal/salesnote"
)

// SalesNote renders internal sales notes with go-pdf/fpdf. It is the only
// package that imports the PDF library.
type SalesNote struct{}

// New returns the sales note renderer.
func New() *SalesNote { return &SalesNote{} }

// Render draws the note in its declared format.
func (*SalesNote) Render(note salesnote.Note) ([]byte, error) {
	note = sanitize(note)
	switch note.Format {
	case salesnote.FormatA4:
		return renderA4(note)
	case salesnote.FormatTicket:
		return renderTicket(note)
	default:
		return nil, fmt.Errorf("unsupported format %q", note.Format)
	}
}

// sanitize replaces control characters, which have no glyph, with spaces.
func sanitize(n salesnote.Note) salesnote.Note {
	n.Code = clean(n.Code)
	n.Company.Name = clean(n.Company.Name)
	n.Company.LegalName = clean(n.Company.LegalName)
	n.Company.TaxID = clean(n.Company.TaxID)
	n.Company.Address = clean(n.Company.Address)
	n.Company.Phone = clean(n.Company.Phone)
	n.Company.Email = clean(n.Company.Email)
	n.Company.Note = strings.Join(salesnote.NoteLines(cleanKeepNewlines(n.Company.Note)), "\n")
	n.Customer.Name = clean(n.Customer.Name)
	n.Customer.Document = clean(n.Customer.Document)
	n.Customer.Address = clean(n.Customer.Address)
	n.SoldBy = clean(n.SoldBy)
	items := make([]salesnote.Item, len(n.Items))
	for i, item := range n.Items {
		item.Name = clean(item.Name)
		item.Presentation = clean(item.Presentation)
		items[i] = item
	}
	n.Items = items
	return n
}

func clean(s string) string { return mapControl(s, func(r rune) bool { return r < 0x20 || r == 0x7f }) }

func cleanKeepNewlines(s string) string {
	return mapControl(s, func(r rune) bool { return (r < 0x20 && r != '\n' && r != '\r') || r == 0x7f })
}

func mapControl(s string, isControl func(rune) bool) string {
	return strings.Map(func(r rune) rune {
		if isControl(r) {
			return ' '
		}
		return r
	}, s)
}
