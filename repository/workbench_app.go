package repository

import (
	"context"

	"loginTest/domain"
)

// WorkbenchAppRepository 工作台应用持久化接口。
type WorkbenchAppRepository interface {
	FindByName(ctx context.Context, name string) (*domain.WorkbenchApp, error)
	MaxSortOrder(ctx context.Context) (int, error)
	Create(ctx context.Context, app *domain.WorkbenchApp) error
	ListVisible(ctx context.Context) ([]*domain.WorkbenchApp, error)
	ListAll(ctx context.Context) ([]*domain.WorkbenchApp, error)
}
