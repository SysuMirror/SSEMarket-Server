package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"loginTest/core"
	"loginTest/domain"
	"loginTest/util"
)

const userSessionTTL = 5 * time.Minute

func userSessionKey(userID int) string {
	return util.PrefixUserSession + strconv.Itoa(userID)
}

// LoadUserSession 从 Redis 读取用户快照，miss 时回源 DB 并回填（TTL 5min）。
func LoadUserSession(ctx context.Context, userID int) (*domain.User, error) {
	if userID == 0 {
		return nil, fmt.Errorf("invalid user id")
	}
	if core.MyRedis != nil {
		data, err := core.MyRedis.Get(ctx, userSessionKey(userID)).Bytes()
		if err == nil {
			var user domain.User
			if json.Unmarshal(data, &user) == nil && user.ID != 0 {
				return &user, nil
			}
		}
	}
	if Users == nil {
		return nil, fmt.Errorf("user service unavailable")
	}
	u, err := Users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	_ = StoreUserSession(ctx, u)
	return u, nil
}

// StoreUserSession 写入 Redis 用户快照。
func StoreUserSession(ctx context.Context, user *domain.User) error {
	if core.MyRedis == nil || user.ID == 0 {
		return nil
	}
	data, err := json.Marshal(user)
	if err != nil {
		return err
	}
	return core.MyRedis.Set(ctx, userSessionKey(user.ID), data, userSessionTTL).Err()
}

// InvalidateUserSession 使用户 session 缓存失效。
func InvalidateUserSession(ctx context.Context, userID int) {
	if core.MyRedis == nil || userID == 0 {
		return
	}
	_ = core.MyRedis.Del(ctx, userSessionKey(userID)).Err()
}

// CacheUserAfterAuth 登录/注册成功后预热 session 缓存。
func CacheUserAfterAuth(ctx context.Context, user *domain.User) {
	_ = StoreUserSession(ctx, user)
}
