// 返回token中间件，不用管，要看也行。模板写法来着，至于为什么，我也不是很懂
package middleware

import (
	"net/http"
	"strings"

	"loginTest/common"
	"loginTest/service"

	"github.com/gin-gonic/gin"
)

func AuthMiddleware_admin() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		//获取authorization header
		tokenString := ctx.GetHeader("Authorization")
		//若token为空或token格式不正确
		if tokenString == "" || len(tokenString) <= 7 || !strings.HasPrefix(tokenString, "Bearer ") {
			ctx.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "权限不足"})
			ctx.Abort()
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
		if err != nil || admin == nil || admin.ID == 0 || !admin.IsActive {
			ctx.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "权限不足"})
			ctx.Abort()
			return
		}
		ctx.Set("admin", admin)
		ctx.Next()
	}
}
