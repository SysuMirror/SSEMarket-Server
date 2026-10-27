package middleware

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"loginTest/domain"
	"loginTest/service"

	"github.com/gin-gonic/gin"
)

type forumPATUsers struct {
	service.UserService
	active      bool
	verifyCalls int
}

func (s *forumPATUsers) VerifyAPIToken(_ context.Context, token string) (*domain.UserAPITokenVerify, error) {
	s.verifyCalls++
	if token != "sk_xq_test" {
		return nil, errors.New("invalid credential")
	}
	return &domain.UserAPITokenVerify{UserID: 7, Scopes: []string{"forum:read"}, IsActive: s.active}, nil
}
func (s *forumPATUsers) GetByID(context.Context, int) (*domain.User, error) {
	return &domain.User{ID: 7}, nil
}

func TestForumPATAuthRejectsLoginTokensAndRevocation(t *testing.T) {
	old := service.Users
	users := &forumPATUsers{active: true}
	service.Users = users
	t.Cleanup(func() { service.Users = old })
	r := gin.New()
	r.POST("/mcp", ForumPATAuth(), func(c *gin.Context) { c.Status(204) })
	for _, tc := range []struct {
		auth, path string
		want       int
	}{{"", "/mcp", 401}, {"Bearer jwt-login", "/mcp", 401}, {"Basic sk_xq_test", "/mcp", 401}, {"", "/mcp?token=sk_xq_test", 401}, {"Bearer sk_xq_test", "/mcp", 204}} {
		req := httptest.NewRequest("POST", tc.path, nil)
		req.Header.Set("Authorization", tc.auth)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Fatalf("got %d want %d", w.Code, tc.want)
		}
	}
	users.active = false
	req := httptest.NewRequest("POST", "/mcp", nil)
	req.Header.Set("Authorization", "Bearer sk_xq_test")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 401 || users.verifyCalls != 2 {
		t.Fatalf("revoked PAT accepted or identity cached: %d calls %d", w.Code, users.verifyCalls)
	}
}
