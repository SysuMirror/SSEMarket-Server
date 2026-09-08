package middleware

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"loginTest/api"
)

// LoginRateLimit —— 按「IP + 浏览器指纹」维度对登录接口限流,触发后封禁并发邮件报警。
//
// 背景: 全站用户走同一校园网出口,纯 IP 限流会误伤,因此叠加浏览器指纹维度:
//   * 指纹优先取请求头 X-Device-ID(前端接入 FingerprintJS 或本地持久设备ID后自动生效)
//   * 未携带时降级为 UA + Accept-Language + Accept 的哈希(区分不同设备/浏览器,
//     扫号脚本通常固定 UA,会聚集到同一键被精准限流;UA 为空的裸脚本同样聚集)
//
// 环境变量(均有默认值):
//   LOGIN_LIMIT_MAX       单窗口最大调用次数(默认 500)
//   LOGIN_LIMIT_WINDOW    统计窗口,秒(默认 60)
//   LOGIN_LIMIT_BLOCK     触发后封禁时长,秒(默认 180)
//   LOGIN_ALERT_EMAIL     报警接收邮箱(默认 2215020897@qq.com)
//   LOGIN_ALERT_COOLDOWN  同一键两次报警最小间隔,秒(默认 900)
func LoginRateLimit() gin.HandlerFunc {
	maxN := envIntOrDefault("LOGIN_LIMIT_MAX", 500)
	window := time.Duration(envIntOrDefault("LOGIN_LIMIT_WINDOW", 60)) * time.Second
	blockDur := time.Duration(envIntOrDefault("LOGIN_LIMIT_BLOCK", 180)) * time.Second
	alertCooldown := time.Duration(envIntOrDefault("LOGIN_ALERT_COOLDOWN", 900)) * time.Second
	alertEmail := os.Getenv("LOGIN_ALERT_EMAIL")
	if alertEmail == "" {
		alertEmail = "2215020897@qq.com"
	}

	return func(c *gin.Context) {
		ip := clientIPFromProxy(c)
		if isInternalIP(ip) { // 回环/内网/白名单放行,避免误伤健康检查与内部调用
			c.Next()
			return
		}
		key := ip + "/" + requestFingerprint(c)

		blocked, hitCount, justBlocked := recordLoginAttempt(key, window, maxN, blockDur, alertCooldown)
		if justBlocked {
			go sendLoginAlert(alertEmail, ip, c.Request.UserAgent(), hitCount, window, blockDur)
		}
		if blocked {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"message": "操作过于频繁，请稍后再试"})
			return
		}
		c.Next()
	}
}

// requestFingerprint 返回 12 位十六进制指纹:
// X-Device-ID > (UA + Accept-Language + Accept) 的 sha1。
func requestFingerprint(c *gin.Context) string {
	if id := c.GetHeader("X-Device-ID"); len(id) >= 8 {
		sum := sha1.Sum([]byte("devid:" + id))
		return hex.EncodeToString(sum[:])[:12]
	}
	ua := c.Request.UserAgent()
	lang := c.GetHeader("Accept-Language")
	acc := c.GetHeader("Accept")
	sum := sha1.Sum([]byte(ua + "|" + lang + "|" + acc))
	return hex.EncodeToString(sum[:])[:12]
}

// clientIPFromProxy 优先取 nginx 覆写过的 X-Real-IP(外部不可伪造)。
func clientIPFromProxy(c *gin.Context) string {
	if rip := c.GetHeader("X-Real-IP"); rip != "" {
		return rip
	}
	return c.ClientIP()
}

type loginLimitState struct {
	windowStart  time.Time
	count        int
	blockedUntil time.Time
	lastAlert    time.Time
}

var (
	loginLimitMu     sync.Mutex
	loginLimitStates = make(map[string]*loginLimitState)
)

func recordLoginAttempt(key string, window time.Duration, maxN int, blockDur, alertCooldown time.Duration) (blocked bool, hitCount int, justBlocked bool) {
	now := time.Now()
	loginLimitMu.Lock()
	defer loginLimitMu.Unlock()

	st, ok := loginLimitStates[key]
	if !ok || now.Sub(st.windowStart) > window {
		// 防 map 无限膨胀:超 5 万条时清理过期项
		if len(loginLimitStates) > 50000 {
			for k, v := range loginLimitStates {
				if now.After(v.blockedUntil) && now.Sub(v.windowStart) > window {
					delete(loginLimitStates, k)
				}
			}
		}
		st = &loginLimitState{windowStart: now}
		loginLimitStates[key] = st
	}

	if now.Before(st.blockedUntil) {
		return true, st.count, false
	}
	st.count++
	if st.count > maxN {
		st.blockedUntil = now.Add(blockDur)
		justBlocked = now.Sub(st.lastAlert) >= alertCooldown
		if justBlocked {
			st.lastAlert = now
		}
		return true, st.count, justBlocked
	}
	return false, st.count, false
}

func sendLoginAlert(emailAddr, ip, ua string, hitCount int, window, blockDur time.Duration) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[login_rate_limit] 发送报警邮件 panic: %v", r)
		}
	}()
	subject := fmt.Sprintf("【安全报警】%s 疑似扫号,已触发登录限流", ip)
	body := fmt.Sprintf(
		"检测到可疑登录行为(按 IP+浏览器指纹 维度):\n\n"+
			"  来源 IP:    %s\n"+
			"  User-Agent: %.120s\n"+
			"  触发规则:   %s 内调用登录接口 %d 次(阈值 %d 次)\n"+
			"  处置:       已封禁该来源 %s\n"+
			"  时间:       %s\n\n"+
			"若为正常用户(校园网共享出口下的同指纹用户聚集),可放宽 LOGIN_LIMIT_MAX;\n"+
			"若确认扫号,建议在防火墙层面进一步封禁该 IP。\n"+
			"本邮件同一来源最少间隔 %s 发送一次。",
		ip, ua, window, hitCount, hitCount-1, blockDur,
		time.Now().Format("2006-01-02 15:04:05 MST"), blockDur,
	)
	if err := api.SendPlainEmail(emailAddr, subject, body); err != nil {
		log.Printf("[login_rate_limit] 报警邮件发送失败 -> %s: %v", emailAddr, err)
	} else {
		log.Printf("[login_rate_limit] 已向 %s 发送限流报警(IP=%s UA=%.60s)", emailAddr, ip, ua)
	}
}

func envIntOrDefault(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}
