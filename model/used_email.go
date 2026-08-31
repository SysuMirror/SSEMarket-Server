package model

import "time"

// UsedEmail 记录用户曾经使用过的邮箱。
// 历史邮箱仅用于追溯，本阶段不改变其重新注册或登录策略。
type UsedEmail struct {
	UsedEmailID int       `gorm:"primaryKey;column:usedEmailID"`
	UserID      int       `gorm:"column:userID;type:int;not null;index"`
	Email       string    `gorm:"column:email;type:varchar(255);not null;index"`
	Created     time.Time `gorm:"column:created;type:datetime;not null"`
}

func (UsedEmail) TableName() string { return "used_emails" }
