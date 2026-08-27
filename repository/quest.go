package repository

import (
	"context"

	"loginTest/domain"
)

// QuestRepository Quest 专用读写（不改动既有 PostRepository 行为）。
type QuestRepository interface {
	List(ctx context.Context, filter domain.QuestListFilter) ([]*domain.Post, int64, error)
	GetQuestPost(ctx context.Context, postID int) (*domain.Post, error)
	CreateQuestPost(ctx context.Context, in domain.QuestPostCreateInput) (*domain.Post, error)
	UpdateQuestStatus(ctx context.Context, postID int, status string) error
	UpdateQuestStatusByAuthor(ctx context.Context, postID, userID int, status string) error

	GetOrderByPostID(ctx context.Context, postID int) (*domain.QuestOrder, error)
	CreateOrder(ctx context.Context, order *domain.QuestOrder) error
	UpdateOrder(ctx context.Context, order *domain.QuestOrder) error
}
