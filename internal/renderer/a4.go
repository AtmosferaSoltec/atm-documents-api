package renderer

import (
	"errors"
	"fmt"
	"strings"

	"github.com/AtmosferaSoltec/atm-documents-api/internal/salesnote"
	"github.com/go-pdf/fpdf"
)

// The A4 sales note. Measures come from the layout La Vendimia printed before
// this service existed, so the document keeps its look.

const (
	a4Width   = 595.28
	a4Height  = 841.89
	a4MarginX = 36.0
	a4Top     = 32.0
	// a4Limit is the lowest y content may reach; the fixed footer lives below it.
	a4Limit   = a4Height - 70
	a4Content = a4Width - 2*a4MarginX
	a4Radius  = 6.0
	a4Border  = 0.8
	a4Text    = 8.5

	// maxA4Pages keeps a runaway payload from producing an enormous document.
	maxA4Pages = 20

	docBoxWidth = 200.0
	totalsWidth = 220.0
)

var errTooManyPages = errors.New("document exceeds the page limit")

type a4Column struct {
	label string
	width float64 // 0 = takes the remaining width
	align align
}

var a4Columns = [...]a4Column{
	{"N°", 24, alignCenter},
	{"CANT.", 40, alignCenter},
	{"UNIDAD", 86, alignLeft},
	{"DESCRIPCIÓN", 0, alignLeft},
	{"P. UNIT.", 70, alignRight},
	{"IMPORTE", 78, alignRight},
}

// renderA4 draws the note twice: the first pass counts pages so the second can
// print "Page 1 of N" in the footer.
func renderA4(n salesnote.Note) ([]byte, error) {
	_, pages, err := drawA4(n, 0)
	if err != nil {
		return nil, err
	}
	out, _, err := drawA4(n, pages)
	return out, err
}

type a4Doc struct {
	c     *canvas
	n     salesnote.Note
	pages int // total pages, 0 while still unknown
	y     float64
}

func drawA4(n salesnote.Note, knownPages int) ([]byte, int, error) {
	pdf := fpdf.New("P", "pt", "A4", "")
	pdf.SetTitle(n.Code, true)
	pdf.SetAuthor(n.Company.LegalName, true)
	pdf.SetCreator("ATM-Documents-API", true)
	pdf.SetCreationDate(n.IssuedAt)
	a := &a4Doc{c: newCanvas(pdf), n: n, pages: knownPages}
	pdf.SetHeaderFunc(a.watermarks)
	pdf.SetFooterFunc(a.footer)

	pdf.AddPage()
	a.y = a4Top
	if err := a.header(); err != nil {
		return nil, 0, err
	}
	a.parties()
	a.table()
	a.words()
	a.summary()
	a.companyNote()

	pages := pdf.PageNo()
	if pages > maxA4Pages {
		return nil, 0, errTooManyPages
	}
	out, err := a.c.output()
	return out, pages, err
}

// ensure starts a new page when h points do not fit below the cursor.
func (a *a4Doc) ensure(h float64) {
	if a.y+h > a4Limit {
		a.c.pdf.AddPage()
		a.y = a4Top
	}
}

func (a *a4Doc) ink(size float64, bold bool, col rgb) {
	a.c.font(size, bold)
	a.c.textColor(col)
}

// ---- Fixed elements, repeated on every page ----

// watermarks runs at the start of each page, so what it draws sits behind the content.
func (a *a4Doc) watermarks() {
	c := a.c
	size, bold := c.size, c.bold
	defer c.font(size, bold)

	if a.n.IsVoided {
		c.pdf.SetAlpha(0.1, "Normal")
		a.ink(70, true, ink)
		c.line(0, 320, a4Width, 70*lineHeightFactor, alignCenter, "ANULADA")
		c.pdf.SetAlpha(1, "Normal")
	}
	if a.n.IsTest {
		lines := []struct {
			text string
			size float64
		}{{"SIN VALOR", 92}, {"DOCUMENTO DE PRUEBA", 24}, {"NO VÁLIDO ANTE SUNAT", 24}}
		top := 300.0
		height := 0.0
		for _, l := range lines {
			height += l.size * lineHeightFactor
		}
		c.pdf.SetAlpha(0.22, "Normal")
		c.pdf.TransformBegin()
		c.pdf.TransformRotate(35, a4Width/2, top+height/2)
		y := top
		for _, l := range lines {
			a.ink(l.size, true, stamp)
			c.line(0, y, a4Width, l.size*lineHeightFactor, alignCenter, l.text)
			y += l.size * lineHeightFactor
		}
		c.pdf.TransformEnd()
		c.pdf.SetAlpha(1, "Normal")
	}
}

