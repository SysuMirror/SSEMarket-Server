package middleware

import (
	"loginTest/observability"

	"github.com/gin-gonic/gin"
)

// RequestID 为每个请求注入 X-Request-ID，贯穿 context 与响应头。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(observability.RequestIDHeader)
		if id == "" {
			id = observability.NewRequestID()
		}
		c.Set(observability.RequestIDHeader, id)
		c.Header(observability.RequestIDHeader, id)
		c.Request = c.Request.WithContext(observability.WithRequestID(c.Request.Context(), id))
		c.Next()
	}
}

// AttachRequestIDToSpan 将 X-Request-ID 写入当前 trace span（需在 otelgin 之后）。
func AttachRequestIDToSpan() gin.HandlerFunc {
	return func(c *gin.Context) {
		if id := observability.RequestIDFromGin(c); id != "" {
			observability.SetSpanRequestID(c.Request.Context(), id)
		}
		c.Next()
	}
}
