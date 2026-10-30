package salesnote

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"time"
)

// SampleNote is the fictional note the console previews, so a template can be
// seen without sending real data. Nothing in it belongs to a real company.
func SampleNote(format Format) Note {
	items := []Item{
		{Name: "Vino tinto reserva", Presentation: "Botella 750 ml", Quantity: 2, UnitPrice: 68, LineTotal: 136},
		{Name: "Pisco quebranta", Presentation: "Botella 500 ml", Quantity: 1, UnitPrice: 45.5, LineTotal: 45.5},
		{Name: "Queso andino madurado", Presentation: "Unidad", Quantity: 3, UnitPrice: 18.9, LineTotal: 56.7},
	}
	total := 0.0
	for _, item := range items {
		total += item.LineTotal
	}
	taxable := total / 1.18
	return Note{
		Format: format,
		Company: Company{
			Name:      "Tienda de Ejemplo",
			LegalName: "Comercial de Ejemplo S.A.C.",
			TaxID:     "20000000001",
			Address:   "Av. Ejemplo 123, Lima, Lima, Lima",
			Phone:     "999 999 999",
			Email:     "ventas@ejemplo.pe",
			Note:      "GRACIAS POR SU COMPRA\nEnvíos a todo el Perú",
			Logo:      sampleLogo(),
		},
		Code:          "NV-00000001",
		IssuedAt:      time.Date(2026, 10, 30, 15, 30, 0, 0, time.UTC),
		Customer:      Customer{Name: "Cliente de Ejemplo", Document: "DNI 00000000", Address: "Jr. Ejemplo 456, Lima"},
		Items:         items,
		TaxableAmount: taxable,
		TaxAmount:     total - taxable,
		Total:         total,
		SoldBy:        "Vendedor de Ejemplo",
	}
}

// sampleLogo is a flat gray block that shows where a real logo goes.
func sampleLogo() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 240, 100))
	fill := color.RGBA{R: 0xd0, G: 0xd0, B: 0xd0, A: 0xff}
	for x := 0; x < 240; x++ {
		for y := 0; y < 100; y++ {
			img.Set(x, y, fill)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	return buf.Bytes()
}
