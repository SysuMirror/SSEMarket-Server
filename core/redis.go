package core

import (
	"context"
	"fmt"
	"log"
	"time"

	"loginTest/config"

	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
)

var MyRedis *redis.Client

func RedisInit() *redis.Client {
	host := viper.GetString("redis.host")
	port := viper.GetString("redis.port")
	password := viper.GetString("redis.password")
	addr := fmt.Sprintf("%s:%s", host, port)

	poolSize := config.RedisPoolSize()
	minIdle := config.RedisMinIdleConns()
	dialTimeout := config.RedisDialTimeout()
	readTimeout := config.RedisReadTimeout()
	writeTimeout := config.RedisWriteTimeout()

	rds := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           0,
		PoolSize:     poolSize,
		MinIdleConns: minIdle,
		DialTimeout:  dialTimeout,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
	})

	// 3. 新增：Redis健康检查（启动即校验连接，避免运行时报错）
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_, err := rds.Ping(ctx).Result()
	if err != nil {
		panic(fmt.Sprintf("❌ Redis 连接失败：%v", err))
	}

	// 4. 赋值全局变量（保留原有逻辑）
	MyRedis = rds
	log.Printf("✅ Redis 初始化成功 addr=%s pool=%d min_idle=%d read=%s write=%s",
		addr, poolSize, minIdle, readTimeout, writeTimeout)
	return rds
}