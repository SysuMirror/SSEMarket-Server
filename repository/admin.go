package repository

import (
	"context"
	"time"

	"loginTest/domain"
)

// AdminRepository 管理员持久化接口。
type AdminRepository interface {
	FindByID(ctx context.Context, id int) (*domain.Admin, error)
	FindBySsoSub(ctx context.Context, sub string) (*domain.Admin, error)
	FindByEmail(ctx context.Context, email string) (*domain.Admin, error)
	List(ctx context.Context) ([]domain.Admin, error)
	CountActive(ctx context.Context) (int64, error)
	Create(ctx context.Context, admin *domain.Admin) error
	UpdateLogin(ctx context.Context, id int, name, email, ssoSub string, at time.Time) error
	SetActive(ctx context.Context, id int, active bool) error
	DeleteByID(ctx context.Context, id int) error
	FilterUsers(ctx context.Context, filter domain.AdminUserFilter) ([]domain.AdminUserItem, error)
	DeleteUserByName(ctx context.Context, name string) error
	CreateAdminPost(ctx context.Context, in domain.AdminPostInput) (int, error)
}
