package cache

import (
	"context"

	"loginTest/core"
	"loginTest/util"
)

// PublicFeedGeneration 读取当前公共 Feed 缓存世代号（Redis GET）。
// 未初始化或 Redis 不可用时返回 0，此时 key 为 g0。
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

// InvalidatePublicFeed 使所有公共 Feed 列表缓存失效。
// 实现：Redis INCR 世代号 + 删除旧版 home:page1 key；新请求自动使用 g{n+1} 新 key。
func InvalidatePublicFeed(ctx context.Context) {
	if core.MyRedis != nil {
		_ = core.MyRedis.Incr(ctx, util.PublicFeedGenKey).Err()
	}
	// 兼容旧 key
	Invalidate(util.GetHomeBrowsePage1Key())
}
