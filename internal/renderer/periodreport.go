package renderer

import (
	"errors"
	"strconv"

	"github.com/AtmosferaSoltec/atm-documents-api/internal/periodreport"
	"github.com/go-pdf/fpdf"
)

// The Orbita period report: A4, one block per currency with its totals and the
// two rankings drawn as proportional bars.

const (
	prMarginX = 36.0
	prTop     = 40.0
	prLimit   = a4Height - 60 // lowest y content may reach; the footer lives below
	prContent = a4Width - 2*prMarginX
	prGap     = 10.0

	prNameWidth    = 185.0
	prBarWidth     = 140.0
	prAmountWidth  = 105.0
	prPercentWidth = prContent - prNameWidth - prBarWidth - prAmountWidth - 3*prGap
	prBarHeight    = 7.0
	prRowText      = 9.0

	// maxReportPages keeps a runaway payload from producing an enormous document.
	maxReportPages = 40
)

var (
	incomeColor  = rgb{0x2e, 0x7d, 0x32}
	expenseColor = rgb{0xc6, 0x28, 0x28}
	accentColor  = rgb{0x5b, 0x3a, 0x8c}
	trackColor   = rgb{0xec, 0xec, 0xec}
)

var errTooManyReportPages = errors.New("report exceeds the page limit")

// PeriodReport renders the Orbita period report with go-pdf/fpdf.
type PeriodReport struct{}

// NewPeriodReport returns the period report renderer.
func NewPeriodReport() *PeriodReport { return &PeriodReport{} }

type reportDoc struct {
	c     *canvas
	y     float64
	pages int
}

// Render draws the report.
func (*PeriodReport) Render(report periodreport.Report) ([]byte, error) {
	report = sanitizeReport(report)
	pdf := fpdf.New("P", "pt", "A4", "")
	pdf.SetTitle("Reporte de Orbita", true)
	pdf.SetCreator("ATM-Documents-API", true)
	pdf.SetCreationDate(report.From)
	pdf.SetModificationDate(report.From)
	d := &reportDoc{c: newCanvas(pdf)}
	pdf.SetFooterFunc(d.footer)

	d.newPage()
	d.header(report)
	for _, currency := range report.Currencies {
		d.currencyBlock(currency)
		if d.pages > maxReportPages {
			return nil, errTooManyReportPages
		}
	}
	if err := pdf.Error(); err != nil {
		return nil, err
	}
	return d.c.output()
}

func (d *reportDoc) newPage() {
	d.c.pdf.AddPage()
	d.pages++
	d.y = prTop
}

// ensure moves to a new page when h points do not fit below y.
func (d *reportDoc) ensure(h float64) {
	if d.y+h > prLimit && d.pages <= maxReportPages {
		d.newPage()
	}
}

func (d *reportDoc) footer() {
	c := d.c
	c.stroke(rule, 0.5)
	c.pdf.Line(prMarginX, a4Height-44, a4Width-prMarginX, a4Height-44)
	c.font(8, false)
	c.textColor(muted)
	c.line(prMarginX, a4Height-40, prContent/2, 14, alignLeft, "Generado por Orbita")
	c.line(prMarginX+prContent/2, a4Height-40, prContent/2, 14, alignRight, "Página "+strconv.Itoa(c.pdf.PageNo()))
}

func (d *reportDoc) header(r periodreport.Report) {
	c := d.c
	c.font(20, true)
	c.textColor(accentColor)
	c.line(prMarginX, d.y, prContent, 26, alignLeft, "Reporte de Orbita")
	d.y += 28
	c.font(11, false)
	c.textColor(ink)
	c.line(prMarginX, d.y, prContent, 16, alignLeft, "Periodo: "+periodreport.Period(r.From, r.To))
	d.y += 22
	c.stroke(rule, 0.8)
	c.pdf.Line(prMarginX, d.y, a4Width-prMarginX, d.y)
	d.y += 16
}