func (a *a4Doc) footer() {
	c := a.c
	size, bold := c.size, c.bold
	defer c.font(size, bold)

	const footerHeight = 25.4
	top := a4Height - 24 - footerHeight
	c.stroke(rule, 0.6)
	c.pdf.Line(a4MarginX, top, a4Width-a4MarginX, top)

	a.ink(7, false, muted)
	lh := 7 * lineHeightFactor
	y := top + 6
	c.line(a4MarginX, y, a4Content, lh, alignLeft, "Atendido por: "+a.n.SoldBy)
	page := fmt.Sprintf("Página %d", c.pdf.PageNo())
	if a.pages > 0 {
		page += fmt.Sprintf(" de %d", a.pages)
	}
	c.line(a4MarginX, y, a4Content, lh, alignRight, page)
	c.line(a4MarginX, y+lh+2, a4Content, lh, alignLeft, salesnote.Credits(a.n.IsTest))
}

// ---- Header: logo, issuer and the document box ----

type textBlock struct {
	lines  []string
	size   float64
	bold   bool
	col    rgb
	top    float64
	bottom float64
}

func (b textBlock) height() float64 {
	return b.top + float64(len(b.lines))*b.size*lineHeightFactor + b.bottom
}

func (a *a4Doc) header() error {
	c, n := a.c, a.n
	const logoW, logoH, logoGap, issuerGap = 96.0, 64.0, 14.0, 14.0

	hasLogo := len(n.Company.Logo) > 0
	issuerX := a4MarginX
	if hasLogo {
		issuerX += logoW + logoGap
	}
	issuerW := a4MarginX + a4Content - docBoxWidth - issuerGap - issuerX

	wrapped := func(text string, size float64, bold bool, w float64) []string {
		c.font(size, bold)
		return c.wrap(text, w, w)
	}
	blocks := []textBlock{{lines: wrapped(strings.ToUpper(n.Company.LegalName), 12, true, issuerW), size: 12, bold: true, col: ink, bottom: 3}}
	if !strings.EqualFold(n.Company.Name, n.Company.LegalName) {
		blocks = append(blocks, textBlock{lines: wrapped(n.Company.Name, a4Text, true, issuerW), size: a4Text, bold: true, col: ink})
	}
	for _, text := range []string{n.Company.Address, optional("Teléfono: ", n.Company.Phone), optional("Correo: ", n.Company.Email)} {
		if text != "" {
			blocks = append(blocks, textBlock{lines: wrapped(text, a4Text, false, issuerW), size: a4Text, col: muted, top: 1.5})
		}
	}
	issuerH := 0.0
	for _, b := range blocks {
		issuerH += b.height()
	}

	// Document box: RUC, document type on a band, and the number.
	const typeSize = 11.5
	typeLH := typeSize * lineHeightFactor
	rucLines := wrapped("RUC "+n.Company.TaxID, typeSize, true, docBoxWidth)
	typeLines := wrapped("NOTA DE VENTA", typeSize, true, docBoxWidth-16)
	numLines := wrapped(n.Code, typeSize, true, docBoxWidth)
	rucH := 10 + float64(len(rucLines))*typeLH + 7
	typeH := 7 + float64(len(typeLines))*typeLH + 7
	numH := 7 + float64(len(numLines))*typeLH + 10

	rowH := max(issuerH, rucH+typeH+numH)
	if hasLogo {
		rowH = max(rowH, logoH)
	}
	top := a.y
	boxX := a4MarginX + a4Content - docBoxWidth

	if hasLogo {
		if err := c.drawLogo("logo", n.Company.Logo, a4MarginX, top+(rowH-logoH)/2, logoW, logoH); err != nil {
			return err
		}
	}
	y := top + (rowH-issuerH)/2
	for _, b := range blocks {
		a.ink(b.size, b.bold, b.col)
		y += b.top
		y += c.lines(issuerX, y, issuerW, b.size*lineHeightFactor, alignLeft, b.lines)
		y += b.bottom
	}

	c.fill(band)
	c.pdf.Rect(boxX, top+rucH, docBoxWidth, typeH, "F")
	c.stroke(rule, a4Border)
	c.roundedBox(boxX, top, docBoxWidth, rowH, a4Radius, "D")
	a.ink(typeSize, true, ink)
	c.lines(boxX, top+10, docBoxWidth, typeLH, alignCenter, rucLines)
	c.lines(boxX+8, top+rucH+7, docBoxWidth-16, typeLH, alignCenter, typeLines)
	c.lines(boxX, top+rucH+typeH+7, docBoxWidth, typeLH, alignCenter, numLines)

	a.y = top + rowH + 12
	return nil
}

