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
