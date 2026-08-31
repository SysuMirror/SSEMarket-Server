package model

import "time"

// WorkbenchApp 工作台站点/应用（含集市开发者平台模块经平台中转创建的站点）
type WorkbenchApp struct {
	AppID       int       `gorm:"primaryKey;autoIncrement;column:app_id"`
	Name        string    `gorm:"column:name;type:varchar(50);unique_index"`
	Icon        string    `gorm:"column:icon;type:varchar(100)"`
	TargetType  string    `gorm:"column:target_type;type:varchar(10)"` // route | link
	Target      string    `gorm:"column:target;type:varchar(500)"`
	GroupName   string    `gorm:"column:group_name;type:varchar(20)"` // common 等，预留扩展
	ColorBg     string    `gorm:"column:color_bg;type:varchar(20)"`
	ColorFg     string    `gorm:"column:color_fg;type:varchar(20)"`
	Source      string    `gorm:"column:source;type:varchar(20);default:'local'"` // local | platform
	InvokerSlug string    `gorm:"column:invoker_slug;type:varchar(100)"`          // 经平台中转时的真实调用方模块 slug
	SortOrder   int       `gorm:"column:sort_order;type:int;default:0"`
	IsVisible   bool      `gorm:"column:is_visible;type:tinyint(1);default:1"`
	CreatedAt   time.Time `gorm:"column:created_at;type:datetime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;type:datetime"`
}

func (WorkbenchApp) TableName() string {
	return "workbench_apps"
}
