package repository

import (
	"context"
)

// HotDataRepository 热度数据持久化接口。
type HotDataRepository interface {
	GetBrowseNum(ctx context.Context, postID int) int64
	UpdateBrowseNum(ctx context.Context, postID int, browseNum int64) error
}
