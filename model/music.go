package model

import (
	"time"
)

// Music 音乐表：音频与封面文件存本地 bind-mount 目录（宿主 /root/SSE_Market/sse_market_server_resource/music/）
// FileName/CoverName 只存相对 key，出口统一走 /api/music/file|cover/:musicID，便于日后迁移对象存储
type Music struct {
	MusicID     int       `gorm:"primary_key;column:musicID"`
	UserID      int       `gorm:"index:musicuser;column:userID;type:int"`
	User        User      `gorm:"association_foreignkey:userID;foreignkey:userID"`
	Title       string    `gorm:"column:title;type:varchar(100)"`
	Artist      string    `gorm:"column:artist;type:varchar(100)"`
	Album       string    `gorm:"column:album;type:varchar(100)"`
	FileName    string    `gorm:"column:file_name;type:varchar(255)"`
	CoverName   string    `gorm:"column:cover_name;type:varchar(255)"`
	Format      string    `gorm:"column:format;type:varchar(10)"`
	SizeBytes   int64     `gorm:"column:size_bytes;type:bigint"`
	DurationSec int       `gorm:"column:duration_sec;type:int"`
	PlayCount   int       `gorm:"column:play_count;type:int"`
	IsPublished bool      `gorm:"column:is_published;type:bool"`
	IsDelete    bool      `gorm:"column:is_delete;type:bool"`
	PostTime    time.Time `gorm:"column:post_time;type:datetime"`
}