func optional(prefix, value string) string {
	if value == "" {
		return ""
	}
	return prefix + value
}

// ---- Customer and issue date ----

type field struct{ label, value string }

const (
	fieldGap     = 2.5
	boxPadV      = 8.0
	boxPadH      = 10.0
	partiesGap   = 8.0
	leftLabelW   = 72.0
	rightLabelW  = 84.0
	sectionAfter = 10.0
)

func (a *a4Doc) fieldsHeight(w, labelW float64, fields []field) float64 {
	c := a.c
	c.font(a4Text, false)
	lh := c.lineHeight()
	h := 0.0
	for _, f := range fields {
		h += float64(len(c.wrap(f.value, w-labelW, w-labelW)))*lh + fieldGap
	}
	return h
}

func (a *a4Doc) drawFields(x, y, w, labelW float64, fields []field) {
	c := a.c
	lh := a4Text * lineHeightFactor
	for _, f := range fields {
		a.ink(a4Text, true, ink)
		c.line(x, y, labelW, lh, alignLeft, f.label)
		a.ink(a4Text, false, ink)
		lines := c.wrap(f.value, w-labelW, w-labelW)
		y += c.lines(x+labelW, y, w-labelW, lh, alignLeft, lines) + fieldGap
	}
}

func (a *a4Doc) parties() {
	n := a.n
	docLabel, docNumber := salesnote.SplitDocument(n.Customer.Document)
	left := []field{{"CLIENTE:", n.Customer.Name}}
	if docNumber != "" {
		left = append(left, field{docLabel + ":", docNumber})
	}
	if n.Customer.Address != "" {
		left = append(left, field{"DIRECCIÓN:", n.Customer.Address})
	}
	right := []field{
		{"FECHA:", salesnote.IssueDate(n.IssuedAt)},
		{"HORA:", salesnote.IssueTime(n.IssuedAt)},
		{"MONEDA:", "SOLES"},
	}

	leftW := a4Content - docBoxWidth - partiesGap
	h := 2*boxPadV + max(
		a.fieldsHeight(leftW-2*boxPadH, leftLabelW, left),
		a.fieldsHeight(docBoxWidth-2*boxPadH, rightLabelW, right),
	)
	a.ensure(h)
	c := a.c
	c.stroke(rule, a4Border)
	c.roundedBox(a4MarginX, a.y, leftW, h, a4Radius, "D")
	c.roundedBox(a4MarginX+a4Content-docBoxWidth, a.y, docBoxWidth, h, a4Radius, "D")
	a.drawFields(a4MarginX+boxPadH, a.y+boxPadV, leftW-2*boxPadH, leftLabelW, left)
	a.drawFields(a4MarginX+a4Content-docBoxWidth+boxPadH, a.y+boxPadV, docBoxWidth-2*boxPadH, rightLabelW, right)
	a.y += h + sectionAfter
}

