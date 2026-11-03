package periodreport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/AtmosferaSoltec/atm-documents-api/internal/activity"
	"github.com/AtmosferaSoltec/atm-documents-api/internal/auth"
	"github.com/AtmosferaSoltec/atm-documents-api/internal/platform/httpserver"
	"github.com/gin-gonic/gin"
)

type lineRequest struct {
	Name    string `json:"name"`
	Total   string `json:"total"`
	Percent string `json:"percent"`
}

type currencyRequest struct {
	Currency     string        `json:"currency"`
	IncomeTotal  string        `json:"income_total"`
	ExpenseTotal string        `json:"expense_total"`
	Balance      string        `json:"balance"`
	Expenses     []lineRequest `json:"expenses"`
	Incomes      []lineRequest `json:"incomes"`
}

type request struct {
	From       string            `json:"from"`
	To         string            `json:"to"`
	Currencies []currencyRequest `json:"currencies"`
}

// ActivityRecorder receives one event per render request.
type ActivityRecorder interface {
	Record(activity.Event)
}

// Handler serves the period report endpoint.
type Handler struct {
	service  *Service
	recorder ActivityRecorder
	timeout  time.Duration
	logger   *slog.Logger
}

// NewHandler builds the HTTP handler. timeout bounds one request end to end.
func NewHandler(service *Service, recorder ActivityRecorder, timeout time.Duration, logger *slog.Logger) *Handler {
	return &Handler{service: service, recorder: recorder, timeout: timeout, logger: logger}
}

// RegisterRoutes mounts the endpoint on an already authenticated group.
func RegisterRoutes(group *gin.RouterGroup, h *Handler) {
	group.POST("/documents/period-report", h.create)
}

func (h *Handler) create(c *gin.Context) {
	started := time.Now()
	system := auth.FromContext(c).System
	rows := 0
	record := func(outcome activity.Outcome, size int) {
		h.recorder.Record(activity.Event{
			System: system, Template: TemplateCode, Format: "a4", Items: rows, Bytes: size,
			DurationMS: time.Since(started).Milliseconds(), Outcome: outcome,
		})
	}

	body := http.MaxBytesReader(c.Writer, c.Request.Body, MaxBodyBytes)
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	var req request
	if err := decoder.Decode(&req); err != nil {
		record(activity.OutcomeInvalid, 0)
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpserver.Error(c, http.StatusRequestEntityTooLarge, CodePayloadTooLarge, "Request body is too large.")
			return
		}
		httpserver.Error(c, http.StatusBadRequest, CodeInvalidRequest, "Request body must be valid JSON with known fields.")
		return
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		record(activity.OutcomeInvalid, 0)
		httpserver.Error(c, http.StatusBadRequest, CodeInvalidRequest, "Request body must contain a single JSON object.")
		return
	}
	report, err := req.toReport()
	rows = report.Rows()
	if err != nil {
		record(activity.OutcomeInvalid, 0)
		httpserver.Error(c, http.StatusUnprocessableEntity, CodeInvalidPayload, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()
	pdf, err := h.service.Render(ctx, report)
	var validation *ValidationError
	switch {
	case errors.As(err, &validation):
		record(activity.OutcomeInvalid, 0)
		httpserver.Error(c, http.StatusUnprocessableEntity, CodeInvalidPayload, validation.Error())
		return
	case errors.Is(err, ErrBusy):
		record(activity.OutcomeBusy, 0)
		httpserver.Error(c, http.StatusServiceUnavailable, CodeBusy, "The service is busy. Try again shortly.")
		return
	case err != nil:
		record(activity.OutcomeError, 0)
		// The payload holds financial data, so only metadata is logged.
		h.logger.Error("period report render failed", "request_id", c.GetHeader("X-Request-ID"), "error", err.Error())
		httpserver.Error(c, http.StatusInternalServerError, CodeRenderFailed, "Unable to render the document.")
		return
	}
	record(activity.OutcomeOK, len(pdf))
	h.logger.Info("period report rendered",
		"request_id", c.GetHeader("X-Request-ID"),
		"system", system,
		"currencies", len(report.Currencies),
		"rows", rows,
		"bytes", len(pdf),
		"duration_ms", time.Since(started).Milliseconds(),
	)
	c.Header("Content-Disposition", `inline; filename="`+FileName(report.From, report.To)+`"`)
	c.Data(http.StatusOK, "application/pdf", pdf)
}

// toReport parses the dates and maps the DTOs; the figures are validated by
// Report.Validate. On a date error it still returns the rows decoded so far.
func (r request) toReport() (Report, error) {
	report := Report{Currencies: make([]Currency, len(r.Currencies))}
	for i, c := range r.Currencies {
		report.Currencies[i] = Currency{
			Code: c.Currency, Income: c.IncomeTotal, Expense: c.ExpenseTotal, Balance: c.Balance,
			Expenses: lines(c.Expenses), Incomes: lines(c.Incomes),
		}
	}
	var err error
	if report.From, err = ParseDate("from", r.From); err != nil {
		return report, err
	}
	if report.To, err = ParseDate("to", r.To); err != nil {
		return report, err
	}
	return report, nil
}

func lines(in []lineRequest) []Line {
	out := make([]Line, len(in))
	for i, l := range in {
		out[i] = Line(l)
	}
	return out
}
