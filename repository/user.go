package repository

import (
	"context"

	"loginTest/domain"
)

// UserRepository 用户持久化接口。
type UserRepository interface {
	FindByID(ctx context.Context, id int) (*domain.User, error)
	FindByEmail(ctx context.Context, email string) (*domain.User, error)
	FindByPhone(ctx context.Context, phone string) (*domain.User, error)
}
