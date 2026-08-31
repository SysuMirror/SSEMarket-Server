package observability

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type ctxKey struct{}

const RequestIDHeader = "X-Request-ID"

// RequestIDFromContext 从 context 读取 request id。
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(ctxKey{}).(string); ok {
		return id
	}
	return ""
}

// WithRequestID 将 request id 写入 context。
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// RequestIDFromGin 从 Gin context 读取 request id。
func RequestIDFromGin(c *gin.Context) string {
	if v, ok := c.Get(RequestIDHeader); ok {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}

// NewRequestID 生成新的 request id。
func NewRequestID() string {
	return uuid.New().String()
}

// SetSpanRequestID 将 request id 写入当前 span。
func SetSpanRequestID(ctx context.Context, id string) {
	if id == "" {
		return
	}
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetAttributes(attribute.String("request.id", id))
	}
}
