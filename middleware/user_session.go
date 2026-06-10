package middleware

import (
	"context"

	"loginTest/domain"
	"loginTest/service"
)

// LoadUserSession 委托 Service 层 session 缓存。
func LoadUserSession(ctx context.Context, userID int) (*domain.User, error) {
	return service.LoadUserSession(ctx, userID)
}

// StoreUserSession 委托 Service 层 session 缓存。
func StoreUserSession(ctx context.Context, user *domain.User) error {
	return service.StoreUserSession(ctx, user)
}

// InvalidateUserSession 委托 Service 层 session 缓存。
func InvalidateUserSession(ctx context.Context, userID int) {
	service.InvalidateUserSession(ctx, userID)
}

// RefreshUserSession 登录成功后写入 session 缓存。
func RefreshUserSession(ctx context.Context, user *domain.User) {
	service.CacheUserAfterAuth(ctx, user)
}
