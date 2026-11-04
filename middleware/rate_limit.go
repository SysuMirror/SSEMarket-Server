package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"loginTest/core"
	"loginTest/domain"

	"github.com/gin-gonic/gin"
)

// UserRateLimit 通用「按登录用户」固定窗口限流中间件。
//
// 与具体业务解耦：scope 由调用方命名（如 "presence-heartbeat"），
// 控制器只处理业务，不需要知道限流规则。
//
//   - 计数放 Redis，key = ratelimit:<scope>:<userID>:<窗口序号>，随窗口自动过期；
//   - Redis 不可用或报错时放行（fail-open），在线人数/搜索都是弱功能，
//     不能因为限流器自身故障拖垮页面；
//   - 超限返回 429 + Retry-After，前端可据此退避。
//
// 说明：固定窗口允许窗口边界附近的短时突发（最多约 2 倍 limit），
// 对心跳/搜索这类接口足够，且每次请求只有 1 次 Redis pipeline RTT。
func UserRateLimit(scope string, limit int, window time.Duration) gin.HandlerFunc {
	if scope == "" || limit <= 0 || window <= 0 {
		panic("UserRateLimit: scope/limit/window 必须为正")
	}
	windowMs := window.Milliseconds()
	return func(c *gin.Context) {
		value, exists := c.Get("user")
		user, ok := value.(*domain.User)
		if !exists || !ok || user == nil || user.ID <= 0 {
			// 正常不会走到这里（auth 组已有 StrictAuthMiddleware），
			// 直接放行，避免中间件顺序变化时误伤。
			c.Next()
			return
		}
		if core.MyRedis == nil {
			c.Next()
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 500*time.Millisecond)
		defer cancel()
		bucket := time.Now().UnixMilli() / windowMs
		key := fmt.Sprintf("ratelimit:%s:%d:%d", scope, user.ID, bucket)

		pipe := core.MyRedis.TxPipeline()
		countCmd := pipe.Incr(ctx, key)
		pipe.Expire(ctx, key, window+time.Second)
		if _, err := pipe.Exec(ctx); err != nil {
			c.Next()
			return
		}
		if countCmd.Val() <= int64(limit) {
			c.Next()
			return
		}

		c.Header("Retry-After", strconv.Itoa(int(window.Seconds())))
		c.JSON(http.StatusTooManyRequests, gin.H{
			"code": http.StatusTooManyRequests,
			"msg":  "操作过于频繁，请稍后再试",
		})
		c.Abort()
	}
}

// IPRateLimit 通用「按客户端 IP」固定窗口限流中间件，用于无登录态端点（如静默 SSO）。
//
//   - 计数放 Redis，key = ratelimit:ip:<scope>:<ip>:<窗口序号>，随窗口自动过期；
//   - Redis 不可用或报错时放行（fail-open），身份兑换失败本就静默回落登录表单，
//     不能因为限流器自身故障把用户挡在门外；
//   - 超限返回 429 + Retry-After。
func IPRateLimit(scope string, limit int, window time.Duration) gin.HandlerFunc {
	if scope == "" || limit <= 0 || window <= 0 {
		panic("IPRateLimit: scope/limit/window 必须为正")
	}
	windowMs := window.Milliseconds()
	return func(c *gin.Context) {
		if core.MyRedis == nil {
			c.Next()
			return
		}
		ip := clientIPFromProxy(c)
		ctx, cancel := context.WithTimeout(c.Request.Context(), 500*time.Millisecond)
		defer cancel()
		bucket := time.Now().UnixMilli() / windowMs
		key := fmt.Sprintf("ratelimit:ip:%s:%s:%d", scope, ip, bucket)

		pipe := core.MyRedis.TxPipeline()
		countCmd := pipe.Incr(ctx, key)
		pipe.Expire(ctx, key, window+time.Second)
		if _, err := pipe.Exec(ctx); err != nil {
			c.Next()
			return
		}
		if countCmd.Val() <= int64(limit) {
			c.Next()
			return
		}

		c.Header("Retry-After", strconv.Itoa(int(window.Seconds())))
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"code": http.StatusTooManyRequests,
			"msg":  "sso_unavailable",
		})
	}
}
