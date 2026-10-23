package model

import "time"

// ThemeEvent 一套节日皮肤的活动时间窗（一行 = 一期活动）。
//
// 由管理员直接 INSERT 维护，代码不再写死日期：
//
//	INSERT INTO theme_events (theme_id, name, starts_at, ends_at, enabled)
//	VALUES ('programmers-day-2026', '2026 程序员节',
//	        '2026-10-24 00:00:00', '2026-10-25 00:00:00', 1);
//
// 时间一律按「北京时间墙钟」填写（DATETIME，不做时区转换），
// 读取时由 domain.ThemeWallClock 解释成 +08:00，服务器部署在哪个时区都不影响。
// 同一个 theme_id 可以有多期活动（唯一索引是 theme_id + starts_at）。
type ThemeEvent struct {
	EventID   int       `gorm:"primaryKey;autoIncrement;column:event_id"`
	ThemeID   string    `gorm:"column:theme_id;type:varchar(64);not null;uniqueIndex:uk_theme_event_window,priority:1"`
	Name      string    `gorm:"column:name;type:varchar(100);not null;default:''"`
	StartsAt  time.Time `gorm:"column:starts_at;type:datetime;not null;uniqueIndex:uk_theme_event_window,priority:2;index:idx_theme_event_starts"`
	EndsAt    time.Time `gorm:"column:ends_at;type:datetime;not null;index:idx_theme_event_ends"`
	Enabled   bool      `gorm:"column:enabled;type:tinyint(1);not null;default:1"`
	CreatedAt time.Time `gorm:"column:created_at;type:datetime"`
	UpdatedAt time.Time `gorm:"column:updated_at;type:datetime"`
}

func (ThemeEvent) TableName() string { return "theme_events" }
