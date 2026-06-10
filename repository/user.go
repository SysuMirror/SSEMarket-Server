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
	FindAuthByEmail(ctx context.Context, email string) (*domain.User, string, error)
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	ExistsByPhone(ctx context.Context, phone string) (bool, error)
	Create(ctx context.Context, name, phone, email, hashedPassword string) (*domain.User, error)
	SaveRefreshToken(ctx context.Context, userID int, token string, expiresAt int64) error
	UseCDKey(ctx context.Context, content string) error
}
