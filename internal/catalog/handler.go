package catalog

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/AtmosferaSoltec/atm-documents-api/internal/activity"
	"github.com/AtmosferaSoltec/atm-documents-api/internal/platform/httpserver"
	"github.com/AtmosferaSoltec/atm-documents-api/internal/salesnote"
	"github.com/gin-gonic/gin"
)

// Renderer is the sales note use case the preview needs.
type Renderer interface {
	Render(ctx context.Context, note salesnote.Note) ([]byte, error)
}

// Handler serves the console's management routes.
type Handler struct {
	renderer Renderer
	activity *activity.Recorder
	settings Settings
	logger   *slog.Logger
}

// NewHandler builds the handler.
func NewHandler(renderer Renderer, recorder *activity.Recorder, settings Settings, logger *slog.Logger) *Handler {
	return &Handler{renderer: renderer, activity: recorder, settings: settings, logger: logger}
}

// RegisterRoutes mounts the routes on a group already restricted to the superadmin.
func RegisterRoutes(group *gin.RouterGroup, h *Handler) {
	group.GET("/templates", h.list)
	group.GET("/templates/:code/preview", h.preview)
	group.GET("/activity", h.recentActivity)
}

func (h *Handler) list(c *gin.Context) {
	c.JSON(http.StatusOK, Listing{
		Service: Service{
			Environment:          h.settings.Environment,
			AllowedSystems:       h.settings.AllowedSystems,
			MaxConcurrentRenders: h.settings.MaxConcurrentRenders,
			RenderTimeoutSeconds: int(h.settings.RenderTimeout.Seconds()),
		},
		Templates: Templates(),
	})
}

// preview renders the template with fictional data, so the console never
// sends or stores a real document.
func (h *Handler) preview(c *gin.Context) {
	template, ok := Find(c.Param("code"))
	if !ok {
		httpserver.Error(c, http.StatusNotFound, "template_not_found", "There is no template with that code.")
		return
	}
	format := c.DefaultQuery("format", string(salesnote.FormatA4))
	if !template.Supports(format) {
		httpserver.Error(c, http.StatusUnprocessableEntity, "invalid_format", "The template does not support that format.")
		return
	}
	pdf, err := h.renderer.Render(c.Request.Context(), salesnote.SampleNote(salesnote.Format(format)))
	if err != nil {
		h.logger.Error("template preview failed", "request_id", c.GetHeader("X-Request-ID"), "template", template.Code, "format", format, "error", err.Error())
		httpserver.Error(c, http.StatusInternalServerError, salesnote.CodeRenderFailed, "Unable to render the preview.")
		return
	}
	c.Header("Content-Disposition", `inline; filename="`+template.Code+`-`+format+`-preview.pdf"`)
	c.Data(http.StatusOK, "application/pdf", pdf)
}

func (h *Handler) recentActivity(c *gin.Context) {
	c.JSON(http.StatusOK, h.activity.Snapshot())
}
