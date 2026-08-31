package controller

import (
	"loginTest/domain"

	"github.com/gin-gonic/gin"
)

// viewerFromAuth returns the authenticated user injected by StrictAuthMiddleware.
func viewerFromAuth(c *gin.Context) (*domain.User, bool) {
	value, exists := c.Get("user")
	if !exists {
		return nil, false
	}
	user, ok := value.(*domain.User)
	if !ok || user.ID == 0 {
		return nil, false
	}
	return user, true
}

// requireViewerLegacyPhone identifies the current user from JWT only.
// The legacy userTelephone request field is ignored; kept for backward-compatible request bodies.
func requireViewerLegacyPhone(c *gin.Context, _ string) (*domain.User, bool) {
	return viewerFromAuth(c)
}
