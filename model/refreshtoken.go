package model

import "time"

// RefreshToken 记录用户刷新用的 refresh token
// 一次性使用，过期时间与 JWT 中的 refresh token 一致
// successor_token/used_at 用于 60 秒轮换宽限：并发刷新时后到者可凭已用记录
// 直接换发 successor，而不是 401（IETF OAuth BCP 对 rotation 的标准做法）
//
// 表结构建议：
//   CREATE TABLE `refresh_tokens` (
//     `id` int NOT NULL AUTO_INCREMENT,
//     `user_id` int NOT NULL,
//     `token` varchar(255) NOT NULL UNIQUE,
//     `expires_at` datetime NOT NULL,
//     `is_used` tinyint(1) NOT NULL DEFAULT 0,
//     `created_at` datetime NOT NULL,
//     `successor_token` varchar(255) NULL,
//     `used_at` datetime NULL,
//     PRIMARY KEY (`id`),
//     KEY `idx_refresh_tokens_user_id` (`user_id`)
//   );
// 实际建表请以数据库迁移/运维脚本为准。

type RefreshToken struct {
	ID             int        `gorm:"primary_key;column:id"`
	UserID         int        `gorm:"column:user_id;type:int"`
	Token          string     `gorm:"column:token;type:varchar(255);unique"`
	ExpiresAt      time.Time  `gorm:"column:expires_at;type:datetime"`
	IsUsed         bool       `gorm:"column:is_used;type:tinyint(1);default:0"`
	CreatedAt      time.Time  `gorm:"column:created_at;type:datetime"`
	SuccessorToken *string    `gorm:"column:successor_token;type:varchar(255)"`
	UsedAt         *time.Time `gorm:"column:used_at;type:datetime"`
}

// TableName 指定表名
func (RefreshToken) TableName() string {
	return "refresh_tokens"
}
