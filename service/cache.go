package service

import (
	"context"
	"time"

	"loginTest/cache"
	"loginTest/common"
)

// MultiLevelGet 封装 CacheManager Cache-Aside，供 Service 层统一使用。
func MultiLevelGet[T any](ctx context.Context, key string, loader func() (T, error), profile cache.Profile) (T, error) {
	return cache.GetAside(ctx, key, loader, profile)
}

// MultiLevelGetLegacy 兼容旧调用（按 expire 推断 L2 TTL）。
func MultiLevelGetLegacy[T any](ctx context.Context, key string, loader func() (T, error), expire time.Duration) (T, error) {
	return cache.GetAside(ctx, key, loader, cache.Profile{
		Name: "legacy", L1TTL: 5 * time.Minute, L2TTL: expire,
	})
}

// InvalidatePostDetail 帖子详情变更时失效两级缓存。
func InvalidatePostDetail(postID int) {
	common.InvalidatePostDetailCache(postID)
}

// InvalidateCommentList 评论列表变更时失效缓存。
func InvalidateCommentList(postID int, postType string) {
	common.InvalidateCommentListCache(postID, postType)
}

// InvalidateHomeBrowse 公共 Feed 列表变更时失效缓存（世代号 +1）。
func InvalidateHomeBrowse() {
	common.InvalidateHomeBrowseCache()
}

// InvalidateUserInfo 用户信息变更时失效缓存。
func InvalidateUserInfo(userID int) {
	common.InvalidateUserInfoCache(userID)
}

// DelMultiLevel 删除指定 key 的两级缓存。
func DelMultiLevel(keys ...string) {
	cache.Invalidate(keys...)
}
