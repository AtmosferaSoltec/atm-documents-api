package salesnote

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"testing"
	"time"
)

func TestIntegerInWords(t *testing.T) {
	// Same expectations La Vendimia's TypeScript version had.
	cases := map[int]string{
		0:       "CERO",
		1:       "UNO",
		15:      "QUINCE",
		21:      "VEINTIUNO",
		30:      "TREINTA",
		45:      "CUARENTA Y CINCO",
		100:     "CIEN",
		101:     "CIENTO UNO",
		120:     "CIENTO VEINTE",
		500:     "QUINIENTOS",
		999:     "NOVECIENTOS NOVENTA Y NUEVE",
		1000:    "MIL",
		1001:    "MIL UNO",
		2500:    "DOS MIL QUINIENTOS",
		21000:   "VEINTIUN MIL",
		31000:   "TREINTA Y UN MIL",
		100000:  "CIEN MIL",
		1000000: "UN MILLÓN",
		2350000: "DOS MILLONES TRESCIENTOS CINCUENTA MIL",
	}
	for n, want := range cases {
		if got := IntegerInWords(n); got != want {
			t.Errorf("IntegerInWords(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestAmountInWords(t *testing.T) {
	cases := map[float64]string{
		120:         "CIENTO VEINTE CON 00/100 SOLES",
		1234.5:      "MIL DOSCIENTOS TREINTA Y CUATRO CON 50/100 SOLES",
		0.99:        "CERO CON 99/100 SOLES",
		0.1 + 0.2:   "CERO CON 30/100 SOLES", // 0.30000000000000004 must not drift
		99999999.99: "NOVENTA Y NUEVE MILLONES NOVECIENTOS NOVENTA Y NUEVE MIL NOVECIENTOS NOVENTA Y NUEVE CON 99/100 SOLES",
	}
	for amount, want := range cases {
		if got := AmountInWords(amount); got != want {
			t.Errorf("AmountInWords(%v) = %q, want %q", amount, got, want)
		}
	}
}

func TestAmount(t *testing.T) {
	cases := map[float64]string{
		0:           "0.00",
		5:           "5.00",
		1234.5:      "1,234.50",
		1234567.891: "1,234,567.89",
		0.005:       "0.01",
		-12.3:       "-12.30",
	}
	for value, want := range cases {
		if got := Amount(value); got != want {
			t.Errorf("Amount(%v) = %q, want %q", value, got, want)
		}
	}
	if got := Money(128); got != "S/ 128.00" {
		t.Errorf("Money(128) = %q", got)
	}
}

func TestDatesAreInLimaTime(t *testing.T) {
	// 03:30 UTC on the 14th is 22:30 on the 13th in Lima.
	at := time.Date(2026, 10, 14, 3, 30, 0, 0, time.UTC)
	if got := IssueDate(at); got != "13/10/2026" {
		t.Errorf("IssueDate = %q", got)
	}
	if got := IssueTime(at); got != "10:30 p. m." {
		t.Errorf("IssueTime = %q", got)
	}
	if got := IssueTime(time.Date(2026, 1, 1, 5, 5, 0, 0, time.UTC)); got != "12:05 a. m." {
		t.Errorf("midnight = %q", got)
	}
}

func TestSplitDocument(t *testing.T) {
	for in, want := range map[string][2]string{
		"DNI 45872310": {"DNI", "45872310"},
		"ruc 2060":     {"RUC", "2060"},
		"45872310":     {"DOC.", "45872310"},
		"":             {"DOC.", ""},
	} {
		label, number := SplitDocument(in)
		if label != want[0] || number != want[1] {
			t.Errorf("SplitDocument(%q) = (%q, %q), want %v", in, label, number, want)
		}
	}
}

func validNote() Note {
	return Note{
		Format:   FormatA4,
		Code:     "NV-00000021",
		IssuedAt: time.Now(),
		Company:  Company{Name: "La Vendimia", LegalName: "Bodegas La Vendimia S.A.C.", TaxID: "20601234567"},
		Items:    []Item{{Name: "Vino", Presentation: "Botella", Quantity: 1, UnitPrice: 10, LineTotal: 10}},
		Total:    10,
	}
}

func TestValidate(t *testing.T) {
	big := image.NewRGBA(image.Rect(0, 0, 4000, 4000)) // 16 MP
	var bigPNG bytes.Buffer
	if err := png.Encode(&bigPNG, big); err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		mutate func(*Note)
		field  string
	}{
		"bad format":     {func(n *Note) { n.Format = "a3" }, "format"},
		"empty code":     {func(n *Note) { n.Code = "" }, "code"},
		"code injection": {func(n *Note) { n.Code = "NV-1\r\nX: y" }, "code"},
		"no issue time":  {func(n *Note) { n.IssuedAt = time.Time{} }, "issued_at"},
		"no legal name":  {func(n *Note) { n.Company.LegalName = "" }, "company.legal_name"},
		"no items":       {func(n *Note) { n.Items = nil }, "items"},
		"zero quantity":  {func(n *Note) { n.Items[0].Quantity = 0 }, "items[0].quantity"},
		"negative price": {func(n *Note) { n.Items[0].UnitPrice = -1 }, "items[0].unit_price"},
		"huge total":     {func(n *Note) { n.Total = 1e12 }, "total"},
		"not an image":   {func(n *Note) { n.Company.Logo = []byte("GIF89a not really") }, "company.logo_base64"},
		"oversized logo": {func(n *Note) { n.Company.Logo = bigPNG.Bytes() }, "company.logo_base64"},
		"too many items": {func(n *Note) {
			n.Format = FormatTicket
			n.Items = make([]Item, MaxItemsTicket+1)
			for i := range n.Items {
				n.Items[i] = Item{Name: "x", Quantity: 1}
			}
		}, "items"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			note := validNote()
			tc.mutate(&note)
			var verr *ValidationError
			if err := note.Validate(); !errors.As(err, &verr) || verr.Field != tc.field {
				t.Fatalf("Validate() = %v, want error on %s", err, tc.field)
			}
		})
	}
	if err := validNote().Validate(); err != nil {
		t.Fatalf("a valid note was rejected: %v", err)
	}
}

type fakeRenderer struct {
	calls   int
	release chan struct{}
}

func (f *fakeRenderer) Render(Note) ([]byte, error) {
	f.calls++
	if f.release != nil {
		<-f.release
	}
	return []byte("%PDF-fake"), nil
}

func TestServiceValidatesBeforeRendering(t *testing.T) {
	fake := &fakeRenderer{}
	svc := NewService(fake, 1)
	bad := validNote()
	bad.Code = ""
	if _, err := svc.Render(context.Background(), bad); err == nil {
		t.Fatal("expected a validation error")
	}
	if fake.calls != 0 {
		t.Fatal("renderer was called for an invalid note")
	}
	if _, err := svc.Render(context.Background(), validNote()); err != nil {
		t.Fatal(err)
	}
}

func TestServiceLimitsConcurrency(t *testing.T) {
	fake := &fakeRenderer{release: make(chan struct{})}
	svc := NewService(fake, 1)
	done := make(chan error, 1)
	go func() {
		_, err := svc.Render(context.Background(), validNote())
		done <- err
	}()
	// Wait until the first render holds the only slot.
	for len(svc.slots) == 0 {
		time.Sleep(time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := svc.Render(ctx, validNote()); !errors.Is(err, ErrBusy) {
		t.Fatalf("second render error = %v, want ErrBusy", err)
	}
	close(fake.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
