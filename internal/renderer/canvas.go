package renderer

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"strings"
	"unicode/utf8"

	"github.com/go-pdf/fpdf"
)

// rgb is a color in 0-255 components.
type rgb struct{ r, g, b int }

var (
	ink   = rgb{0x11, 0x11, 0x11}
	muted = rgb{0x55, 0x55, 0x55}
	rule  = rgb{0x9a, 0x9a, 0x9a}
	band  = rgb{0xe9, 0xe9, 0xe9}
	faint = rgb{0xd4, 0xd4, 0xd4}
	black = rgb{0, 0, 0}
	stamp = rgb{0xC6, 0x28, 0x28}
)

type align int

const (
	alignLeft align = iota
	alignCenter
	alignRight
)

// lineHeightFactor is the default distance between lines, as a multiple of the font size.
const lineHeightFactor = 1.2

// canvas wraps fpdf with the few drawing primitives the templates need.
//
// All units are PDF points. Text uses the built-in Helvetica (Windows-1252),
// which covers Spanish accents, "ñ", "¡", "¿" and the middle dot, so no font
// file has to ship with the service.
type canvas struct {
	pdf  *fpdf.Fpdf
	tr   func(string) string
	size float64
	bold bool
}

func newCanvas(pdf *fpdf.Fpdf) *canvas {
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetCatalogSort(true)
	return &canvas{pdf: pdf, tr: pdf.UnicodeTranslatorFromDescriptor("")}
}

func (c *canvas) font(size float64, bold bool) {
	c.size, c.bold = size, bold
	style := ""
	if bold {
		style = "B"
	}
	c.pdf.SetFont("Helvetica", style, size)
}

func (c *canvas) textColor(col rgb) { c.pdf.SetTextColor(col.r, col.g, col.b) }
func (c *canvas) fill(col rgb)      { c.pdf.SetFillColor(col.r, col.g, col.b) }
func (c *canvas) stroke(col rgb, width float64) {
	c.pdf.SetDrawColor(col.r, col.g, col.b)
	c.pdf.SetLineWidth(width)
}

// width measures text in the current font.
func (c *canvas) width(s string) float64 { return c.pdf.GetStringWidth(c.tr(s)) }

// lineHeight is the default line box for the current font size.
func (c *canvas) lineHeight() float64 { return c.size * lineHeightFactor }

// wrap breaks text into lines. The first line may be narrower (a label sits
// before it). A word longer than a whole line is split by characters.
func (c *canvas) wrap(text string, first, width float64) []string {
	var lines []string
	current := ""
	limit := func() float64 {
		if len(lines) == 0 {
			return first
		}
		return width
	}
	for _, word := range strings.Fields(text) {
		candidate := word
		if current != "" {
			candidate = current + " " + word
		}
		if c.width(candidate) <= limit() {
			current = candidate
			continue
		}
		if current != "" {
			lines = append(lines, current)
			current = ""
		}
		for c.width(word) > limit() {
			cut := 0
			for i := range word {
				if i > 0 && c.width(word[:i]) > limit() {
					break
				}
				cut = i
			}
			if cut == 0 {
				_, cut = utf8.DecodeRuneInString(word)
			}
			lines = append(lines, word[:cut])
			word = word[cut:]
		}
		current = word
	}
	if current != "" || len(lines) == 0 {
		lines = append(lines, current)
	}
	return lines
}

// line draws one line of text inside a box of height lh whose top-left is (x, y).
func (c *canvas) line(x, y, w, lh float64, a align, s string) {
	if s == "" {
		return
	}
	offset := 0.0
	switch a {
	case alignCenter:
		offset = (w - c.width(s)) / 2
	case alignRight:
		offset = w - c.width(s)
	}
	// 0.36 em centers the glyph body (cap height of Helvetica is ~0.72 em) in the line box.
	c.pdf.Text(x+offset, y+lh/2+c.size*0.36, c.tr(s))
}

// lines draws wrapped lines top to bottom and returns the height used.
func (c *canvas) lines(x, y, w, lh float64, a align, lines []string) float64 {
	for i, s := range lines {
		c.line(x, y+float64(i)*lh, w, lh, a, s)
	}
	return float64(len(lines)) * lh
}

// labeled draws "LABEL value" where the label is bold; the value wraps under
// the full width. It returns the height used.
func (c *canvas) labeled(x, y, w, lh float64, label, value string) float64 {
	c.font(c.size, true)
	labelWidth := c.width(label)
	c.font(c.size, false)
	parts := c.wrap(value, w-labelWidth, w)
	c.font(c.size, true)
	c.line(x, y, w, lh, alignLeft, label)
	c.font(c.size, false)
	c.line(x+labelWidth, y, w-labelWidth, lh, alignLeft, parts[0])
	for i, s := range parts[1:] {
		c.line(x, y+float64(i+1)*lh, w, lh, alignLeft, s)
	}
	return float64(len(parts)) * lh
}

// labeledHeight is the height labeled would use, without drawing.
func (c *canvas) labeledHeight(w, lh float64, label, value string) float64 {
	c.font(c.size, true)
	labelWidth := c.width(label)
	c.font(c.size, false)
	return float64(len(c.wrap(value, w-labelWidth, w))) * lh
}

// roundedBox strokes (and optionally fills) a rounded rectangle.
func (c *canvas) roundedBox(x, y, w, h, r float64, style string) {
	c.pdf.RoundedRect(x, y, w, h, r, "1234", style)
}

// logoSize fits a w x h image inside a box keeping its ratio ("object-fit: contain").
func logoSize(imgW, imgH int, boxW, boxH float64) (w, h float64) {
	scale := boxW / float64(imgW)
	if s := boxH / float64(imgH); s < scale {
		scale = s
	}
	return float64(imgW) * scale, float64(imgH) * scale
}

// drawLogo registers and draws a logo centered in the box. The image is
// decoded and re-encoded as a plain PNG, which strips interlacing and other
// variants fpdf cannot embed.
func (c *canvas) drawLogo(name string, data []byte, x, y, boxW, boxH float64) error {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode logo: %w", err)
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		return fmt.Errorf("encode logo: %w", err)
	}
	c.pdf.RegisterImageOptionsReader(name, fpdf.ImageOptions{ImageType: "PNG", ReadDpi: false}, &encoded)
	b := img.Bounds()
	w, h := logoSize(b.Dx(), b.Dy(), boxW, boxH)
	c.pdf.ImageOptions(name, x+(boxW-w)/2, y+(boxH-h)/2, w, h, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
	return c.pdf.Error()
}

// output serializes the document.
func (c *canvas) output() ([]byte, error) {
	var buf bytes.Buffer
	if err := c.pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("write pdf: %w", err)
	}
	return buf.Bytes(), nil
}