func (d *reportDoc) currencyBlock(cur periodreport.Currency) {
	c := d.c
	d.ensure(120)
	c.font(13, true)
	c.textColor(ink)
	c.line(prMarginX, d.y, prContent, 18, alignLeft, cur.Code)
	d.y += 24

	boxW := (prContent - 2*prGap) / 3
	boxH := 46.0
	balanceColor := ink
	if cur.Balance != "" && cur.Balance[0] == '-' {
		balanceColor = expenseColor
	}
	for i, box := range []struct {
		label, value string
		color        rgb
	}{
		{"Ingresos", cur.Income, incomeColor},
		{"Gastos", cur.Expense, expenseColor},
		{"Balance", cur.Balance, balanceColor},
	} {
		x := prMarginX + float64(i)*(boxW+prGap)
		c.stroke(rule, 0.8)
		c.roundedBox(x, d.y, boxW, boxH, a4Radius, "D")
		c.font(8.5, false)
		c.textColor(muted)
		c.line(x+10, d.y+6, boxW-20, 12, alignLeft, box.label)
		c.font(13, true)
		c.textColor(box.color)
		c.line(x+10, d.y+22, boxW-20, 18, alignLeft, periodreport.Money(cur.Code, box.value))
	}
	d.y += boxH + 16

	d.ranking("Gastos por categoría", cur, cur.Expenses, expenseColor)
	d.ranking("Fuentes de ingreso", cur, cur.Incomes, incomeColor)
	d.y += 8
}

func (d *reportDoc) ranking(title string, cur periodreport.Currency, rows []periodreport.Line, color rgb) {
	c := d.c
	d.ensure(18 + 22)
	c.font(10.5, true)
	c.textColor(ink)
	c.line(prMarginX, d.y, prContent, 16, alignLeft, title)
	d.y += 20
	if len(rows) == 0 {
		c.font(prRowText, false)
		c.textColor(muted)
		c.line(prMarginX, d.y, prContent, 14, alignLeft, "Sin movimientos en el periodo")
		d.y += 22
		return
	}
	for _, row := range rows {
		c.font(prRowText, false)
		lines := c.wrap(row.Name, prNameWidth, prNameWidth)
		if len(lines) > 2 {
			lines = lines[:2]
		}
		lh := c.lineHeight()
		h := float64(len(lines))*lh + 6
		d.ensure(h)

		c.textColor(ink)
		c.lines(prMarginX, d.y, prNameWidth, lh, alignLeft, lines)

		barX := prMarginX + prNameWidth + prGap
		barY := d.y + (h-6-prBarHeight)/2
		c.fill(trackColor)
		c.pdf.Rect(barX, barY, prBarWidth, prBarHeight, "F")
		if share, err := strconv.ParseFloat(row.Percent, 64); err == nil && share > 0 {
			c.fill(color)
			c.pdf.Rect(barX, barY, prBarWidth*share/100, prBarHeight, "F")
		}

		amountX := barX + prBarWidth + prGap
		c.textColor(ink)
		c.line(amountX, d.y, prAmountWidth, lh, alignRight, periodreport.Money(cur.Code, row.Total))
		c.textColor(muted)
		c.line(amountX+prAmountWidth+prGap, d.y, prPercentWidth, lh, alignRight, periodreport.Percent(row.Percent))
		d.y += h
	}
	d.y += 10
}

// sanitizeReport replaces control characters in the only free text, the names.
func sanitizeReport(r periodreport.Report) periodreport.Report {
	currencies := make([]periodreport.Currency, len(r.Currencies))
	for i, cur := range r.Currencies {
		cur.Expenses = cleanLines(cur.Expenses)
		cur.Incomes = cleanLines(cur.Incomes)
		currencies[i] = cur
	}
	r.Currencies = currencies
	return r
}

func cleanLines(in []periodreport.Line) []periodreport.Line {
	out := make([]periodreport.Line, len(in))
	for i, l := range in {
		l.Name = clean(l.Name)
		out[i] = l
	}
	return out
}
