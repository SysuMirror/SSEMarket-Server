package middleware

import (
	"net/http"
	"strings"

	"loginTest/service"

	"github.com/gin-gonic/gin"
)

// ForumPATAuth deliberately applies only to the native forum MCP route. A PAT
// must not become a login JWT or gain access to unrelated /api/auth endpoints.
func ForumPATAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || !strings.HasPrefix(parts[1], "sk_xq_") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "invalid bearer PAT"})
			return
		}
		identity, err := service.Users.VerifyAPIToken(c.Request.Context(), parts[1])
		if err != nil || identity == nil || !identity.IsActive || identity.UserID <= 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "invalid or revoked PAT"})
			return
		}
		// Reload on every request: revocation and bans must not depend on an MCP
		// session cache. No credentials are included in downstream context.
		user, err := service.Users.GetByID(c.Request.Context(), identity.UserID)
		if err != nil || user == nil || user.ID != identity.UserID {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "account unavailable"})
			return
		}
		c.Set("user", user)
		c.Set("forum_pat_scopes", identity.Scopes)
		c.Next()
	}
}
