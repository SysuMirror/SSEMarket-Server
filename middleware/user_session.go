package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"loginTest/core"
	"loginTest/domain"
	"loginTest/model"
	"loginTest/service"
	"loginTest/util"
)

const (
	userSessionTTL = 5 * time.Minute
)

func userSessionKey(userID int) string {
	return util.PrefixUserSession + strconv.Itoa(userID)
}

func domainUserToModel(u *domain.User) model.User {
	if u == nil {
		return model.User{}
	}
	return model.User{
		UserID:                     u.ID,
		Phone:                      u.Phone,
		Email:                      u.Email,
		Name:                       u.Name,
		Intro:                      u.Intro,
		AvatarURL:                  u.AvatarURL,
		Identity:                   u.Identity,
		Score:                      u.Score,
		Banend:                     u.BanEnd,
		Punishnum:                  u.PunishNum,
		HideProfilePosts:           u.HideProfilePosts,
		ISEmailNotificationBlocked: u.ISEmailNotificationBlocked,
	}
}

// LoadUserSession 从 Redis 读取用户快照，miss 时回源并回填（TTL 5min）。
func LoadUserSession(ctx context.Context, userID int) (model.User, error) {
	if userID == 0 {
		return model.User{}, fmt.Errorf("invalid user id")
	}
	key := userSessionKey(userID)
	if core.MyRedis != nil {
		data, err := core.MyRedis.Get(ctx, key).Bytes()
		if err == nil {
			var user model.User
			if json.Unmarshal(data, &user) == nil && user.UserID != 0 {
				return user, nil
			}
		}
	}
	if service.Users == nil {
		return model.User{}, fmt.Errorf("user service unavailable")
	}
	u, err := service.Users.GetByID(ctx, userID)
	if err != nil {
		return model.User{}, err
	}
	user := domainUserToModel(u)
	_ = StoreUserSession(ctx, user)
	return user, nil
}

// StoreUserSession 写入 Redis 用户快照。
func StoreUserSession(ctx context.Context, user model.User) error {
	if core.MyRedis == nil || user.UserID == 0 {
		return nil
	}
	data, err := json.Marshal(user)
	if err != nil {
		return err
	}
	return core.MyRedis.Set(ctx, userSessionKey(user.UserID), data, userSessionTTL).Err()
}

// InvalidateUserSession 使缓存失效。
func InvalidateUserSession(ctx context.Context, userID int) {
	if core.MyRedis == nil || userID == 0 {
		return
	}
	_ = core.MyRedis.Del(ctx, userSessionKey(userID)).Err()
}
