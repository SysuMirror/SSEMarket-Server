package repository

import (
	"context"

	"loginTest/domain"
)

// SsoBindingRepository UniSSO 绑定关系持久化。
type SsoBindingRepository interface {
	// FindBySub 按 UniSSO sub 查绑定；未命中返回 domain.ErrNotFound。
	FindBySub(ctx context.Context, sub string) (*domain.SsoBinding, error)
	// FindByUserID 按集市用户ID查绑定；未命中返回 domain.ErrNotFound。
	FindByUserID(ctx context.Context, userID int) (*domain.SsoBinding, error)
	// Create 写入绑定；唯一索引冲突(sub 或 userID 已绑定)返回 domain.ErrConflict。
	Create(ctx context.Context, b *domain.SsoBinding) error
	// DeleteByUserID 删除该用户的绑定；无绑定记录返回 domain.ErrNotFound。
	DeleteByUserID(ctx context.Context, userID int) error
}
