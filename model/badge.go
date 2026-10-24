package model

import "time"

// Badge 勋章目录（一枚勋章一行）。
//
// 勋章图片统一放在图床，管理员在后台/脚本上传拿到绝对 http(s) 地址后，
// 直接 INSERT 一行即可，不需要改代码：
//
//	INSERT INTO badges (code, name, description, image_url, category, sort_order, enabled)
//	VALUES ('programmers-day-2027', '2027 程序员节', '2027 程序员节限定',
//	        'https://image.ssemarket.cn/i/...png', 'festival', 100, 1);
//
// code 是稳定标识：发放记录/幂等键都靠它，后续改名不影响已发出去的勋章。
type Badge struct {
	BadgeID     int    `gorm:"primaryKey;autoIncrement;column:badge_id"`
	Code        string `gorm:"column:code;type:varchar(64);not null;uniqueIndex:uk_badge_code"`
	Name        string `gorm:"column:name;type:varchar(64);not null;default:''"`
	Description string `gorm:"column:description;type:varchar(255);not null;default:''"`
	ImageURL    string `gorm:"column:image_url;type:varchar(1024);not null;default:''"`
	// Category 只用于前端分组/排序：anniversary=注册周年，festival=节日限定，special=其它。
	Category  string    `gorm:"column:category;type:varchar(32);not null;default:''"`
	SortOrder int       `gorm:"column:sort_order;type:int;not null;default:0"`
	Enabled   bool      `gorm:"column:enabled;type:tinyint(1);not null;default:1"`
	CreatedAt time.Time `gorm:"column:created_at;type:datetime"`
	UpdatedAt time.Time `gorm:"column:updated_at;type:datetime"`
}

func (Badge) TableName() string { return "badges" }

// UserBadge 发放记录（谁拥有哪枚勋章）。
//
// 批量补发直接 INSERT ... SELECT 即可，两个唯一索引负责幂等：
//   - (user_id, badge_id)：同一个人同一枚勋章只发一次；
//   - (user_id, award_key)：同一件事（如 2026 程序员节、某年某周年）只记一次。
//
// award_key 由发放脚本约定，例如 programmers-day-2026 / anniversary:2023:3。
// 手工补发时也必须填一个可读的键，不要留空（唯一索引会把空串当成重复）。
type UserBadge struct {
	ID       int    `gorm:"primaryKey;autoIncrement;column:id"`
	UserID   int    `gorm:"column:user_id;type:int;not null;uniqueIndex:uk_user_badge,priority:1;uniqueIndex:uk_user_badge_award,priority:1;index:idx_user_badge_user"`
	BadgeID  int    `gorm:"column:badge_id;type:int;not null;uniqueIndex:uk_user_badge,priority:2;index:idx_user_badge_badge"`
	AwardKey string `gorm:"column:award_key;type:varchar(128);not null;uniqueIndex:uk_user_badge_award,priority:2"`

	// Source auto=系统批量补发；manual=管理员手工补发。
	Source string `gorm:"column:source;type:varchar(16);not null;default:'auto'"`
	Note   string `gorm:"column:note;type:varchar(255);not null;default:''"`

	AwardedAt time.Time `gorm:"column:awarded_at;type:datetime;not null;index:idx_user_badge_time"`
	CreatedAt time.Time `gorm:"column:created_at;type:datetime"`
}

func (UserBadge) TableName() string { return "user_badges" }

// UserBadgePref 用户的勋章展示偏好（没有行 = 全部默认）。
//
//   - worn_badge_id：在帖子/评论等「对外」场景挂的那一枚，0 = 不挂；
//   - profile_badge_ids：个人主页勋章墙展示哪些，JSON 数组文本。
//     空字符串 = 默认全部展示；"[]" = 明确选择不展示；"[1,4]" = 只展示这两枚。
type UserBadgePref struct {
	UserID          int       `gorm:"primaryKey;column:user_id;type:int;not null"`
	WornBadgeID     int       `gorm:"column:worn_badge_id;type:int;not null;default:0"`
	ProfileBadgeIDs string    `gorm:"column:profile_badge_ids;type:text"`
	UpdatedAt       time.Time `gorm:"column:updated_at;type:datetime"`
}

func (UserBadgePref) TableName() string { return "user_badge_prefs" }
