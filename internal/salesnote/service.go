package salesnote

import (
	"context"
	"fmt"
)

// Renderer draws a validated note as a PDF.
type Renderer interface {
	Render(note Note) ([]byte, error)
}

// Service renders sales notes, bounding how many render at once.
type Service struct {
	renderer Renderer
	slots    chan struct{}
}

// NewService builds a Service that renders at most maxConcurrent notes at once.
func NewService(renderer Renderer, maxConcurrent int) *Service {
	return &Service{renderer: renderer, slots: make(chan struct{}, maxConcurrent)}
}

// Render validates the note and returns its PDF.
func (s *Service) Render(ctx context.Context, note Note) ([]byte, error) {
	if err := note.Validate(); err != nil {
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
	pdf, err := s.renderer.Render(note)
	if err != nil {
		return nil, fmt.Errorf("render sales note: %w", err)
	}
	return pdf, nil
}
