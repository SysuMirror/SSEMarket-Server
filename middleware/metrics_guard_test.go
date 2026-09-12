package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

// resetGuard 让 metricsTokenOnce 重新执行（下一个读取者重新从 viper 取值）。
func resetGuard() { metricsTokenOnce = sync.Once{} }

func newGuardRouter(hit *bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/metrics", MetricsGuard(func(c *gin.Context) {
		*hit = true
		c.String(http.StatusOK, "metrics body")
	}))
	return r
}

func TestMetricsGuardPassthroughWhenNoToken(t *testing.T) {
	viper.Set("observability.metrics_token", "")
	resetGuard()
	hit := false
	r := newGuardRouter(&hit)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	if w.Code != http.StatusOK || w.Body.String() != "metrics body" || !hit {
		t.Fatalf("passthrough broken: code=%d body=%q hit=%v", w.Code, w.Body.String(), hit)
	}
}

func TestMetricsGuardEnforcesBearer(t *testing.T) {
	viper.Set("observability.metrics_token", "s3cret")
	resetGuard()
	t.Cleanup(func() { viper.Set("observability.metrics_token", "") })

	hit := false
	r := newGuardRouter(&hit)

	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"missing header", "", 401},
		{"wrong scheme", "Basic s3cret", 401},
		{"wrong token", "Bearer nope", 401},
		{"case-insensitive scheme", "bearer s3cret", 200},
		{"valid", "Bearer s3cret", 200},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/metrics", nil)
		if tc.header != "" {
			req.Header.Set("Authorization", tc.header)
		}
		r.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Fatalf("%s: got %d want %d", tc.name, w.Code, tc.want)
		}
	}
	if !hit {
		t.Fatalf("valid request never reached handler")
	}
}
