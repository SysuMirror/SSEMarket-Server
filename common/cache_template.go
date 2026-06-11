package common

import (
	"context"
	"loginTest/cache"
	"loginTest/util"
	"time"
)

var ctx = context.Background()

// MultiLevelGet 多级缓存统一读取（委托 CacheManager）。
func MultiLevelGet[T any](key string, dbFunc func() (T, error), expire time.Duration) (T, error) {
	profile := cache.Profile{
		Name:  "legacy",
		L1TTL: 5 * time.Minute,
		L2TTL: expire,
	}
	return cache.GetAside(ctx, key, dbFunc, profile)
}

// DelMultiLevelCache 删除两级缓存（写接口调用）。
func DelMultiLevelCache(keys ...string) {
	cache.Invalidate(keys...)
}

// InvalidatePostDetailCache 帖子详情变更时失效两级缓存。
func InvalidatePostDetailCache(postID int) {
	if postID <= 0 {
		return
	}
	cache.Invalidate(util.GetPostDetailKey(postID))
}

// InvalidateCommentListCache 评论列表变更时失效缓存。
func InvalidateCommentListCache(postID int, postType string) {
	if postID <= 0 {
		return
	}
	cache.Invalidate(util.GetCommentListKey(postID, postType))
}

// InvalidateHomeBrowseCache 首页列表变更时失效缓存。
func InvalidateHomeBrowseCache() {
	cache.Invalidate(util.GetHomeBrowsePage1Key())
}

// InvalidateUserInfoCache 用户信息变更时失效缓存。
func InvalidateUserInfoCache(userID int) {
	if userID <= 0 {
		return
	}
	cache.Invalidate(util.GetUserInfoKey(userID))
}
