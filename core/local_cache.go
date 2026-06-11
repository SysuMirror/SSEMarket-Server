package core

import (
	"fmt"
	"time"

	"github.com/allegro/bigcache/v3"
)

// LocalCache L1 本地缓存（全项目共用）。
var LocalCache *bigcache.BigCache

// InitLocalCache 初始化本地缓存（生产级：5min 窗口、1024 分片、512MB 上限）。
func InitLocalCache() {
	config := bigcache.Config{
		Shards:             1024,
		LifeWindow:         5 * time.Minute,
		CleanWindow:        1 * time.Minute,
		MaxEntriesInWindow: 1000 * 1000,
		HardMaxCacheSize:   512,
		Verbose:            false,
	}

	cache, err := bigcache.NewBigCache(config)
	if err != nil {
		panic(fmt.Sprintf("本地缓存初始化失败: %v", err))
	}

	LocalCache = cache
	fmt.Println("✅ L1本地缓存(BigCache)初始化成功")
}
