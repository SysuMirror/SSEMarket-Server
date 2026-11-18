package service

import (
	"context"
	"errors"
	"log"
	"loginTest/core"
	"loginTest/observability"
	"loginTest/repository"
	"loginTest/util"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
)

// 全局单例
var HotData *HotDataService
var ctx = context.Background()

// HotDataService 浏览数计数服务：Redis INCR 为跨实例唯一真相，周期同步到 MySQL。
type HotDataService struct {
	repo            repository.HotDataRepository
	dbSyncInterval  time.Duration
	browseDirty     sync.Map // postID -> time.Time，该帖 Redis 计数有变更、待落库的时间
	redisHealthy    bool
	stopChan        chan struct{}
	shutdownOnce    sync.Once
	wg              sync.WaitGroup
	lastSyncedPosts map[int]int64 // 上次已成功写入 DB 的浏览数，避免重复 UPDATE
	lastSyncedMutex sync.Mutex
}

// InitHotDataService 初始化
func InitHotDataService(repo repository.HotDataRepository) {
	dbSyncInterval := viper.GetDuration("hot_data.db_sync_interval")
	if dbSyncInterval == 0 {
		dbSyncInterval = 5 * time.Second
	}

	HotData = &HotDataService{
		repo:            repo,
		dbSyncInterval:  dbSyncInterval,
		redisHealthy:    true,
		stopChan:        make(chan struct{}),
		lastSyncedPosts: make(map[int]int64),
	}

	HotData.wg.Add(2)
	go func() {
		defer HotData.wg.Done()
		HotData.startHealthCheck()
	}()
	go func() {
		defer HotData.wg.Done()
		HotData.startDBSyncTask()
	}()
}

func (s *HotDataService) browseNumFromDB(postID int) int64 {
	return s.repo.GetBrowseNum(context.Background(), postID)
}

// ensureBrowseCountKeySeeded 在 Redis 中不存在计数 key 时用 DB 的 browse_num 做 SetNX，避免 Incr 从 0 起算导致与 MySQL 分叉。
func (s *HotDataService) ensureBrowseCountKeySeeded(postID int) {
	if !s.redisHealthy {
		return
	}
	key := util.GetPostBrowseCountKey(postID)
	_, err := core.MyRedis.Get(ctx, key).Result()
	if err == nil {
		return
	}
	if !errors.Is(err, redis.Nil) {
		return
	}
	n := s.browseNumFromDB(postID)
	ok, errNX := core.MyRedis.SetNX(ctx, key, n, 0).Result()
	if errNX == nil && ok {
		log.Printf("[hot_data] browse count key missing, seeded from DB postID=%d val=%d", postID, n)
	}
}

// IncrBrowseCount 浏览数 +1：直接 Redis INCR，多副本下与 MySQL 最终由 syncToDB 对齐。
func (s *HotDataService) IncrBrowseCount(postID int) int64 {
	if !s.redisHealthy {
		return s.browseNumFromDB(postID)
	}
	s.ensureBrowseCountKeySeeded(postID)
	key := util.GetPostBrowseCountKey(postID)
	n, err := core.MyRedis.Incr(ctx, key).Result()
	if err != nil {
		log.Printf("[hot_data] IncrBrowseCount redis Incr failed postID=%d: %v", postID, err)
		return s.browseNumFromDB(postID)
	}
	s.browseDirty.Store(postID, time.Now())
	return n
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

// startDBSyncTask 启动数据库同步任务
func (s *HotDataService) startDBSyncTask() {
	ticker := time.NewTicker(s.dbSyncInterval)
	lagTicker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	defer lagTicker.Stop()

	for {
		select {
		case <-ticker.C:
			s.syncToDB()
		case <-lagTicker.C:
			s.updateBrowseSyncLag()
		case <-s.stopChan:
			s.syncToDB()
			s.updateBrowseSyncLag()
			return
		}
	}
}

// updateBrowseSyncLag 计算最久未同步的 dirty 浏览数延迟。
func (s *HotDataService) updateBrowseSyncLag() {
	var maxLag time.Duration
	now := time.Now()
	s.browseDirty.Range(func(k, v interface{}) bool {
		dirtyAt, ok := v.(time.Time)
		if !ok {
			return true
		}
		if lag := now.Sub(dirtyAt); lag > maxLag {
			maxLag = lag
		}
		return true
	})
	observability.BrowseSyncLag.Set(maxLag.Seconds())
}

// syncToDB 将 Redis 中的浏览数同步到数据库
func (s *HotDataService) syncToDB() {
	if !s.redisHealthy {
		return
	}

	postIDs := make([]int, 0, 64)
	s.browseDirty.Range(func(k, v interface{}) bool {
		postIDs = append(postIDs, k.(int))
		return true
	})
	if len(postIDs) == 0 {
		return
	}

	s.lastSyncedMutex.Lock()
	defer s.lastSyncedMutex.Unlock()

	for _, postID := range postIDs {
		key := util.GetPostBrowseCountKey(postID)
		s.ensureBrowseCountKeySeeded(postID)
		redisBrowseNum, err := core.MyRedis.Get(ctx, key).Int64()
		if err != nil {
			continue
		}

		lastSynced, exists := s.lastSyncedPosts[postID]
		if exists && lastSynced == redisBrowseNum {
			s.browseDirty.Delete(postID)
			continue
		}

		err = s.repo.UpdateBrowseNum(context.Background(), postID, redisBrowseNum)
		if err != nil {
			continue
		}
		s.lastSyncedPosts[postID] = redisBrowseNum
		s.browseDirty.Delete(postID)
	}
}

// Shutdown 停止后台任务并 flush Redis 浏览数到 MySQL。
func (s *HotDataService) Shutdown() {
	s.shutdownOnce.Do(func() {
		close(s.stopChan)
	})
	s.wg.Wait()
}
