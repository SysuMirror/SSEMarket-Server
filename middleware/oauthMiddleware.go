package middleware

import (
	"errors"
	"net/http"
	"strings"

	"loginTest/common"
	"loginTest/domain"
	"loginTest/service"

	"github.com/gin-gonic/gin"
)

// OAuthMiddleware OAuth2认证中间件
func OAuthMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// 获取Authorization header
		tokenString := ctx.GetHeader("Authorization")

		// 检查token格式
		if tokenString == "" || len(tokenString) <= 7 || !strings.HasPrefix(tokenString, "Bearer ") {
			ctx.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "OAuth2 token缺失或格式错误"})
			ctx.Abort()
			return
		}

		// 提取token
		tokenString = tokenString[7:]

		// 解析OAuth2 token
		token, claims, err := common.ParseOAuth2Token(tokenString)
		if err != nil || !token.Valid {
			ctx.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "OAuth2 token无效或已过期"})
			ctx.Abort()
			return
		}

		// 验证用户ID
		userID := claims.UserID
		if userID == 0 {
			ctx.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "OAuth2 token中用户ID无效"})
			ctx.Abort()
			return
		}

		// 验证应用ID
		appID := claims.AppID
		if appID == "" {
			ctx.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "OAuth2 token中应用ID无效"})
			ctx.Abort()
			return
		}

		u, err := service.Users.GetByID(ctx.Request.Context(), userID)
		if err != nil || u == nil || u.ID == 0 {
			ctx.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "用户不存在"})
			ctx.Abort()
			return
		}

		app, err := service.Users.CheckOAuth2App(ctx.Request.Context(), appID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				ctx.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "OAuth2应用不存在或未激活"})
			} else {
				ctx.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "OAuth2应用校验失败"})
			}
			ctx.Abort()
			return
		}

		// 将用户信息、应用信息和权限范围写入上下文
		ctx.Set("oauth2_user", u)
		ctx.Set("oauth2_app", app)
		ctx.Set("oauth2_scope", claims.Scope)
		ctx.Set("oauth2_user_id", userID)
		ctx.Set("oauth2_app_id", appID)

		ctx.Next()
	}
}
