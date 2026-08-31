package util

import (
	"context"
	"loginTest/core"
	"time"

	"github.com/google/uuid"
)

var lockCtx = context.Background()

const unlockScript = `
if redis.call("get", KEYS[1]) == ARGV[1] then
  return redis.call("del", KEYS[1])
else
  return 0
end`

// Lock 基于 Redis 的互斥锁（SET NX EX + Lua 安全释放）。
//
// 用途：cache/manager 在缓存 miss 回源 DB 前，避免多进程同时加载同一 key。
// 当前 compose 为单实例部署，防击穿主靠 singleflight；本锁为多实例水平扩展预留。
type Lock struct {
	key    string
	value  string
	expire time.Duration
}

// NewLock 创建锁（key=锁名称）。
func NewLock(key string) *Lock {
	return &Lock{
		key:    key,
		value:  uuid.NewString(),
		expire: 5 * time.Second,
	}
}

// TryLock 尝试加锁（SET NX EX）。
func (l *Lock) TryLock() bool {
	if core.MyRedis == nil {
		return false
	}
	ok, err := core.MyRedis.SetNX(lockCtx, l.key, l.value, l.expire).Result()
	return err == nil && ok
}

// UnLock 仅当 value 匹配时删除，防止释放他人锁。
func (l *Lock) UnLock() {
	if core.MyRedis == nil {
		return
	}
	_ = core.MyRedis.Eval(lockCtx, unlockScript, []string{l.key}, l.value).Err()
}
