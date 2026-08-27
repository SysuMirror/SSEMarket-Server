package domain

import "time"

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

// SsoUserinfo UniSSO userinfo 端点返回（仅取需要的字段）。
type SsoUserinfo struct {
	Sub               string `json:"sub"`
	PreferredUsername string `json:"preferred_username"`
	Email             string `json:"email"`
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
