package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"loginTest/common"
	"loginTest/domain"

	"github.com/gin-gonic/gin"
)

func parseBearerToken(c *gin.Context) (token string, isWS bool, ok bool) {
	tokenString := c.GetHeader("Authorization")
	if tokenString == "" {
		tokenString = c.Query("token")
		isWS = true
	}
	if tokenString == "" {
		return "", isWS, false
	}
	if !isWS {
		if len(tokenString) <= 7 || !strings.HasPrefix(tokenString, "Bearer ") {
			return "", isWS, false
		}
		return tokenString[7:], isWS, true
	}
	if len(tokenString) <= 7 {
		return "", isWS, false
	}
	return tokenString, isWS, true
}

func loadUserFromToken(c *gin.Context, tokenString string, isWS bool) (*domain.User, bool) {
	token, claims, err := common.ParseToken(tokenString)
	if err != nil || !token.Valid || claims.UserID == 0 {
		if isWS {
			fmt.Println(tokenString)
		}
		return nil, false
	}
	user, err := LoadUserSession(c.Request.Context(), claims.UserID)
	if err != nil || user == nil || user.ID == 0 {
		return nil, false
	}
	return user, true
}

// StrictAuthMiddleware Fail-Closed：/api/auth/* 必须登录。
func StrictAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, isWS, ok := parseBearerToken(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "权限不足"})
			c.Abort()
			return
		}
		user, valid := loadUserFromToken(c, tokenString, isWS)
		if !valid {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "权限不足"})
			c.Abort()
			return
		}
		c.Set("user", user)
		c.Next()
	}
}

// OptionalAuthMiddleware 可选登录：/api/noauth/* 游客可访问，有 token 则注入用户。
func OptionalAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, isWS, ok := parseBearerToken(c)
		if !ok {
			c.Set("user", nil)
			c.Next()
			return
		}
		user, valid := loadUserFromToken(c, tokenString, isWS)
		if !valid {
			c.Set("user", nil)
		} else {
			c.Set("user", user)
		}
		c.Next()
	}
}
