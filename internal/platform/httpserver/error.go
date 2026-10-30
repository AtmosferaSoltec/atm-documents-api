package httpserver

import "github.com/gin-gonic/gin"

// ErrorDetail is the body of every error response.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ErrorResponse is the JSON envelope of every error response.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// Error aborts the request with a stable error code.
func Error(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, ErrorResponse{Error: ErrorDetail{Code: code, Message: message}})
}
