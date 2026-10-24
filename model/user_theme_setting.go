package model

import "time"

// UserThemeSetting 用户的全站外观设置（皮肤 / 开场画面 / 背景 / 卡片透明度）。
//
// 只存「选择」，不存皮肤图片：
//   - theme_mode：auto / default / 前端注册的皮肤 ID（如 programmers-day-2026）；
//   - background_source：default（皮肤自带背景） / custom（用户上传的图床图片）；
//   - custom_background_url：图床返回的绝对 http(s) 地址，只有 custom 模式才使用；
//   - post_card_opacity：帖子卡片不透明度（30-100），只在开启全站背景时生效。
//
// 没有行 = 全部默认（auto + 开场画面开 + 背景开 + 皮肤自带背景 + 卡片 70%）。
type UserThemeSetting struct {
	UserID              int    `gorm:"primaryKey;column:user_id;type:int;not null"`
	ThemeMode           string `gorm:"column:theme_mode;type:varchar(64);not null;default:'auto'"`
	SplashEnabled       bool   `gorm:"column:splash_enabled;type:tinyint(1);not null;default:1"`
	BackgroundEnabled   bool   `gorm:"column:background_enabled;type:tinyint(1);not null;default:1"`
	BackgroundSource    string `gorm:"column:background_source;type:varchar(16);not null;default:'default'"`
	CustomBackgroundURL string `gorm:"column:custom_background_url;type:varchar(2048);not null;default:''"`
	// CustomSkinID 当前选中的「用户自定义皮肤库」皮肤；>0 时覆盖皮肤背景/单图背景。
	CustomSkinID    int       `gorm:"column:custom_skin_id;type:int;not null;default:0"`
	PostCardOpacity int       `gorm:"column:post_card_opacity;type:tinyint unsigned;not null;default:70"`
	UpdatedAt       time.Time `gorm:"column:updated_at;type:datetime"`
}

func (UserThemeSetting) TableName() string { return "user_theme_settings" }
