package service

import (
	"context"
	"log"
	"strconv"
	"time"

	"loginTest/core"
	"loginTest/domain"
	"loginTest/util"

	"github.com/redis/go-redis/v9"
)

// presenceService 基于 Redis ZSET 的滑动窗口在线统计。
//
// 每个用户是一个 member，score 是最后一次心跳的毫秒时间戳。统计前先按窗口
// 清掉旧成员，再用 ZCARD 得到去重后的在线人数；同一用户开多个标签页也只算一次。
// 所有操作放在一个 Pipeline 里，单次心跳最多 1 次 RTT。
type presenceService struct{}

// NewPresenceService 构造在线状态服务。它只依赖全局 Redis 客户端，无需 DB。
func NewPresenceService() PresenceService {
	return &presenceService{}
}

// OnlineKeyForScene 返回某场景/内容的 ZSET key。targetID<=0 或 scene 为空时返回空串。
func OnlineKeyForScene(scene string, targetID int) string {
	return util.GetPresenceSceneKey(scene, targetID)
}

func (s *presenceService) Heartbeat(ctx context.Context, userID int, scene string, targetID int) domain.PresenceSnapshot {
	snapshot := domain.PresenceSnapshot{
		Scene:            scene,
		TargetID:         targetID,
		WindowSeconds:    int(domain.PresenceWindow / time.Second),
		HeartbeatSeconds: domain.PresenceHeartbeatHint,
	}
	if userID <= 0 || core.MyRedis == nil {
		return snapshot
	}

	// 心跳不跟随请求取消：即使客户端已经断开，这次活跃也应该被记下来。
	opCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()

	now := time.Now()
	cutoff := now.Add(-domain.PresenceWindow).UnixMilli()
	member := strconv.Itoa(userID)
	newSceneKey := OnlineKeyForScene(scene, targetID)
	currentKey := util.GetPresenceCurrentKey(userID)

	// 上一次心跳所在的场景。切换内容时必须先把它从旧内容的 ZSET 里移除，
	// 否则用户在 150 秒窗口内会被到过的每个帖子/小说同时计数（表现为所有页面人数一样）。
	prevSceneKey, _ := core.MyRedis.Get(opCtx, currentKey).Result()

	pipe := core.MyRedis.TxPipeline()
	if prevSceneKey != "" && prevSceneKey != newSceneKey {
		pipe.ZRem(opCtx, prevSceneKey, member)
	}
	addPresenceMember(pipe, opCtx, util.PrefixPresenceOnline, member, now, cutoff)
	onlineCmd := pipe.ZCard(opCtx, util.PrefixPresenceOnline)

	var viewersCmd *redis.IntCmd
	if newSceneKey != "" {
		addPresenceMember(pipe, opCtx, newSceneKey, member, now, cutoff)
		viewersCmd = pipe.ZCard(opCtx, newSceneKey)
		pipe.Set(opCtx, currentKey, newSceneKey, domain.PresenceKeyTTL)
	} else {
		// 回到首页/搜索等无具体内容的页面：清掉「当前场景」标记。
		pipe.Del(opCtx, currentKey)
	}

	if _, err := pipe.Exec(opCtx); err != nil {
		// 在线人数是弱功能：Redis 抖动时返回 0，不影响页面本身。
		log.Printf("[presence] heartbeat failed user=%d scene=%s target=%d: %v", userID, scene, targetID, err)
		return snapshot
	}
	if onlineCmd != nil {
		snapshot.Online = int(onlineCmd.Val())
	}
	if viewersCmd != nil {
		snapshot.Viewers = int(viewersCmd.Val())
	}
	return snapshot
}

func (s *presenceService) Snapshot(ctx context.Context, scene string, targetID int) domain.PresenceSnapshot {
	snapshot := domain.PresenceSnapshot{
		Scene:            scene,
		TargetID:         targetID,
		WindowSeconds:    int(domain.PresenceWindow / time.Second),
		HeartbeatSeconds: domain.PresenceHeartbeatHint,
	}
	if core.MyRedis == nil {
		return snapshot
	}

	opCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()

	cutoff := strconv.FormatInt(time.Now().Add(-domain.PresenceWindow).UnixMilli(), 10)
	pipe := core.MyRedis.Pipeline()
	pipe.ZRemRangeByScore(opCtx, util.PrefixPresenceOnline, "-inf", cutoff)
	onlineCmd := pipe.ZCard(opCtx, util.PrefixPresenceOnline)

	var viewersCmd *redis.IntCmd
	if key := OnlineKeyForScene(scene, targetID); key != "" {
		pipe.ZRemRangeByScore(opCtx, key, "-inf", cutoff)
		viewersCmd = pipe.ZCard(opCtx, key)
	}

	if _, err := pipe.Exec(opCtx); err != nil {
		log.Printf("[presence] snapshot failed scene=%s target=%d: %v", scene, targetID, err)
		return snapshot
	}
	if onlineCmd != nil {
		snapshot.Online = int(onlineCmd.Val())
	}
	if viewersCmd != nil {
		snapshot.Viewers = int(viewersCmd.Val())
	}
	return snapshot
}

// addPresenceMember 写入/刷新成员，并顺手清掉窗口外的旧成员、续上 key 的 TTL。
func addPresenceMember(pipe redis.Pipeliner, ctx context.Context, key, member string, now time.Time, cutoff int64) {
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(now.UnixMilli()), Member: member})
	pipe.ZRemRangeByScore(ctx, key, "-inf", strconv.FormatInt(cutoff, 10))
	pipe.Expire(ctx, key, domain.PresenceKeyTTL)
}
