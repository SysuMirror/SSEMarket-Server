package repository

import (
	"context"

	"loginTest/domain"
)

// PostRepository 帖子持久化接口，所有方法带 context 以支持超时与取消。
type PostRepository interface {
	FindByID(ctx context.Context, id int) (*domain.Post, error)
	List(ctx context.Context, filter domain.BrowseFilter) ([]*domain.Post, int64, error)
	Count(ctx context.Context, filter domain.BrowseFilter) (int64, error)
	UpdateBrowseNum(ctx context.Context, id int, browseNum int64) error
}
