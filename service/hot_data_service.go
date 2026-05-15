package service

import (
	"context"
	"loginTest/core"
	"loginTest/util"
	"sync"
	"time"

	"github.com/spf13/viper"
)

// 全局单例
var HotData *HotDataService
var ctx = context.Background()

// HotDataService 仅做浏览数计数，不操作DB，不依赖model
type HotDataService struct {
	mergeWindow  time.Duration
	syncInterval time.Duration
	browseDelta  sync.Map
	degrageStore sync.Map
	redisHealthy bool
	stopChan     chan struct{}
}

// InitHotDataService 初始化
func InitHotDataService() {
	mergeWindow := viper.GetDuration("hot_data.merge_window")
	if mergeWindow == 0 {
		mergeWindow = 1 * time.Second
	}

	syncInterval := viper.GetDuration("hot_data.sync_interval")
	if syncInterval == 0 {
		syncInterval = 1 * time.Second
	}

	HotData = &HotDataService{
		mergeWindow:  mergeWindow,
		syncInterval: syncInterval,
		redisHealthy: true,
		stopChan:     make(chan struct{}),
	}

	go HotData.startSyncTask()
	go HotData.startHealthCheck()
}

// IncrBrowseCount 浏览数+1（本地聚合）
func (s *HotDataService) IncrBrowseCount(postID int) int64 {
	delta := int64(1)
	if val, ok := s.browseDelta.Load(postID); ok {
		delta = val.(int64) + 1
	}
	s.browseDelta.Store(postID, delta)

	key := util.GetPostBrowseCountKey(postID)
	var base int64 = 0

	if s.redisHealthy {
		val, _ := core.MyRedis.Get(ctx, key).Int64()
		base = val
	} else {
		if v, ok := s.degrageStore.Load(key); ok {
			base = v.(int64)
		}
	}

	return base + delta
}

// 批量同步到Redis
func (s *HotDataService) startSyncTask() {
	ticker := time.NewTicker(s.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.syncDelta()
		case <-s.stopChan:
			s.syncDelta()
			return
		}
	}
}

func (s *HotDataService) syncDelta() {
	if !s.redisHealthy {
		return
	}

	s.browseDelta.Range(func(k, v interface{}) bool {
		postID := k.(int)
		delta := v.(int64)
		if delta <= 0 {
			return true
		}

		key := util.GetPostBrowseCountKey(postID)
		_, err := core.MyRedis.IncrBy(ctx, key, delta).Result()
		if err == nil {
			s.browseDelta.Store(postID, int64(0))
		}
		return true
	})
}

// Redis健康检查
func (s *HotDataService) startHealthCheck() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			_, err := core.MyRedis.Ping(ctx).Result()
			if err != nil {
				if s.redisHealthy {
					s.redisHealthy = false
				}
			} else {
				if !s.redisHealthy {
					s.redisHealthy = true
				}
			}
		case <-s.stopChan:
			return
		}
	}
}

// Shutdown 关闭
func (s *HotDataService) Shutdown() {
	close(s.stopChan)
}
