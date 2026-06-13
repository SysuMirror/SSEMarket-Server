package cache

import (
	"context"

	"loginTest/core"
	"loginTest/util"
)

// PublicFeedGeneration 当前公共 Feed 缓存世代号（失效时 INCR）。
func PublicFeedGeneration(ctx context.Context) int64 {
	if core.MyRedis == nil {
		return 0
	}
	n, err := core.MyRedis.Get(ctx, util.PublicFeedGenKey).Int64()
	if err != nil {
		return 0
	}
	return n
}

// InvalidatePublicFeed 使所有公共 Feed 缓存失效（世代号 +1，旧 key 自然过期）。
func InvalidatePublicFeed(ctx context.Context) {
	if core.MyRedis != nil {
		_ = core.MyRedis.Incr(ctx, util.PublicFeedGenKey).Err()
	}
	// 兼容旧 key
	Invalidate(util.GetHomeBrowsePage1Key())
}
