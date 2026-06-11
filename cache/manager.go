package cache

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"loginTest/core"
	"loginTest/util"

	"golang.org/x/sync/singleflight"
)

// Manager 统一二级缓存治理入口（Cache-Aside + 击穿/穿透/雪崩防护）。
type Manager struct {
	sf singleflight.Group
}

// Default 全局 CacheManager 实例，InitManager 后可用。
var Default *Manager

func InitManager() {
	Default = &Manager{}
}

// GetAside Cache-Aside 读取：L1 → L2 → DB，支持逻辑过期与击穿重试。
func GetAside[T any](ctx context.Context, key string, loader func() (T, error), profile Profile) (T, error) {
	if Default == nil {
		InitManager()
	}
	return getAside(Default, ctx, key, loader, profile)
}

func getAside[T any](m *Manager, ctx context.Context, key string, loader func() (T, error), profile Profile) (T, error) {
	var zero T
	if profile.L1TTL <= 0 {
		profile.L1TTL = 5 * time.Minute
	}
	if profile.L2TTL <= 0 {
		profile.L2TTL = 30 * time.Minute
	}

	if data, err := core.LocalCache.Get(key); err == nil {
		if cv, ok := decodeValue[T](data); ok {
			if isLogicValid(cv) {
				return cv.Data, nil
			}
			if profile.LogicalExpire {
				refreshAsync(m, ctx, key, loader, profile)
				return cv.Data, nil
			}
		}
	}

	if redisData, err := core.MyRedis.Get(ctx, key).Bytes(); err == nil {
		if cv, ok := decodeValue[T](redisData); ok {
			if isLogicValid(cv) {
				_ = core.LocalCache.Set(key, redisData)
				return cv.Data, nil
			}
			if profile.LogicalExpire {
				_ = core.LocalCache.Set(key, redisData)
				refreshAsync(m, ctx, key, loader, profile)
				return cv.Data, nil
			}
		}
	}

	lock := util.NewLock("lock:" + key)
	acquired := lock.TryLock()
	if !acquired {
		time.Sleep(50 * time.Millisecond)
		if data, err := core.LocalCache.Get(key); err == nil {
			if cv, ok := decodeValue[T](data); ok && (isLogicValid(cv) || profile.LogicalExpire) {
				if profile.LogicalExpire && !isLogicValid(cv) {
					refreshAsync(m, ctx, key, loader, profile)
				}
				return cv.Data, nil
			}
		}
		if redisData, err := core.MyRedis.Get(ctx, key).Bytes(); err == nil {
			if cv, ok := decodeValue[T](redisData); ok && (isLogicValid(cv) || profile.LogicalExpire) {
				_ = core.LocalCache.Set(key, redisData)
				if profile.LogicalExpire && !isLogicValid(cv) {
					refreshAsync(m, ctx, key, loader, profile)
				}
				return cv.Data, nil
			}
		}
	} else {
		defer lock.UnLock()
		if redisData, err := core.MyRedis.Get(ctx, key).Bytes(); err == nil {
			if cv, ok := decodeValue[T](redisData); ok && isLogicValid(cv) {
				_ = core.LocalCache.Set(key, redisData)
				return cv.Data, nil
			}
		}
	}

	v, err, _ := m.sf.Do(key, func() (interface{}, error) {
		return loader()
	})
	if err != nil {
		return zero, err
	}
	dbData, ok := v.(T)
	if !ok {
		return zero, nil
	}

	setBothLevels(ctx, key, dbData, profile)
	return dbData, nil
}

func refreshAsync[T any](m *Manager, ctx context.Context, key string, loader func() (T, error), profile Profile) {
	go func() {
		bg, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _, _ = m.sf.Do("refresh:"+key, func() (interface{}, error) {
			data, err := loader()
			if err != nil {
				log.Printf("[cache] async refresh failed key=%s profile=%s err=%v", key, profile.Name, err)
				return nil, err
			}
			setBothLevels(bg, key, data, profile)
			return nil, nil
		})
	}()
}

func setBothLevels[T any](ctx context.Context, key string, data T, profile Profile) {
	cv := wrapValue(data, profile.L1TTL)
	raw, err := encodeValue(cv)
	if err != nil {
		return
	}
	_ = core.MyRedis.Set(ctx, key, raw, util.GetRandomExpire(profile.L2TTL)).Err()
	_ = core.LocalCache.Set(key, raw)
}

// Invalidate 删除 L1 + L2 缓存。
func Invalidate(keys ...string) {
	if Default == nil {
		InitManager()
	}
	ctx := context.Background()
	for _, key := range keys {
		core.LocalCache.Delete(key)
		core.MyRedis.Del(ctx, key)
		log.Printf("[cache] invalidated key=%s", key)
	}
}

// Set 直接写入两级缓存（预热场景）。
func Set[T any](ctx context.Context, key string, data T, profile Profile) error {
	setBothLevels(ctx, key, data, profile)
	return nil
}

var ready atomic.Bool

// MarkReady 标记预热完成，/ready 探针通过后才接流量。
func MarkReady() {
	ready.Store(true)
	log.Println("✅ 缓存预热完成，服务就绪")
}

// IsReady 是否已完成预热。
func IsReady() bool {
	return ready.Load()
}
