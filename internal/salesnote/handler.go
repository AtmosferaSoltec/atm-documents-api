package salesnote

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/AtmosferaSoltec/atm-documents-api/internal/auth"
	"github.com/AtmosferaSoltec/atm-documents-api/internal/platform/httpserver"
	"github.com/gin-gonic/gin"
)

// MaxBodyBytes bounds the request: a 2 MiB logo grows by a third in base64.
const MaxBodyBytes = 4 << 20

type companyRequest struct {
	Name       string `json:"name"`
	LegalName  string `json:"legal_name"`
	TaxID      string `json:"tax_id"`
	Address    string `json:"address"`
	Phone      string `json:"phone"`
	Email      string `json:"email"`
	Note       string `json:"note"`
	LogoBase64 string `json:"logo_base64"`
}

type customerRequest struct {
	Name     string `json:"name"`
	Document string `json:"document"`
	Address  string `json:"address"`
}

type itemRequest struct {
	Name         string  `json:"name"`
	Presentation string  `json:"presentation"`
	Quantity     float64 `json:"quantity"`
	UnitPrice    float64 `json:"unit_price"`
	LineTotal    float64 `json:"line_total"`
}

type request struct {
	Format        string          `json:"format"`
	Code          string          `json:"code"`
	IssuedAt      time.Time       `json:"issued_at"`
	Company       companyRequest  `json:"company"`
	Customer      customerRequest `json:"customer"`
	Items         []itemRequest   `json:"items"`
	TaxableAmount float64         `json:"taxable_amount"`
	TaxAmount     float64         `json:"tax_amount"`
	Total         float64         `json:"total"`
	SoldBy        string          `json:"sold_by"`
	IsVoided      bool            `json:"is_voided"`
	IsTest        bool            `json:"is_test"`
}

// Handler serves the sales note endpoint.
type Handler struct {
	service *Service
	timeout time.Duration
	logger  *slog.Logger
}

// NewHandler builds the HTTP handler. timeout bounds one request end to end.
func NewHandler(service *Service, timeout time.Duration, logger *slog.Logger) *Handler {
	return &Handler{service: service, timeout: timeout, logger: logger}
}

// RegisterRoutes mounts the endpoint on an already authenticated group.
func RegisterRoutes(group *gin.RouterGroup, h *Handler) {
	group.POST("/documents/sales-note", h.create)
}

func (h *Handler) create(c *gin.Context) {
	body := http.MaxBytesReader(c.Writer, c.Request.Body, MaxBodyBytes)
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	var req request
	if err := decoder.Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpserver.Error(c, http.StatusRequestEntityTooLarge, CodePayloadTooLarge, "Request body is too large.")
			return
		}
		httpserver.Error(c, http.StatusBadRequest, CodeInvalidRequest, "Request body must be valid JSON with known fields.")
		return
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		httpserver.Error(c, http.StatusBadRequest, CodeInvalidRequest, "Request body must contain a single JSON object.")
		return
	}
	note, err := req.toNote()
	if err != nil {
		httpserver.Error(c, http.StatusUnprocessableEntity, CodeInvalidPayload, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()
	started := time.Now()
	pdf, err := h.service.Render(ctx, note)
	var validation *ValidationError
	switch {
	case errors.As(err, &validation):
		httpserver.Error(c, http.StatusUnprocessableEntity, CodeInvalidPayload, validation.Error())
		return
	case errors.Is(err, ErrBusy):
		httpserver.Error(c, http.StatusServiceUnavailable, CodeBusy, "The service is busy. Try again shortly.")
		return
	case err != nil:
		// The payload holds personal and financial data, so only metadata is logged.
		h.logger.Error("sales note render failed", "request_id", c.GetHeader("X-Request-ID"), "format", note.Format, "error", err.Error())
		httpserver.Error(c, http.StatusInternalServerError, CodeRenderFailed, "Unable to render the document.")
		return
	}
	h.logger.Info("sales note rendered",
		"request_id", c.GetHeader("X-Request-ID"),
		"system", auth.FromContext(c).System,
		"format", note.Format,
		"items", len(note.Items),
		"bytes", len(pdf),
		"duration_ms", time.Since(started).Milliseconds(),
	)
	c.Header("Content-Disposition", `inline; filename="`+fileName(note.Code)+`.pdf"`)
	c.Data(http.StatusOK, "application/pdf", pdf)
}

func (r request) toNote() (Note, error) {
	var logo []byte
	if r.Company.LogoBase64 != "" {
		// Reject before decoding: the decoded size is at most 3/4 of the text.
		if len(r.Company.LogoBase64) > base64.StdEncoding.EncodedLen(MaxLogoBytes) {
			return Note{}, invalid("company.logo_base64", "is too large")
		}
		decoded, err := base64.StdEncoding.DecodeString(r.Company.LogoBase64)
		if err != nil {
			return Note{}, invalid("company.logo_base64", "must be standard base64")
		}
		logo = decoded
	}
	items := make([]Item, len(r.Items))
	for i, item := range r.Items {
		items[i] = Item(item)
	}
	return Note{
		Format:   Format(r.Format),
		Code:     r.Code,
		IssuedAt: r.IssuedAt,
		Company: Company{
			Name: r.Company.Name, LegalName: r.Company.LegalName, TaxID: r.Company.TaxID,
			Address: r.Company.Address, Phone: r.Company.Phone, Email: r.Company.Email,
			Note: r.Company.Note, Logo: logo,
		},
		Customer:      Customer(r.Customer),
		Items:         items,
		TaxableAmount: r.TaxableAmount,
		TaxAmount:     r.TaxAmount,
		Total:         r.Total,
		SoldBy:        r.SoldBy,
		IsVoided:      r.IsVoided,
		IsTest:        r.IsTest,
	}, nil
}

var unsafeFileChars = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// fileName makes the note code safe for a Content-Disposition header.
func fileName(code string) string { return unsafeFileChars.ReplaceAllString(code, "") }
