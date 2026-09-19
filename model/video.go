package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	VideoStatusDraft     = "draft"
	VideoStatusPublished = "published"
	VideoStatusHidden    = "hidden"
)

type Video struct {
	VideoID         int64          `gorm:"primaryKey;autoIncrement;column:videoID"`
	AdminID         int            `gorm:"index:idx_videos_admin;column:adminID"`
	MediaAssetID    int64          `gorm:"column:media_asset_id;default:0"`
	Title           string         `gorm:"column:title;type:varchar(120);not null"`
	Description     string         `gorm:"column:description;type:varchar(2000);default:''"`
	CoverURL        string         `gorm:"column:cover_url;type:varchar(2048);default:''"`
	SourceType      string         `gorm:"column:source_type;type:varchar(16);not null"`
	SourceURL       string         `gorm:"column:source_url;type:varchar(2048);not null"`
	BilibiliBVID    string         `gorm:"column:bilibili_bvid;type:varchar(32);default:''"`
	ExternalPageURL string         `gorm:"column:external_page_url;type:varchar(2048);default:''"`
	Status          string         `gorm:"column:status;type:varchar(16);default:'draft';index:idx_videos_status_published_at"`
	DurationSec     int            `gorm:"column:duration_sec;type:int;default:0"`
	SortWeight      int            `gorm:"column:sort_weight;type:int;default:0"`
	ViewCount       int64          `gorm:"column:view_count;type:bigint;default:0"`
	PlayCount       int64          `gorm:"column:play_count;type:bigint;default:0"`
	PublishedAt     *time.Time     `gorm:"column:published_at;type:datetime"`
	CreatedAt       time.Time      `gorm:"column:created_at;type:datetime"`
	UpdatedAt       time.Time      `gorm:"column:updated_at;type:datetime"`
	DeletedAt       gorm.DeletedAt `gorm:"column:deleted_at;index"`

	StorageKey       string `gorm:"column:storage_key;type:varchar(512);default:''"`
	PlaybackKey      string `gorm:"column:playback_key;type:varchar(512);default:''"`
	OriginalFileName string `gorm:"column:original_file_name;type:varchar(255);default:''"`
	FileSizeBytes    int64  `gorm:"column:file_size_bytes;type:bigint;default:0"`
	Width            int    `gorm:"column:width;type:int;default:0"`
	Height           int    `gorm:"column:height;type:int;default:0"`
	TranscodeStatus  string `gorm:"column:transcode_status;type:varchar(16);default:'ready'"`
	TranscodeError   string `gorm:"column:transcode_error;type:varchar(1000);default:''"`
}

func (Video) TableName() string { return "videos" }
