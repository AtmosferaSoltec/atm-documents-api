package salesnote

import "errors"

// Stable error codes returned in the HTTP error body.
const (
	CodeInvalidRequest  = "invalid_request"
	CodeInvalidPayload  = "invalid_payload"
	CodePayloadTooLarge = "payload_too_large"
	CodeRenderFailed    = "render_failed"
	CodeBusy            = "service_busy"
)

// ErrBusy means every render slot stayed taken until the request gave up.
var ErrBusy = errors.New("render capacity exhausted")
