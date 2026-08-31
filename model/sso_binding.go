package model

import "time"

// SsoBinding UniSSO 身份(sso_sub)与集市账号的绑定关系表。
type SsoBinding struct {
	ID          int       `gorm:"primary_key;column:id"`
	LocalUserID int       `gorm:"column:local_user_id;type:int;not null"`
	SsoSub      string    `gorm:"column:sso_sub;type:varchar(64);not null"`
	SsoUsername string    `gorm:"column:sso_username;type:varchar(100);not null;default:''"`
	CreatedAt   time.Time `gorm:"column:created_at;type:datetime;not null"`
}

func (SsoBinding) TableName() string { return "sso_bindings" }
