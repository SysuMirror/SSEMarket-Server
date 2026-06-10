package gormrepo

import (
	"context"

	"loginTest/repository"
)

type chatRepository struct{}

func NewChatRepository() repository.ChatRepository {
	return &chatRepository{}
}

func (r *chatRepository) ListHistory(_ context.Context, _, _ int, _, _ int) ([]*repository.ChatMessage, error) {
	return nil, nil
}

func (r *chatRepository) Create(_ context.Context, _ *repository.ChatMessage) error {
	return nil
}

func (r *chatRepository) CountUnread(_ context.Context, _ int) (int64, error) {
	return 0, nil
}
