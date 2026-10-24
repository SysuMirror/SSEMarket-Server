package repository

import (
	"context"

	"loginTest/domain"
)

// UserSkinRepository 用户自定义皮肤与皮肤库持久化接口。
type UserSkinRepository interface {
	// Create 新建皮肤并把皮肤加入作者自己的皮肤库（同一事务）。
	Create(ctx context.Context, in domain.UserSkinInput) (*domain.UserSkin, error)
	// ListLibrary 用户皮肤库（自己上传 + 添加别人的）。
	ListLibrary(ctx context.Context, userID int) ([]domain.UserSkin, error)
	// SearchPublic 搜索已公开的皮肤；keyword 匹配皮肤名/作者名。
	SearchPublic(ctx context.Context, userID int, keyword string, limit int) ([]domain.UserSkin, error)
	FindByID(ctx context.Context, skinID int) (*domain.UserSkin, error)
	FindByShareToken(ctx context.Context, token string) (*domain.UserSkin, error)
	// FindForUser 取「在当前用户皮肤库中」的皮肤；用于应用自定义皮肤时校验。
	FindForUser(ctx context.Context, userID, skinID int) (*domain.UserSkin, error)
	// AddToLibrary 幂等加入皮肤库。
	AddToLibrary(ctx context.Context, userID, skinID int) error
	// RemoveFromLibrary 从皮肤库移除，并把仍是该皮肤的背景设置回退为 0。
	RemoveFromLibrary(ctx context.Context, userID, skinID int) error
	// SetPublic 作者设置公开/私密；返回更新后的皮肤。
	SetPublic(ctx context.Context, skinID, ownerID int, public bool) (*domain.UserSkin, error)
	// Delete 作者删除皮肤：清理皮肤库记录，并把正在使用它的用户背景回退。
	Delete(ctx context.Context, skinID, ownerID int) error
}
