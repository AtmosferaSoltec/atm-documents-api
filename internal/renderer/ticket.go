package renderer

import (
	"errors"
	"strings"

	"github.com/AtmosferaSoltec/atm-documents-api/internal/salesnote"
	"github.com/go-pdf/fpdf"
)

// The 80 mm thermal ticket.
//
// Black only: thermal paper prints no color or gray, and a light gray comes out
// as broken text. No boxes or borders either, because outlines print as empty
// rectangles; separators are rows of dashes, which always print.
//
// A thermal strip has no fixed height but a PDF page does, so the ticket is
// drawn twice: once on a very tall page to measure the content, and again on a
// page cut to exactly that height.

const (
	mm = 2.8346 // 1 mm in PDF points

	ticketWidth = 80 * mm
	// The roll is 80 mm but the print head covers about 72, so the side padding
	// leaves that gap: drawing to the edge would send the last letter of every
	// line where there is no ink.
	ticketPadX    = 4 * mm
	ticketPadY    = 5 * mm
	ticketContent = ticketWidth - 2*ticketPadX
	ticketSize    = 8.0
	ticketLH      = ticketSize * 1.25
	ticketQtyW    = 30.0
	ticketAmountW = 48.0
	ticketDescW   = ticketContent - ticketQtyW - ticketAmountW
	ticketItemGap = 3.0

	// measureHeight is the tallest page the measuring pass uses. PDF viewers
	// cap a page at 14400 points, so a ticket longer than that cannot print.
	measureHeight = 14400.0
)

var errTicketTooLong = errors.New("ticket is too long to print")

func renderTicket(n salesnote.Note) ([]byte, error) {
	height, err := measureTicket(n)
	if err != nil {
		return nil, err
	}
	out, _, err := drawTicket(n, height)
	return out, err
}

func measureTicket(n salesnote.Note) (float64, error) {
	_, used, err := drawTicket(n, measureHeight)
	if err != nil {
		return 0, err
	}
	if used+ticketPadY > measureHeight {
		return 0, errTicketTooLong
	}
	return used + ticketPadY, nil
}

type ticketDoc struct {
	c *canvas
	n salesnote.Note
	y float64
}

// drawTicket draws on a page of the given height and returns the y where the content ended.
func drawTicket(n salesnote.Note, pageHeight float64) ([]byte, float64, error) {
	pdf := fpdf.New("P", "pt", "A4", "")
	pdf.SetTitle(n.Code, true)
	pdf.SetAuthor(n.Company.LegalName, true)
	pdf.SetCreator("ATM-Documents-API", true)
	pdf.SetCreationDate(n.IssuedAt)
	pdf.AddPageFormat("P", fpdf.SizeType{Wd: ticketWidth, Ht: pageHeight})
	t := &ticketDoc{c: newCanvas(pdf), n: n, y: ticketPadY}
	t.c.textColor(black)
	t.c.stroke(black, 0.5)

	if err := t.draw(); err != nil {
		return nil, 0, err
	}
	out, err := t.c.output()
	return out, t.y, err
}

