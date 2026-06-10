package repository

import (
	"context"

	"loginTest/domain"
)

// CommentRepository 评论持久化接口。
type CommentRepository interface {
	ListByPostID(ctx context.Context, postID int, postType string, limit, offset int) ([]*domain.Comment, int64, error)
	FindByID(ctx context.Context, id int) (*domain.Comment, error)
	Create(ctx context.Context, c *domain.Comment) error
	Delete(ctx context.Context, id int) error
}
