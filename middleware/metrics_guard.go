package middleware

import (
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

// MetricsGuard 保护 /metrics 端点：配置了 observability.metrics_token 时要求
// Authorization: Bearer <token>；未配置时行为不变（透传）。
// 目的：生产环境可安全暴露指标给内网采集器（sse-agent），同时不破坏本地开发。
var (
	metricsTokenOnce sync.Once
	metricsToken     string
)

func metricsTokenValue() string {
	metricsTokenOnce.Do(func() {
		metricsToken = strings.TrimSpace(viper.GetString("observability.metrics_token"))
	})
	return metricsToken
}

func MetricsGuard(next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := metricsTokenValue()
		if token == "" {
			next(c)
			return
		}
		const prefix = "Bearer "
		auth := c.Request.Header.Get("Authorization")
		if len(auth) <= len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) || auth[len(prefix):] != token {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "metrics token required"})
			return
		}
		next(c)
	}
}
