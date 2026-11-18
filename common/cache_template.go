package common

import (
	"context"
	"loginTest/cache"
	"loginTest/util"
)

var ctx = context.Background()

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

// InvalidateHomeBrowseCache 帖子列表公共段变更时调用（发帖/删帖/改隐私等）。
// 内部通过世代号 INCR 批量失效，见 cache.InvalidatePublicFeed。
func InvalidateHomeBrowseCache() {
	cache.InvalidatePublicFeed(ctx)
}

// InvalidateUserInfoCache 用户信息变更时失效缓存。
func InvalidateUserInfoCache(userID int) {
	if userID <= 0 {
		return
	}
	cache.Invalidate(util.GetUserInfoKey(userID))
}
