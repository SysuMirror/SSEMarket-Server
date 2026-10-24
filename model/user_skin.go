package model

import "time"

// UserSkin 用户自定义皮肤（桌面/手机两张图床图片）。
//
// 上传默认私密；作者公开后才会出现在搜索/分享结果里。
// share_token 是稳定的分享标识，链接形如 /new/?addSkin=<token>。
type UserSkin struct {
	SkinID      int    `gorm:"primaryKey;autoIncrement;column:skin_id"`
	OwnerID     int    `gorm:"column:owner_id;type:int;not null;index:idx_user_skin_owner"`
	Name        string `gorm:"column:name;type:varchar(50);not null"`
	Description string `gorm:"column:description;type:varchar(255);not null;default:''"`

	DesktopURL string `gorm:"column:desktop_url;type:varchar(1024);not null;default:''"`
	MobileURL  string `gorm:"column:mobile_url;type:varchar(1024);not null;default:''"`

	IsPublic   bool      `gorm:"column:is_public;type:tinyint(1);not null;default:0;index:idx_user_skin_public"`
	ShareToken string    `gorm:"column:share_token;type:varchar(64);not null;uniqueIndex:uk_user_skin_share_token"`
	CreatedAt  time.Time `gorm:"column:created_at;type:datetime"`
	UpdatedAt  time.Time `gorm:"column:updated_at;type:datetime"`
}

func (UserSkin) TableName() string { return "user_skins" }

// UserSkinLibrary 用户的皮肤库：自己上传的皮肤 + 通过搜索/分享链接添加的皮肤。
// 删除皮肤时会级联清理；主动移除只删本表记录，不影响作者原皮肤。
type UserSkinLibrary struct {
	ID      int       `gorm:"primaryKey;autoIncrement;column:id"`
	UserID  int       `gorm:"column:user_id;type:int;not null;uniqueIndex:uk_user_skin_library,priority:1;index:idx_user_skin_library_user"`
	SkinID  int       `gorm:"column:skin_id;type:int;not null;uniqueIndex:uk_user_skin_library,priority:2;index:idx_user_skin_library_skin"`
	AddedAt time.Time `gorm:"column:added_at;type:datetime"`
}

func (UserSkinLibrary) TableName() string { return "user_skin_library" }
