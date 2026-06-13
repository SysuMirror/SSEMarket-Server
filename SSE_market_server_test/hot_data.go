package repository

import (
	"context"
)

// PostBrowseNum 帖子浏览数（预热/批量 seed 用）。
type PostBrowseNum struct {
	PostID    int
	BrowseNum int64
}

// HotDataRepository 热度数据持久化接口。
type HotDataRepository interface {
	GetBrowseNum(ctx context.Context, postID int) int64
	UpdateBrowseNum(ctx context.Context, postID int, browseNum int64) error
	// ListBrowseNumsForWarmup 按 post_time 倒序取最近帖子的 browse_num，用于 Redis 批量 SetNX 预热。
	ListBrowseNumsForWarmup(ctx context.Context, limit int) ([]PostBrowseNum, error)
}
