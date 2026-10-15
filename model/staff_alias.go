package model

// StaffAlias 开发人员匿名「职位账号」映射表（管理员手工 INSERT，用法见 docs/staff_alias.md）。
//
// 字段只有 5 个，管理员平时只碰前 3 个：
//   - alias        被 @ 的对外名字（同时也是职位主页的昵称），如「iwiki开发人员」；
//   - title        职称 / 主页称号；
//   - staff_user_id 真实开发者的 userID：只用于收 @ 通知与「以职位身份回复」的鉴权，
//     任何对外接口都不返回该字段（匿名性全靠它不外泄）。
//   - alias_user_id 由系统按本行自动派生的职位小号（users 表一行，满级 + 职称），
//     管理员插行时留 0 即可，启动同步 / 首次使用时自动回填。
type StaffAlias struct {
	ID          int    `gorm:"primary_key;column:id"`
	Alias       string `gorm:"column:alias;type:varchar(50);not null;uniqueIndex:uk_staff_alias_alias"`
	Title       string `gorm:"column:title;type:varchar(50);not null"`
	StaffUserID int    `gorm:"column:staff_user_id;type:int;not null;index:idx_staff_alias_staff"`
	AliasUserID int    `gorm:"column:alias_user_id;type:int;default:0"`
}
