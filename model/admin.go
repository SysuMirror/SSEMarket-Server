package model

import "time"

// Admin 管理员白名单。UniSSO 登录通过后按 sso_sub / email 匹配本表；
// 不在表内或 is_active=0 的账号无权进入管理台。
type Admin struct {
	AdminID     int        `gorm:"primary_key;column:adminID"`
	Account     string     `gorm:"primary_key;column:account;type:varchar(100);not null"`
	SsoSub      string     `gorm:"column:sso_sub;type:varchar(64)"`
	Email       string     `gorm:"column:email;type:varchar(128)"`
	Name        string     `gorm:"column:name;type:varchar(64)"`
	IsActive    bool       `gorm:"column:is_active;default:1"`
	CreatedAt   *time.Time `gorm:"column:created_at"`
	LastLoginAt *time.Time `gorm:"column:last_login_at"`
}
