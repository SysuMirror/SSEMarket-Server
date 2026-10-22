package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"loginTest/core"
	"loginTest/domain"
	"loginTest/middleware"

	"github.com/gin-gonic/gin"
)

func TestUserRateLimitFailsOpenWithoutRedis(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := core.MyRedis
	core.MyRedis = nil
	t.Cleanup(func() { core.MyRedis = previous })

	r := gin.New()
	r.POST("/x", func(c *gin.Context) {
		c.Set("user", &domain.User{ID: 1})
	}, middleware.UserRateLimit("test", 1, time.Minute), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	// Redis 不可用时应放行，而不是把功能整体挡掉。
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/x", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("第 %d 次请求状态码 = %d, 期望 %d", i+1, w.Code, http.StatusNoContent)
		}
	}
}

func TestUserRateLimitPassesThroughWithoutUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", middleware.UserRateLimit("test", 1, time.Minute), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("状态码 = %d, 期望 %d", w.Code, http.StatusNoContent)
	}
}