// ---- Detail table ----

func (a *a4Doc) columnWidths() []float64 {
	widths := make([]float64, len(a4Columns))
	fixed := 0.0
	for i, col := range a4Columns {
		widths[i] = col.width
		fixed += col.width
	}
	for i, col := range a4Columns {
		if col.width == 0 {
			widths[i] = a4Content - fixed
		}
	}
	return widths
}

func (a *a4Doc) table() {
	c := a.c
	widths := a.columnWidths()
	const headSize, headPadV, cellPadV, cellPadH = 7.5, 6.0, 5.0, 6.0
	headH := 2*headPadV + headSize*lineHeightFactor
	segTop := a.y

	drawHead := func() {
		c.fill(band)
		c.pdf.RoundedRectExt(a4MarginX, a.y, a4Content, headH, a4Radius, a4Radius, 0, 0, "F")
		a.ink(headSize, true, ink)
		x := a4MarginX
		for i, col := range a4Columns {
			c.line(x+cellPadH, a.y+headPadV, widths[i]-2*cellPadH, headSize*lineHeightFactor, col.align, col.label)
			x += widths[i]
		}
		a.y += headH
	}
	closeSegment := func() {
		c.stroke(rule, a4Border)
		c.roundedBox(a4MarginX, segTop, a4Content, a.y-segTop, a4Radius, "D")
	}

	c.font(a4Text, false)
	lh := c.lineHeight()
	rows := make([][][]string, len(a.n.Items))
	heights := make([]float64, len(a.n.Items))
	for i, item := range a.n.Items {
		cells := []string{
			fmt.Sprint(i + 1),
			salesnote.Quantity(item.Quantity),
			item.Presentation,
			item.Name,
			salesnote.Amount(item.UnitPrice),
			salesnote.Amount(item.LineTotal),
		}
		rows[i] = make([][]string, len(cells))
		maxLines := 1
		for j, cell := range cells {
			w := widths[j] - 2*cellPadH
			rows[i][j] = c.wrap(cell, w, w)
			maxLines = max(maxLines, len(rows[i][j]))
		}
		heights[i] = 2*cellPadV + float64(maxLines)*lh
	}

	a.ensure(headH + heights[0])
	segTop = a.y
	drawHead()
	for i := range a.n.Items {
		if a.y+heights[i] > a4Limit {
			closeSegment()
			c.pdf.AddPage()
			a.y = a4Top
			segTop = a.y
			drawHead()
		}
		c.stroke(faint, 0.5)
		c.pdf.Line(a4MarginX, a.y, a4MarginX+a4Content, a.y)
		a.ink(a4Text, false, ink)
		x := a4MarginX
		for j, lines := range rows[i] {
			c.lines(x+cellPadH, a.y+cellPadV, widths[j]-2*cellPadH, lh, a4Columns[j].align, lines)
			x += widths[j]
		}
		a.y += heights[i]
	}
	closeSegment()
}

// ---- Total in words ----

func (a *a4Doc) words() {
	c := a.c
	const marginTop, padV, padH = 8.0, 7.0, 10.0
	innerW := a4Content - 2*padH
	c.font(a4Text, true)
	labelW := c.width("SON: ")
	c.font(a4Text, false)
	lines := c.wrap(salesnote.AmountInWords(a.n.Total), innerW-labelW, innerW-labelW)
	h := 2*padV + float64(len(lines))*c.lineHeight()

	a.ensure(marginTop + h)
	a.y += marginTop
	c.stroke(rule, a4Border)
	c.roundedBox(a4MarginX, a.y, a4Content, h, a4Radius, "D")
	a.ink(a4Text, true, ink)
	c.line(a4MarginX+padH, a.y+padV, labelW, c.lineHeight(), alignLeft, "SON:")
	a.ink(a4Text, false, ink)
	c.lines(a4MarginX+padH+labelW, a.y+padV, innerW-labelW, c.lineHeight(), alignLeft, lines)
	a.y += h
}

