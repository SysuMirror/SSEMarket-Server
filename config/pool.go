package config

import (
	"time"

	"github.com/spf13/viper"
)

// intOrDefault 读取 viper 整型，≤0 时用 defaultVal。
func intOrDefault(key string, defaultVal int) int {
	if n := viper.GetInt(key); n > 0 {
		return n
	}
	return defaultVal
}

// durationOrDefault 读取 viper 时长，未配置或非正数时用 defaultVal。
func durationOrDefault(key string, defaultVal time.Duration) time.Duration {
	if !viper.IsSet(key) {
		return defaultVal
	}
	d := viper.GetDuration(key)
	if d <= 0 {
		return defaultVal
	}
	return d
}

// DatasourcePoolMaxOpen MySQL 连接池最大打开连接数。
func DatasourcePoolMaxOpen() int {
	return intOrDefault("datasource.pool.max_open", 150)
}

// DatasourcePoolMaxIdle MySQL 连接池最大空闲连接数。
func DatasourcePoolMaxIdle() int {
	return intOrDefault("datasource.pool.max_idle", 30)
}

// DatasourcePoolConnMaxLifetime MySQL 连接最大存活时间。
func DatasourcePoolConnMaxLifetime() time.Duration {
	return durationOrDefault("datasource.pool.conn_max_lifetime", 30*time.Minute)
}

// RedisPoolSize Redis 客户端连接池大小。
func RedisPoolSize() int {
	return intOrDefault("redis.pool.size", 100)
}

// RedisMinIdleConns Redis 最小空闲连接。
func RedisMinIdleConns() int {
	return intOrDefault("redis.pool.min_idle", 10)
}

// RedisDialTimeout Redis 建连超时。
func RedisDialTimeout() time.Duration {
	return durationOrDefault("redis.timeout.dial", 500*time.Millisecond)
}

// RedisReadTimeout Redis 读超时。
func RedisReadTimeout() time.Duration {
	return durationOrDefault("redis.timeout.read", 300*time.Millisecond)
}

// RedisWriteTimeout Redis 写超时。
func RedisWriteTimeout() time.Duration {
	return durationOrDefault("redis.timeout.write", 300*time.Millisecond)
}
