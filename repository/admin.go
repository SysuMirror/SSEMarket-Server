package repository

import (
	"context"

	"loginTest/domain"
)

// AdminRepository 管理员持久化接口。
type AdminRepository interface {
	FindByAccount(ctx context.Context, account string) (*domain.Admin, error)
	Count(ctx context.Context) (int64, error)
	Create(ctx context.Context, admin *domain.Admin) error
	Save(ctx context.Context, admin *domain.Admin) error
	DeleteByAccount(ctx context.Context, account string) error
	FilterUsers(ctx context.Context, filter domain.AdminUserFilter) ([]domain.AdminUserItem, error)
	DeleteUserByName(ctx context.Context, name string) error
	CreateAdminPost(ctx context.Context, in domain.AdminPostInput) (int, error)
}
