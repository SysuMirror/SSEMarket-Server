package model

import "time"

// EmojiPack 表情包组。内置组（小汐/SSE娘）ownerUserID=0；
// 每个用户有一个 isPersonal=true 的「我的表情」组（默认空，长按添加的表情进这个组）；
// 用户自建组可分享（isPublic）。
type EmojiPack struct {
	PackID       int       `gorm:"primary_key;column:packID" json:"packID"`
	Title        string    `gorm:"column:title;type:varchar(100);not null" json:"title"`
	OwnerUserID  int       `gorm:"column:ownerUserID;type:int;not null;default:0;index:idx_emoji_pack_owner" json:"ownerUserID"`
	IsPersonal   bool      `gorm:"column:isPersonal;type:bool;not null;default:false" json:"isPersonal"`
	IsPublic     bool      `gorm:"column:isPublic;type:bool;not null;default:false" json:"isPublic"`
	CoverEmojiID int       `gorm:"column:coverEmojiID;type:int;not null;default:0" json:"coverEmojiID"`
	IsDeleted    bool      `gorm:"column:isDeleted;type:bool;not null;default:false" json:"isDeleted"`
	CreatedAt    time.Time `gorm:"column:createdAt;type:datetime" json:"createdAt"`
}

// Emoji 单个表情。内置表情的 url 存裸文件名（前端从内置资源解析）；
// 用户上传的表情 url 是图床绝对地址。删除走软删除：行保留，
// 历史聊天正文里的 [emoji:<id>] token 之后仍能解析渲染。
type Emoji struct {
	EmojiID     int       `gorm:"primary_key;column:emojiID" json:"emojiID"`
	PackID      int       `gorm:"column:packID;type:int;not null;default:0;index:idx_emoji_pack" json:"packID"`
	Name        string    `gorm:"column:name;type:varchar(100);not null" json:"name"`
	URL         string    `gorm:"column:url;type:varchar(500);not null" json:"url"`
	IsBuiltin   bool      `gorm:"column:isBuiltin;type:bool;not null;default:false;index:idx_emoji_builtin" json:"isBuiltin"`
	OwnerUserID int       `gorm:"column:ownerUserID;type:int;not null;default:0;index:idx_emoji_owner" json:"ownerUserID"`
	IsDeleted   bool      `gorm:"column:isDeleted;type:bool;not null;default:false" json:"isDeleted"`
	CreatedAt   time.Time `gorm:"column:createdAt;type:datetime" json:"createdAt"`
}

// UserEmoji 个人表情收藏（面板里的「我的表情」页签）。
// 长按别人发出的表情添加时走「复制一份归自己」，所以收藏行永远指向
// 内置表情或自己的表情，不会被原作者删除拖垮。
type UserEmoji struct {
	FavoriteID int       `gorm:"primary_key;column:favoriteID" json:"favoriteID"`
	UserID     int       `gorm:"column:userID;type:int;not null;uniqueIndex:uk_user_favorite_emoji" json:"userID"`
	EmojiID    int       `gorm:"column:emojiID;type:int;not null;uniqueIndex:uk_user_favorite_emoji" json:"emojiID"`
	CreatedAt  time.Time `gorm:"column:createdAt;type:datetime" json:"createdAt"`
}

// UserEmojiPack 用户添加到表情面板的表情包组（内置组不需要这行，始终展示）。
type UserEmojiPack struct {
	ID        int       `gorm:"primary_key;column:id" json:"id"`
	UserID    int       `gorm:"column:userID;type:int;not null;uniqueIndex:uk_user_emoji_pack" json:"userID"`
	PackID    int       `gorm:"column:packID;type:int;not null;uniqueIndex:uk_user_emoji_pack" json:"packID"`
	CreatedAt time.Time `gorm:"column:createdAt;type:datetime" json:"createdAt"`
}
