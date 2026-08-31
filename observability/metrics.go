package observability

import (
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency in seconds, partitioned by route.",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	}, []string{"method", "route", "status"})

	CacheHitRatio = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cache_hit_ratio",
		Help: "Cache hit ratio for L1 and L2 levels.",
	}, []string{"level", "profile"})

	DBQueryDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "db_query_duration_seconds",
		Help:    "Database query duration in seconds.",
		Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
	}, []string{"operation"})

	RedisPoolStats = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "redis_pool_stats",
		Help: "Redis connection pool statistics.",
	}, []string{"stat"})

	BrowseSyncLag = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "browse_sync_lag_seconds",
		Help: "Lag in seconds between Redis browse count update and MySQL sync.",
	})
)

type cacheCounters struct {
	l1Hits   atomic.Uint64
	l1Misses atomic.Uint64
	l2Hits   atomic.Uint64
	l2Misses atomic.Uint64
}

var cacheStats cacheCounters

// RecordCacheAccess 记录缓存访问结果，用于计算 hit ratio。
func RecordCacheAccess(level, profile string, hit bool) {
	switch level {
	case "l1":
		if hit {
			cacheStats.l1Hits.Add(1)
		} else {
			cacheStats.l1Misses.Add(1)
		}
	case "l2":
		if hit {
			cacheStats.l2Hits.Add(1)
		} else {
			cacheStats.l2Misses.Add(1)
		}
	}
	_ = level
	_ = profile
}

// RefreshCacheHitRatio 根据累计计数刷新 cache_hit_ratio Gauge。
func RefreshCacheHitRatio() {
	updateRatio("l1", "all", cacheStats.l1Hits.Load(), cacheStats.l1Misses.Load())
	updateRatio("l2", "all", cacheStats.l2Hits.Load(), cacheStats.l2Misses.Load())
}

func updateRatio(level, profile string, hits, misses uint64) {
	total := hits + misses
	if total == 0 {
		CacheHitRatio.WithLabelValues(level, profile).Set(0)
		return
	}
	CacheHitRatio.WithLabelValues(level, profile).Set(float64(hits) / float64(total))
}
