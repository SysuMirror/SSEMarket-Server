package cache

import (
	"context"

	"loginTest/core"
	"loginTest/util"
)

// InvalidatePublicFeed 使所有公共 Feed 列表缓存失效。
// 实现：Redis INCR 世代号 + 删除旧版 home:page1 key；新请求自动使用 g{n+1} 新 key。
func InvalidatePublicFeed(ctx context.Context) {
	if core.MyRedis != nil {
		_ = core.MyRedis.Incr(ctx, util.PublicFeedGenKey).Err()
	}
	// 兼容旧 key
	Invalidate(util.GetHomeBrowsePage1Key())
}
