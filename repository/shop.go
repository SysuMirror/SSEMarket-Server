package repository

import (
	"context"

	"loginTest/domain"
)

// ShopRepository 商城商品持久化接口。
type ShopRepository interface {
	Create(ctx context.Context, product *domain.Product) error
	CreateWithFeedPost(ctx context.Context, product *domain.Product, user *domain.User, feedTitle string) error
	FindByID(ctx context.Context, id int) (*domain.Product, error)
	FindRawByID(ctx context.Context, id int) (*domain.Product, error)
	List(ctx context.Context, filter domain.ProductFilter) ([]*domain.Product, error)
	Count(ctx context.Context, filter domain.ProductFilter) (int64, error)
	SoftDelete(ctx context.Context, id int) error
	MarkSold(ctx context.Context, id int) error
	ListCarouselCandidates(ctx context.Context, limit int) ([]*domain.Product, error)
}
