package cache

import (
	"context"
	"encoding/json"
	"log"

	"loginTest/core"
	"loginTest/repository"
	"loginTest/util"

	"github.com/redis/go-redis/v9"
)

const hotPostsZSetKey = "hot_posts_zset"

// WarmupDeps 预热任务依赖。
type WarmupDeps struct {
	Posts       repository.PostRepository
	Users       repository.UserRepository
	HotData     repository.HotDataRepository
	LoadBloom   func(ctx context.Context) error
	LoadHotZSet func(ctx context.Context) error
}

// BuildWarmupTasks 构建默认预热任务列表（P0 优先）。
func BuildWarmupTasks(deps WarmupDeps) []WarmupTask {
	tasks := []WarmupTask{
		{Name: "hot_posts_top100_cache", Priority: 0, Load: taskHotPostsTop100Cache},
		// public_feed_pages 已停用：Browse 列表当前直查 DB，不再预热公共 Feed 缓存。
		{Name: "browse_count_batch", Priority: 0, Load: taskBrowseCountBatch(deps.HotData)},
		{Name: "active_users_top50", Priority: 1, Load: taskActiveUsersTop50(deps.Users)},
	}
	if deps.LoadHotZSet != nil {
		tasks = append([]WarmupTask{{Name: "hot_posts_zset", Priority: 0, Load: deps.LoadHotZSet}}, tasks...)
	}
	if deps.LoadBloom != nil {
		tasks = append([]WarmupTask{{Name: "bloom_filter", Priority: 0, Load: deps.LoadBloom}}, tasks...)
	}
	return tasks
}

func taskHotPostsTop100Cache(ctx context.Context) error {
	members, err := core.MyRedis.ZRevRangeWithScores(ctx, hotPostsZSetKey, 0, 99).Result()
	if err != nil {
		return err
	}
	type hotEntry struct {
		PostID    uint    `json:"postID"`
		Title     string  `json:"title"`
		BrowseNum float64 `json:"browseNum"`
		Score     float64 `json:"score"`
	}
	out := make([]hotEntry, 0, len(members))
	for _, z := range members {
		var info hotEntry
		member, ok := z.Member.(string)
		if !ok {
			continue
		}
		if err := json.Unmarshal([]byte(member), &info); err != nil {
			continue
		}
		info.Score = z.Score
		out = append(out, info)
	}
	return Set(ctx, util.GetHotPostKey(), out, ProfileHotPostList)
}

func taskBrowseCountBatch(hotData repository.HotDataRepository) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		rows, err := hotData.ListBrowseNumsForWarmup(ctx, 500)
		if err != nil {
			return err
		}
		pipe := core.MyRedis.Pipeline()
		seeded := 0
		for _, row := range rows {
			key := util.GetPostBrowseCountKey(row.PostID)
			pipe.SetNX(ctx, key, row.BrowseNum, 0)
			seeded++
		}
		cmds, err := pipe.Exec(ctx)
		if err != nil && err != redis.Nil {
			return err
		}
		created := 0
		for _, cmd := range cmds {
			if sc, ok := cmd.(*redis.BoolCmd); ok && sc.Val() {
				created++
			}
		}
		log.Printf("[cache] browse count batch seeded=%d created=%d", seeded, created)
		return nil
	}
}

func taskActiveUsersTop50(users repository.UserRepository) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		list, err := users.ListTopActive(ctx, 50)
		if err != nil {
			return err
		}
		for _, u := range list {
			if u == nil || u.ID <= 0 {
				continue
			}
			key := util.GetUserInfoKey(u.ID)
			if err := Set(ctx, key, u, ProfileUserInfo); err != nil {
				log.Printf("[cache] warmup user profile failed userID=%d: %v", u.ID, err)
			}
		}
		return nil
	}
}

// SeedPostDetailFromDB 预热单帖详情到 L2（可选，供热点帖主动预热）。
func SeedPostDetailFromDB(ctx context.Context, posts repository.PostRepository, postID int) error {
	if postID <= 0 {
		return nil
	}
	dto, err := posts.LoadDetailDTO(ctx, postID)
	if err != nil {
		return err
	}
	return Set(ctx, util.GetPostDetailKey(postID), dto, ProfilePostDetail)
}

// WarmupPostDetailBatch 批量预热帖子详情（Top N 热帖）。
func WarmupPostDetailBatch(ctx context.Context, posts repository.PostRepository, postIDs []int) {
	for _, id := range postIDs {
		if err := SeedPostDetailFromDB(ctx, posts, id); err != nil {
			log.Printf("[cache] post detail warmup postID=%d err=%v", id, err)
		}
	}
}

// ExtractTopPostIDsFromZSet 从热度 ZSet 提取 postID 列表。
func ExtractTopPostIDsFromZSet(ctx context.Context, n int64) []int {
	members, err := core.MyRedis.ZRevRange(ctx, hotPostsZSetKey, 0, n-1).Result()
	if err != nil {
		return nil
	}
	ids := make([]int, 0, len(members))
	for _, m := range members {
		var info struct {
			PostID uint `json:"postID"`
		}
		if err := json.Unmarshal([]byte(m), &info); err != nil {
			continue
		}
		ids = append(ids, int(info.PostID))
	}
	return ids
}
