package renderer

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/AtmosferaSoltec/atm-documents-api/internal/salesnote"
)

var pageCount = regexp.MustCompile(`/Type /Page[^s]`)

func pages(pdf []byte) int { return len(pageCount.FindAll(pdf, -1)) }

// testLogo is a transparent-background PNG with a colored block, enough to
// exercise image embedding and the alpha channel.
func testLogo(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 300, 120))
	for x := 0; x < 300; x++ {
		for y := 0; y < 120; y++ {
			if x > 20 && x < 280 && y > 20 && y < 100 {
				img.Set(x, y, color.NRGBA{R: 120, G: 30, B: 60, A: 255})
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sampleNote(t *testing.T, format salesnote.Format, items int) salesnote.Note {
	t.Helper()
	list := make([]salesnote.Item, items)
	total := 0.0
	for i := range list {
		price := 10 + float64(i%7)*8.5
		qty := float64(1 + i%3)
		list[i] = salesnote.Item{
			Name:         fmt.Sprintf("Vino Tinto Reserva Especial Cosecha %d", 2015+i%9),
			Presentation: "Botella 750 ml",
			Quantity:     qty,
			UnitPrice:    price,
			LineTotal:    price * qty,
		}
		total += price * qty
	}
	taxable := total / 1.18
	return salesnote.Note{
		Format: format,
		Company: salesnote.Company{
			Name:      "La Vendimia",
			LegalName: "Bodegas La Vendimia S.A.C.",
			TaxID:     "20601234567",
			Address:   "Av. Los Viñedos 123, Ica, Ica, Ica",
			Phone:     "956 123 456",
			Email:     "ventas@lavendimia.pe",
			Note:      "ENVÍOS A TODO EL PERÚ\nPedidos al 956 123 456\nGracias por preferirnos",
			Logo:      testLogo(t),
		},
		Code:          "NV-00000021",
		IssuedAt:      time.Date(2026, 10, 13, 15, 45, 0, 0, time.UTC),
		Customer:      salesnote.Customer{Name: "María Peña Núñez", Document: "DNI 45872310", Address: "Jr. Los Álamos 456, Lima"},
		Items:         list,
		TaxableAmount: taxable,
		TaxAmount:     total - taxable,
		Total:         total,
		SoldBy:        "Carlos Ramírez",
	}
}

// TestPreview writes sample PDFs to $PREVIEW_DIR so a developer can look at
// them. Layout is defined in code, so every template change is checked by eye.
func TestPreview(t *testing.T) {
	dir := os.Getenv("PREVIEW_DIR")
	if dir == "" {
		t.Skip("set PREVIEW_DIR to write sample PDFs")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cases := map[string]salesnote.Note{}
	for name, format := range map[string]salesnote.Format{"a4": salesnote.FormatA4, "ticket": salesnote.FormatTicket} {
		cases[name+"-short"] = sampleNote(t, format, 3)
		long := sampleNote(t, format, 40)
		cases[name+"-long"] = long
		voided := sampleNote(t, format, 3)
		voided.IsVoided = true
		cases[name+"-voided"] = voided
		test := sampleNote(t, format, 3)
		test.IsTest = true
		cases[name+"-test"] = test
		bare := sampleNote(t, format, 2)
		bare.Company.Logo, bare.Company.Note, bare.Company.Phone, bare.Company.Email = nil, "", "", ""
		bare.Customer = salesnote.Customer{Name: "CLIENTES VARIOS"}
		cases[name+"-bare"] = bare
	}
	for name, note := range cases {
		out, err := New().Render(note)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".pdf"), out, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestA4SinglePage(t *testing.T) {
	out, err := New().Render(sampleNote(t, salesnote.FormatA4, 3))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Fatal("output is not a PDF")
	}
	if got := pages(out); got != 1 {
		t.Fatalf("pages = %d, want 1", got)
	}
}

func TestA4PaginatesLongNotes(t *testing.T) {
	out, err := New().Render(sampleNote(t, salesnote.FormatA4, 80))
	if err != nil {
		t.Fatal(err)
	}
	if got := pages(out); got < 2 || got > maxA4Pages {
		t.Fatalf("pages = %d, want 2..%d", got, maxA4Pages)
	}
}

func TestTicketIsAlwaysOnePage(t *testing.T) {
	for _, items := range []int{1, 5, 40, salesnote.MaxItemsTicket} {
		t.Run(fmt.Sprintf("%d items", items), func(t *testing.T) {
			out, err := New().Render(sampleNote(t, salesnote.FormatTicket, items))
			if err != nil {
				t.Fatal(err)
			}
			if got := pages(out); got != 1 {
				t.Fatalf("pages = %d, want 1", got)
			}
		})
	}
}

func TestOutputIsDeterministic(t *testing.T) {
	for _, format := range []salesnote.Format{salesnote.FormatA4, salesnote.FormatTicket} {
		note := sampleNote(t, format, 5)
		first, err := New().Render(note)
		if err != nil {
			t.Fatal(err)
		}
		second, err := New().Render(note)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(first, second) {
			t.Fatalf("%s: same input produced different bytes", format)
		}
	}
}

func TestRendersWithoutOptionalFields(t *testing.T) {
	for _, format := range []salesnote.Format{salesnote.FormatA4, salesnote.FormatTicket} {
		note := sampleNote(t, format, 1)
		note.Company.Logo, note.Company.Note, note.Company.Phone, note.Company.Email = nil, "", "", ""
		note.Customer = salesnote.Customer{}
		if _, err := New().Render(note); err != nil {
			t.Fatalf("%s: %v", format, err)
		}
	}
}

func TestSurvivesHostileText(t *testing.T) {
	note := sampleNote(t, salesnote.FormatA4, 1)
	note.Items[0].Name = strings.Repeat("W", 400) + "\x00\x07 日本語"
	note.Customer.Address = strings.Repeat("palabra ", 60)
	for _, format := range []salesnote.Format{salesnote.FormatA4, salesnote.FormatTicket} {
		note.Format = format
		if _, err := New().Render(note); err != nil {
			t.Fatalf("%s: %v", format, err)
		}
	}
}

func TestRejectsUndecodableLogo(t *testing.T) {
	note := sampleNote(t, salesnote.FormatA4, 1)
	note.Company.Logo = []byte("not an image")
	if _, err := New().Render(note); err == nil {
		t.Fatal("expected an error for a corrupt logo")
	}
}
