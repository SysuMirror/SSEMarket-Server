package controller

// 内部运维邮件通道：sse-agent → Go 后端（raghook 的反向同款，X-Internal-Token 守卫）。
// 复用 SendPlainEmail（SMTP 凭据单一来源在 Go 侧）；防线：
//   1) INTERNAL_TOKEN 未配置或头不匹配 → 403（fail-closed）
//   2) 收件人白名单 INTERNAL_MAIL_TO（逗号分隔；未配置=通道禁用），请求可省 to 取默认
//   3) 进程级限频 1 封/分钟（报警风暴保护；host 脚本自身另有 30min 冷却）
//   4) body 截断 16KB

import (
	"os"
	"strings"
	"sync"
	"time"

	"loginTest/api"

	"github.com/gin-gonic/gin"
)

var (
	internalMailOnce sync.Once
	internalToken    string
	internalMailTo   []string
	mailMu           sync.Mutex
	lastMailSent     time.Time
)

func loadInternalMailEnv() {
	internalMailOnce.Do(func() {
		internalToken = strings.TrimSpace(os.Getenv("INTERNAL_TOKEN"))
		for _, v := range strings.Split(os.Getenv("INTERNAL_MAIL_TO"), ",") {
			if v = strings.TrimSpace(v); v != "" {
				internalMailTo = append(internalMailTo, v)
			}
		}
	})
}

// InternalMail POST /internal/mail {to?, subject, body}
func InternalMail(c *gin.Context) {
	loadInternalMailEnv()
	if internalToken == "" || c.Request.Header.Get("X-Internal-Token") != internalToken {
		c.AbortWithStatusJSON(403, gin.H{"code": 403, "msg": "forbidden"})
		return
	}
	if len(internalMailTo) == 0 {
		c.AbortWithStatusJSON(403, gin.H{"code": 403, "msg": "internal mail disabled (INTERNAL_MAIL_TO unset)"})
		return
	}
	var req struct {
		To      string `json:"to"`
		Subject string `json:"subject" binding:"required"`
		Body    string `json:"body" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(422, gin.H{"code": 422, "msg": "subject/body required"})
		return
	}
	to := strings.TrimSpace(req.To)
	if to == "" {
		to = internalMailTo[0] // 默认收件人（运维值班）
	} else {
		ok := false
		for _, w := range internalMailTo {
			if w == to {
				ok = true
				break
			}
		}
		if !ok {
			c.AbortWithStatusJSON(403, gin.H{"code": 403, "msg": "recipient not in whitelist"})
			return
		}
	}
	if len(req.Body) > 16*1024 {
		req.Body = req.Body[:16*1024] + "\n…(截断)"
	}

	mailMu.Lock()
	if !lastMailSent.IsZero() && time.Since(lastMailSent) < time.Minute {
		mailMu.Unlock()
		c.AbortWithStatusJSON(429, gin.H{"code": 429, "msg": "rate limited: 1 mail/min"})
		return
	}
	lastMailSent = time.Now()
	mailMu.Unlock()

	if err := api.SendPlainEmail(to, req.Subject, req.Body); err != nil {
		c.JSON(502, gin.H{"code": 502, "msg": "send failed: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"code": 0, "msg": "sent", "to": to})
}
