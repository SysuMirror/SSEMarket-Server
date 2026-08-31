package observability

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// StartCollectors 启动后台指标采集（Redis 连接池、缓存命中率）。
func StartCollectors(ctx context.Context, rds *redis.Client) {
	interval := 15 * time.Second
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				RefreshCacheHitRatio()
				collectRedisPoolStats(rds)
			}
		}
	}()
}

func collectRedisPoolStats(rds *redis.Client) {
	if rds == nil {
		return
	}
	stats := rds.PoolStats()
	RedisPoolStats.WithLabelValues("hits").Set(float64(stats.Hits))
	RedisPoolStats.WithLabelValues("misses").Set(float64(stats.Misses))
	RedisPoolStats.WithLabelValues("total_conns").Set(float64(stats.TotalConns))
	RedisPoolStats.WithLabelValues("idle_conns").Set(float64(stats.IdleConns))
	RedisPoolStats.WithLabelValues("stale_conns").Set(float64(stats.StaleConns))
}
