package periodreport

import (
	"context"
	"fmt"
)

// Renderer draws a validated report as a PDF.
type Renderer interface {
	Render(report Report) ([]byte, error)
}

// Service renders period reports, bounding how many render at once.
type Service struct {
	renderer Renderer
	slots    chan struct{}
}

// NewService builds a Service that renders at most maxConcurrent reports at once.
func NewService(renderer Renderer, maxConcurrent int) *Service {
	return &Service{renderer: renderer, slots: make(chan struct{}, maxConcurrent)}
}

// Render validates the report and returns its PDF.
func (s *Service) Render(ctx context.Context, report Report) ([]byte, error) {
	if err := report.Validate(); err != nil {
		return nil, err
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		return nil, ErrBusy
	}
	if err := ctx.Err(); err != nil {
		return nil, ErrBusy
	}
	pdf, err := s.renderer.Render(report)
	if err != nil {
		return nil, fmt.Errorf("render period report: %w", err)
	}
	return pdf, nil
}
