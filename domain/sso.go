package domain

import (
	"strings"
	"time"
)

// SsoBinding UniSSO 身份与集市账号的绑定关系。
type SsoBinding struct {
	ID          int
	LocalUserID int
	SsoSub      string
	SsoUsername string
	CreatedAt   time.Time
}

// SsoStatePayload Redis key `sso:state:{state}` 的值，回调时一次性消费。
type SsoStatePayload struct {
	Verifier string `json:"verifier"`
	BindUID  int    `json:"bind_uid,omitempty"` // >0：已登录用户主动绑定
}

// SsoTicketPayload Redis key `sso:ticket:{ticket}` 的值。
// Type: login(已绑定直接登录) / bind_needed(待绑定) / bound(绑定完成)。
type SsoTicketPayload struct {
	Type        string `json:"type"`
	UID         int    `json:"uid,omitempty"`
	Sub         string `json:"sub,omitempty"`
	SsoUsername string `json:"sso_username,omitempty"`
	SsoEmail    string `json:"sso_email,omitempty"`
}

// SsoBool 兼容 true/"true"/1 三种序列化；缺失或无法识别一律 false。
type SsoBool bool

func (b *SsoBool) UnmarshalJSON(data []byte) error {
	switch strings.Trim(strings.TrimSpace(string(data)), `"`) {
	case "true", "True", "TRUE", "1":
		*b = true
	default:
		*b = false
	}
	return nil
}

// SsoUserinfo UniSSO userinfo 端点返回（仅取需要的字段）。
type SsoUserinfo struct {
	Sub               string  `json:"sub"`
	PreferredUsername string  `json:"preferred_username"`
	Email             string  `json:"email"`
	Internal          SsoBool `json:"internal"`
}

// SsoSession unisso password/verify、session/verify 返回的共享 SSO 会话元数据。
// Cookie 值不可自证，平台只把它当会话指针，必须服务端回 unisso 校验。
type SsoSession struct {
	ID           string `json:"sso_session"`
	CookieName   string `json:"sso_cookie_name"`
	CookieDomain string `json:"sso_cookie_domain"`
	MaxAge       int    `json:"sso_max_age"`
}

// SsoPasswordVerifyUser POST /api/oauth/password/verify 的 user 节点。
type SsoPasswordVerifyUser struct {
	Sub               string  `json:"sub"`
	SseSub            string  `json:"sse_sub"`
	Internal          SsoBool `json:"internal"`
	Name              string  `json:"name"`
	Email             string  `json:"email"`
	PreferredUsername string  `json:"preferred_username"`
}

// SsoExchangeResult POST /api/sso/exchange 的响应体。
type SsoExchangeResult struct {
	Type         string `json:"type"`
	Token        string `json:"token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	SsoUsername  string `json:"sso_username,omitempty"`
	SsoEmail     string `json:"sso_email,omitempty"`
}

// SsoStatus GET /api/sso/status 的响应体。
type SsoStatus struct {
	Bound       bool      `json:"bound"`
	SsoUsername string    `json:"sso_username,omitempty"`
	BoundAt     time.Time `json:"bound_at,omitempty"`
}
