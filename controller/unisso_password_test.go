package controller

import (
	"context"
	"encoding/json"
	"loginTest/domain"
	"loginTest/service"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

func TestLocalPasswordIsNotAccepted(t *testing.T) {
	t.Cleanup(viper.Reset)
	viper.Set("sso.enabled", false)
	for _, test := range []struct {
		name    string
		handler gin.HandlerFunc
		want    int
	}{
		{"login", Login, http.StatusServiceUnavailable},
		{"legacy_reset", ModifyPassword, http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := gin.New()
			r.Use(gin.RecoveryWithWriter(httptest.NewRecorder()))
			r.POST("/", test.handler)
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != test.want {
				t.Fatalf("status = %d, want %d", w.Code, test.want)
			}
		})
	}
}

func TestUniSSOPasswordResetUsesCSRFAndPreservesInvalidCode(t *testing.T) {
	t.Cleanup(viper.Reset)
	viper.Set("sso.enabled", true)
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/csrf-token":
			http.SetCookie(w, &http.Cookie{Name: "unisso_csrf_context", Value: "ctx", Secure: true, Path: "/"})
			_, _ = w.Write([]byte(`{"csrf_token":"token"}`))
		case "/api/auth/password-reset/confirm":
			cookie, _ := r.Cookie("unisso_csrf_context")
			var input resetInput
			if cookie == nil || cookie.Value != "ctx" || r.Header.Get("X-CSRF-Token") != "token" ||
				json.NewDecoder(r.Body).Decode(&input) != nil || input.Email != "student@example.test" ||
				input.Code != "123456" || input.NewPassword != "SamplePass#9" {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"ok":false}`))
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"ok":false,"message":"验证码错误"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	viper.Set("sso.password_verify_url", upstream.URL+"/api/oauth/password/verify")
	status, result := callUnissoPasswordReset(context.Background(), resetInput{
		Email: "student@example.test", Code: "123456", NewPassword: "SamplePass#9",
	}, true, upstream.Client().Transport)
	if status != http.StatusUnauthorized || result.Message != "验证码错误" {
		t.Fatalf("status=%d message=%q", status, result.Message)
	}
}

type unavailablePasswordAuthority struct{ service.SsoService }

func (unavailablePasswordAuthority) LoginViaUnisso(context.Context, string, string) (*domain.AuthTokens, *domain.SsoSession, error) {
	return nil, nil, service.ErrSsoUnavailable
}

func TestPasswordLoginReturns503WhenUniSSOIsUnavailable(t *testing.T) {
	t.Cleanup(viper.Reset)
	viper.Set("sso.enabled", true)
	before := service.Sso
	service.Sso = unavailablePasswordAuthority{}
	t.Cleanup(func() { service.Sso = before })

	r := gin.New()
	r.POST("/login", Login)
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"email":"student@example.test","password":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}
