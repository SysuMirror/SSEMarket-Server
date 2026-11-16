package domain

import "time"

// Admin 管理员实体（管理台白名单条目）。
type Admin struct {
	ID          int
	Account     string
	SsoSub      string
	Email       string
	Name        string
	IsActive    bool
	CreatedAt   time.Time
	LastLoginAt time.Time
}

// AdminUserFilter 管理端用户筛选。
type AdminUserFilter struct {
	Name   string
	Phone  string
	IDPass int
}

// AdminUserItem 管理端用户列表项。
type AdminUserItem struct {
	Phone     string
	Email     string
	Name      string
	PunishNum int
}

// AdminPostInput 管理员发帖入参。
type AdminPostInput struct {
	Title     string
	Content   string
	Partition string
	Photos    string
	Tags      string
}
