package httpserver

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestID propagates X-Request-ID (generating one when missing or malformed)
// and sets the headers every response carries. This service is called only by
// other backends, so it sends no CORS headers.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if _, err := uuid.Parse(requestID); err != nil {
			requestID = uuid.NewString()
		}
		c.Request.Header.Set("X-Request-ID", requestID)
		c.Header("X-Request-ID", requestID)
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Cache-Control", "no-store")
		c.Next()
	}
}
