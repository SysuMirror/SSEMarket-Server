package service

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"loginTest/core"
)

// 推荐流曝光记忆：只要帖子出现在首页推荐页里（不要求点进详情），
// 就记一次曝光。下次刷新时按「次数 + 最近一次时间」轻微降权，
// 让没被点开的帖子往后挪，首页自然轮换，而不是每次刷都一模一样。
//
// 曝光与已读是两回事：
//   - 曝光 = 在 feed 里看到卡片，扣分较轻；
//   - 已读 = 点进详情（post_read_events），扣分更重。
const (
	recImpressionTTL           = 7 * 24 * time.Hour
	recImpressionHalfLifeHours = 48.0
	recImpressionBasePenalty   = 0.20
	recImpressionStepPenalty   = 0.05
	recImpressionMaxPenalty    = 0.50
	recImpressionMaxPerUser    = 1000
)

type recImpression struct {
	count  int
	lastAt time.Time
}

func recImpressionTSKey(userID int) string {
	return fmt.Sprintf("rec:imp:v2:%d", userID)
}

func recImpressionCountKey(userID int) string {
	return fmt.Sprintf("rec:impc:v2:%d", userID)
}

// RecordFeedImpressions 记录本次推荐页实际下发的帖子。写 Redis 是同步的，
// 保证用户马上刷新时能立刻看到新排序；Redis 不可用时退回进程内记忆。
func (s *recommendationService) RecordFeedImpressions(ctx context.Context, userID int, postIDs []int) {
	if userID <= 0 || len(postIDs) == 0 {
		return
	}
	seen := make(map[int]struct{}, len(postIDs))
	now := time.Now()
	for _, id := range postIDs {
		if id > 0 {
			seen[id] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return
	}

	if core.MyRedis != nil {
		tsKey := recImpressionTSKey(userID)
		countKey := recImpressionCountKey(userID)
		pipe := core.MyRedis.Pipeline()
		pipe.ZRemRangeByScore(ctx, tsKey, "-inf", strconv.FormatInt(now.Add(-recImpressionTTL).Unix(), 10))
		// 每人只保留最近 1000 条曝光，避免长期刷 feed 的账号把 ZSET 撑大。
		pipe.ZRemRangeByRank(ctx, tsKey, 0, -recImpressionMaxPerUser-1)
		for id := range seen {
			pipe.ZAdd(ctx, tsKey, redis.Z{Score: float64(now.Unix()), Member: id})
			pipe.HIncrBy(ctx, countKey, strconv.Itoa(id), 1)
		}
		pipe.Expire(ctx, tsKey, recImpressionTTL)
		pipe.Expire(ctx, countKey, recImpressionTTL)
		if _, err := pipe.Exec(ctx); err == nil {
			return
		} else {
			logRecf("feed impression write failed user=%d: %v", userID, err)
		}
	}
	s.recordImpressionMemory(userID, seen, now)
}

func (s *recommendationService) recordImpressionMemory(userID int, seen map[int]struct{}, now time.Time) {
	s.impMu.Lock()
	defer s.impMu.Unlock()
	if len(s.impMemory) >= 4096 {
		s.impMemory = map[int]map[int]recImpression{}
	}
	m := s.impMemory[userID]
	if m == nil {
		m = make(map[int]recImpression, len(seen))
		s.impMemory[userID] = m
	}
	for id := range seen {
		imp := m[id]
		imp.count++
		imp.lastAt = now
		m[id] = imp
	}
}

func (s *recommendationService) impressionsFor(ctx context.Context, userID int) map[int]recImpression {
	if core.MyRedis != nil {
		tsKey := recImpressionTSKey(userID)
		countKey := recImpressionCountKey(userID)
		pipe := core.MyRedis.Pipeline()
		zs := pipe.ZRangeWithScores(ctx, tsKey, 0, -1)
		hs := pipe.HGetAll(ctx, countKey)
		if _, err := pipe.Exec(ctx); err == nil {
			out := make(map[int]recImpression, len(zs.Val()))
			counts := hs.Val()
			for _, z := range zs.Val() {
				id, err := strconv.Atoi(fmt.Sprint(z.Member))
				if err != nil || id <= 0 {
					continue
				}
				imp := recImpression{count: 1, lastAt: time.Unix(int64(z.Score), 0)}
				if n, err := strconv.Atoi(counts[strconv.Itoa(id)]); err == nil && n > 0 {
					imp.count = n
				}
				out[id] = imp
			}
			return out
		}
	}

	s.impMu.Lock()
	defer s.impMu.Unlock()
	m := s.impMemory[userID]
	out := make(map[int]recImpression, len(m))
	for id, imp := range m {
		if time.Since(imp.lastAt) <= recImpressionTTL {
			out[id] = imp
		}
	}
	return out
}

// impressionPenalty 只在近期曝光过时生效；48 小时半衰，避免永久埋没优质老帖。
func impressionPenalty(imp recImpression, now time.Time) float64 {
	if imp.count <= 0 || imp.lastAt.IsZero() {
		return 0
	}
	ageHours := now.Sub(imp.lastAt).Hours()
	if ageHours < 0 {
		ageHours = 0
	}
	decay := math.Pow(0.5, ageHours/recImpressionHalfLifeHours)
	penalty := recImpressionBasePenalty + recImpressionStepPenalty*float64(minInt(imp.count-1, 6))
	if penalty > recImpressionMaxPenalty {
		penalty = recImpressionMaxPenalty
	}
	return penalty * decay
}