func (t *ticketDoc) draw() error {
	n, c := t.n, t.c
	const title = 10.0

	// ---- Company ----
	if len(n.Company.Logo) > 0 {
		const logoW, logoH, gap = 40 * mm, 14 * mm, 4.0
		if err := c.drawLogo("logo", n.Company.Logo, ticketPadX+(ticketContent-logoW)/2, t.y, logoW, logoH); err != nil {
			return err
		}
		t.y += logoH + gap
	}
	t.centered(title, true, strings.ToUpper(n.Company.LegalName))
	if !strings.EqualFold(n.Company.Name, n.Company.LegalName) {
		t.centered(ticketSize, false, n.Company.Name)
	}
	t.centered(ticketSize, true, "R.U.C. "+n.Company.TaxID)
	t.centered(ticketSize, false, n.Company.Address)
	if n.Company.Phone != "" {
		t.centered(ticketSize, false, "Telf.: "+n.Company.Phone)
	}
	if n.Company.Email != "" {
		t.centered(ticketSize, false, n.Company.Email)
	}
	t.divider()

	// ---- Document ----
	t.centered(title, true, "NOTA DE VENTA")
	t.centered(title, true, n.Code)
	if n.IsTest {
		t.testStamp()
	}
	t.divider()

	// ---- Issue date and customer ----
	t.dateLine(salesnote.IssueDate(n.IssuedAt), salesnote.IssueTime(n.IssuedAt))
	t.field("CLIENTE: ", n.Customer.Name)
	if label, number := salesnote.SplitDocument(n.Customer.Document); number != "" {
		t.field(label+": ", number)
	}
	if n.Customer.Address != "" {
		t.field("DIRECCIÓN: ", n.Customer.Address)
	}
	t.divider()

	// ---- Detail ----
	t.ink(ticketSize, true)
	c.line(ticketPadX, t.y, ticketQtyW, ticketLH, alignLeft, "CANT.")
	c.line(ticketPadX+ticketQtyW, t.y, ticketDescW, ticketLH, alignLeft, "DESCRIPCIÓN")
	c.line(ticketPadX+ticketQtyW+ticketDescW, t.y, ticketAmountW, ticketLH, alignRight, "IMPORTE")
	t.y += ticketLH
	for _, item := range n.Items {
		t.y += ticketItemGap
		t.ink(ticketSize, false)
		descW := ticketDescW - 4
		lines := c.wrap(item.Name, descW, descW)
		lines = append(lines, c.wrap(item.Presentation+"  P.U. "+salesnote.Amount(item.UnitPrice), descW, descW)...)
		c.line(ticketPadX, t.y, ticketQtyW-4, ticketLH, alignLeft, salesnote.Quantity(item.Quantity))
		c.lines(ticketPadX+ticketQtyW, t.y, descW, ticketLH, alignLeft, lines)
		c.line(ticketPadX+ticketQtyW+ticketDescW, t.y, ticketAmountW, ticketLH, alignRight, salesnote.Amount(item.LineTotal))
		t.y += float64(len(lines)) * ticketLH
	}
	t.divider()

	// ---- Totals ----
	t.spread(ticketSize, false, "OP. GRAVADA S/", salesnote.Amount(n.TaxableAmount))
	t.spread(ticketSize, false, "I.G.V. (18%) S/", salesnote.Amount(n.TaxAmount))
	t.y += 4
	t.spread(11, true, "TOTAL S/", salesnote.Amount(n.Total))
	t.y += 4
	t.field("SON: ", salesnote.AmountInWords(n.Total))
	if n.IsVoided {
		t.y += 6
		t.centered(14, true, "*** ANULADA ***")
	}
	if n.IsTest {
		t.testStamp()
	}
	t.divider()

	// ---- Footer: no page number, the ticket is one piece ----
	t.centered(ticketSize, false, "Atendido por: "+n.SoldBy)
	t.y += 3
	t.centered(ticketSize, true, "Documento interno.")
	t.centered(ticketSize, true, "No válido como comprobante de pago.")
	if notes := salesnote.NoteLines(n.Company.Note); len(notes) > 0 {
		t.divider()
		for i, line := range notes {
			t.centered(ticketSize, i == 0, line)
		}
	} else {
		t.y += 3
		t.centered(ticketSize, false, "¡Gracias por su compra!")
	}
	t.y += 3
	t.centered(6.5, false, salesnote.Credits(n.IsTest))
	return nil
}

func (t *ticketDoc) ink(size float64, bold bool) {
	t.c.font(size, bold)
	t.c.textColor(black)
}

// centered draws wrapped, centered text with 1.25 line spacing.
func (t *ticketDoc) centered(size float64, bold bool, text string) {
	t.ink(size, bold)
	lh := size * 1.25
	t.y += t.c.lines(ticketPadX, t.y, ticketContent, lh, alignCenter, t.c.wrap(text, ticketContent, ticketContent))
}

// field draws "LABEL value" with a bold label.
func (t *ticketDoc) field(label, value string) {
	t.ink(ticketSize, false)
	t.y += t.c.labeled(ticketPadX, t.y, ticketContent, ticketLH, label, value)
}

// spread draws one text at each end of the line.
func (t *ticketDoc) spread(size float64, bold bool, left, right string) {
	t.ink(size, bold)
	lh := size * 1.25
	t.c.line(ticketPadX, t.y, ticketContent, lh, alignLeft, left)
	t.c.line(ticketPadX, t.y, ticketContent, lh, alignRight, right)
	t.y += lh
}

// dateLine draws the date at the left end and the time at the right end, each
// with a bold label: "FECHA: 13/10/2026 ......... HORA: 10:45 a. m.".
func (t *ticketDoc) dateLine(date, clock string) {
	t.labelAt("FECHA: ", date, ticketPadX)
	c := t.c
	c.font(ticketSize, true)
	labelW := c.width("HORA: ")
	c.font(ticketSize, false)
	t.labelAt("HORA: ", clock, ticketPadX+ticketContent-labelW-c.width(clock))
	t.y += ticketLH
}

// labelAt draws a bold label followed by its value, starting at x.
func (t *ticketDoc) labelAt(label, value string, x float64) {
	c := t.c
	c.font(ticketSize, true)
	labelW := c.width(label)
	c.line(x, t.y, labelW, ticketLH, alignLeft, label)
	c.font(ticketSize, false)
	c.line(x+labelW, t.y, c.width(value), ticketLH, alignLeft, value)
}

// divider draws a row of dashes spanning the printable width.
func (t *ticketDoc) divider() {
	t.ink(ticketSize, false)
	dash := t.c.width("-")
	count := int(ticketContent / dash)
	t.y += 3
	t.c.line(ticketPadX, t.y, ticketContent, ticketLH, alignLeft, strings.Repeat("-", count))
	t.y += ticketLH + 3
}

// testStamp is the "no value" notice. A diagonal stamp does not fit on a strip,
// so it prints in the flow, below the number and again after the total.
func (t *ticketDoc) testStamp() {
	t.y += 6
	for _, line := range []string{"*** SIN VALOR ***", "DOCUMENTO DE PRUEBA", "NO VÁLIDO ANTE SUNAT"} {
		t.centered(11, true, line)
	}
}
