package middleware

import (
	"strconv"
	"time"

	"loginTest/observability"

	"github.com/gin-gonic/gin"
)

// PrometheusMetrics 记录 HTTP 请求延迟指标。
func PrometheusMetrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unknown"
		}
		status := strconv.Itoa(c.Writer.Status())
		observability.HTTPRequestDuration.WithLabelValues(
			c.Request.Method,
			route,
			status,
		).Observe(time.Since(start).Seconds())
	}
}
