package service

import (
	"context"
	"time"

	"loginTest/common"
)

// MultiLevelGet 封装 L1→L2→DB 多级缓存，供 Service 层统一使用。
func MultiLevelGet[T any](ctx context.Context, key string, loader func() (T, error), expire time.Duration) (T, error) {
	_ = ctx // loader 内部应使用 ctx；common 层暂用 background
	return common.MultiLevelGet(key, loader, expire)
}

// InvalidatePostDetail 帖子详情变更时失效两级缓存。
func InvalidatePostDetail(postID int) {
	common.InvalidatePostDetailCache(postID)
}

// DelMultiLevel 删除指定 key 的两级缓存。
func DelMultiLevel(keys ...string) {
	common.DelMultiLevelCache(keys...)
}