// ---- Internal notice (left) and totals (right) ----

func (a *a4Doc) summary() {
	c := a.c
	const marginTop, legalGap = 10.0, 14.0
	const rowPad, grandPad, grandSize = 4.0, 8.0, 12.0
	rowH := 2*rowPad + a4Text*lineHeightFactor
	grandH := 2*grandPad + grandSize*lineHeightFactor
	totalsH := 3 + 2*rowH + 3 + grandH

	legalW := a4Content - totalsWidth - legalGap
	c.font(a4Text, true)
	notice := c.wrap("Documento interno. No válido como comprobante de pago.", legalW, legalW)
	c.font(a4Text, false)
	hint := c.wrap("Si necesitas boleta o factura, pídela al momento de la compra.", legalW, legalW)
	legalH := float64(len(notice))*a4Text*lineHeightFactor + 3 + float64(len(hint))*a4Text*lineHeightFactor

	h := max(totalsH, legalH)
	a.ensure(marginTop + h)
	a.y += marginTop

	a.ink(a4Text, true, ink)
	used := c.lines(a4MarginX, a.y, legalW, a4Text*lineHeightFactor, alignLeft, notice)
	a.ink(a4Text, false, muted)
	c.lines(a4MarginX, a.y+used+3, legalW, a4Text*lineHeightFactor, alignLeft, hint)

	boxX := a4MarginX + a4Content - totalsWidth
	y := a.y + 3
	rows := []struct {
		label string
		value string
	}{{"OP. GRAVADA", salesnote.Money(a.n.TaxableAmount)}, {"I.G.V. (18%)", salesnote.Money(a.n.TaxAmount)}}
	for _, row := range rows {
		a.ink(a4Text, false, muted)
		c.line(boxX+boxPadH, y+rowPad, totalsWidth-2*boxPadH, a4Text*lineHeightFactor, alignLeft, row.label)
		a.ink(a4Text, false, ink)
		c.line(boxX+boxPadH, y+rowPad, totalsWidth-2*boxPadH, a4Text*lineHeightFactor, alignRight, row.value)
		y += rowH
	}
	y += 3
	c.fill(band)
	c.pdf.RoundedRectExt(boxX, y, totalsWidth, grandH, 0, 0, a4Radius, a4Radius, "F")
	a.ink(grandSize, true, ink)
	c.line(boxX+boxPadH, y+grandPad, totalsWidth-2*boxPadH, grandSize*lineHeightFactor, alignLeft, "TOTAL")
	c.line(boxX+boxPadH, y+grandPad, totalsWidth-2*boxPadH, grandSize*lineHeightFactor, alignRight, salesnote.Money(a.n.Total))
	c.stroke(rule, a4Border)
	c.roundedBox(boxX, a.y, totalsWidth, totalsH, a4Radius, "D")

	a.y += h
}

// ---- Company message ----

func (a *a4Doc) companyNote() {
	if a.n.Company.Note == "" {
		return
	}
	c := a.c
	const marginTop, padV, padH, size, factor = 10.0, 8.0, 12.0, 9.0, 1.35
	lh := size * factor
	innerW := a4Content - 2*padH

	type noteLine struct {
		text string
		bold bool
	}
	var lines []noteLine
	for i, line := range salesnote.NoteLines(a.n.Company.Note) {
		c.font(size, i == 0)
		for _, part := range c.wrap(line, innerW, innerW) {
			lines = append(lines, noteLine{part, i == 0})
		}
	}
	h := 2*padV + float64(len(lines))*lh
	a.ensure(marginTop + h)
	a.y += marginTop
	c.stroke(rule, a4Border)
	c.roundedBox(a4MarginX, a.y, a4Content, h, a4Radius, "D")
	for i, l := range lines {
		a.ink(size, l.bold, ink)
		c.line(a4MarginX+padH, a.y+padV+float64(i)*lh, innerW, lh, alignCenter, l.text)
	}
	a.y += h
}
