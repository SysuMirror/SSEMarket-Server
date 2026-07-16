package model

import "time"

// UserAPIToken 用户个人访问令牌（PAT），用于 MCP 等对外调用场景。
// 明文 token 仅在创建时返回一次，DB 只存 SHA-256 hash。
type UserAPIToken struct {
	TokenID     int       `gorm:"primary_key;column:token_id;autoIncrement"`
	UserID      int       `gorm:"column:user_id;type:int;not null;index"`
	TokenHash   string    `gorm:"column:token_hash;type:varchar(64);not null;uniqueIndex"`
	TokenPrefix string    `gorm:"column:token_prefix;type:varchar(16);not null"` // 明文前缀，用于前端展示识别
	Name        string    `gorm:"column:name;type:varchar(64);not null"`         // 用户起的名字，如 "Claude Desktop"
	Scopes      string    `gorm:"column:scopes;type:varchar(255);not null"`      // 逗号分隔，如 "agent:chat,forum:read,forum:write"
	LastUsedAt  *time.Time `gorm:"column:last_used_at;type:datetime"`             // 智能体每次验证时回写；nullable
	IsActive    bool      `gorm:"column:is_active;type:tinyint(1);not null;default:1"`
	CreatedAt   time.Time `gorm:"column:created_at;type:datetime;not null"`
	UpdatedAt   time.Time `gorm:"column:updated_at;type:datetime;not null"`
}

func (UserAPIToken) TableName() string {
	return "user_api_tokens"
}
