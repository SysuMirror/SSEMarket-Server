package gormrepo

import (
	"context"

	"loginTest/domain"
	"loginTest/repository"
)

type commentRepository struct{}

func NewCommentRepository() repository.CommentRepository {
	return &commentRepository{}
}

func (r *commentRepository) ListByPostID(_ context.Context, _ int, _ string, _, _ int) ([]*domain.Comment, int64, error) {
	return nil, 0, nil
}

func (r *commentRepository) FindByID(_ context.Context, _ int) (*domain.Comment, error) {
	return nil, domain.ErrNotFound
}

func (r *commentRepository) Create(_ context.Context, _ *domain.Comment) error {
	return nil
}

func (r *commentRepository) Delete(_ context.Context, _ int) error {
	return nil
}
