package cache

import "time"

// Profile 描述一类数据的缓存策略（Cache-Aside + 可选逻辑过期）。
type Profile struct {
	Name          string
	L1TTL         time.Duration // 逻辑过期（L1 有效窗口）
	L2TTL         time.Duration // Redis 物理 TTL（含 jitter）
	LogicalExpire bool          // true：逻辑过期后仍返回旧值并异步刷新
}

var (
	ProfilePostDetail = Profile{
		Name: "post_detail", L1TTL: 5 * time.Minute, L2TTL: 30 * time.Minute, LogicalExpire: true,
	}
	ProfileCommentList = Profile{
		Name: "comment_list", L1TTL: 3 * time.Minute, L2TTL: 15 * time.Minute,
	}
	ProfileHomeBrowse = Profile{
		Name: "home_browse", L1TTL: 1 * time.Minute, L2TTL: 5 * time.Minute,
	}
	// ProfilePublicFeed 公共帖子列表（与用户无关，browse 公共段）。
	ProfilePublicFeed = Profile{
		Name: "public_feed", L1TTL: 1 * time.Minute, L2TTL: 5 * time.Minute, LogicalExpire: true,
	}
	ProfileUserInfo = Profile{
		Name: "user_info", L1TTL: 10 * time.Minute, L2TTL: time.Hour,
	}
	ProfileHotPostList = Profile{
		Name: "hot_post_list", L1TTL: 5 * time.Minute, L2TTL: 30 * time.Minute,
	}
)
