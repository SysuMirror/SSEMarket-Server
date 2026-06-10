// 返回token中间件，不用管，要看也行。模板写法来着，至于为什么，我也不是很懂
package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"loginTest/common"
	"loginTest/service"

	"github.com/gin-gonic/gin"
)

func AuthMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		//获取authorization header
		isWs := false //ws需要query获得token
		tokenString := ctx.GetHeader("Authorization")
		if tokenString == "" {
			tokenString = ctx.Query("token")
			isWs = true
		}
		//若不是ws连接并且token为空或token格式不正确
		if !isWs && (tokenString == "" || len(tokenString) <= 7 || !strings.HasPrefix(tokenString, "Bearer ")) {
			// 如果请求头中没有Authorization信息，或者信息不合法，则视为游客访问。
			ctx.Set("user", nil)
			ctx.Next()
			return
		}
		//当ws连接并且token小于7
		if isWs && len(tokenString) <= 7 {
			ctx.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "权限不足"})
			ctx.Abort()
			fmt.Println(tokenString)
			return
		}
		//若不是ws连接，表示有bearer
		if !isWs {
			tokenString = tokenString[7:]
		}
		token, claims, err := common.ParseToken(tokenString)
		if err != nil || !token.Valid {
			ctx.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "权限不足"})
			ctx.Abort()
			if isWs {
				fmt.Println(tokenString)
			}
			return
		}
		//验证通过后获取claims中的userId
		userId := claims.UserID
		if userId == 0 {
			ctx.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "权限不足"})
			ctx.Abort()
			if isWs {
				fmt.Println(tokenString)
			}
			return
		}
		user, err := LoadUserSession(ctx.Request.Context(), userId)
		if err != nil || user == nil || user.ID == 0 {
			ctx.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "权限不足"})
			ctx.Abort()
			return
		}
		ctx.Set("user", user)
		ctx.Next()
	}
}

func AuthMiddleware_admin() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		//获取authorization header
		tokenString := ctx.GetHeader("Authorization")
		//若token为空或token格式不正确
		if tokenString == "" || len(tokenString) <= 7 || !strings.HasPrefix(tokenString, "Bearer ") {
			ctx.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "权限不足"})
			return
		}
		tokenString = tokenString[7:]
		token, claims_admin, err := common.ParseToken_admin(tokenString)
		if err != nil || !token.Valid {
			ctx.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "权限不足"})
			ctx.Abort()
			return
		}
		//验证通过后获取claims中的adminId
		adminId := claims_admin.AdminID
		if adminId == 0 {
			ctx.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "权限不足"})
			ctx.Abort()
			return
		}
		admin, err := service.Admins.GetByID(ctx.Request.Context(), adminId)
		if err != nil || admin == nil || admin.ID == 0 {
			ctx.Set("admin", nil)
		} else {
			ctx.Set("admin", admin)
		}
		ctx.Next()
	}
}
