package repository

import (
	"context"

	"loginTest/domain"
)

// SueRepository 举报持久化接口。
type SueRepository interface {
	ListPending(ctx context.Context) ([]*domain.Sue, error)
	FindByID(ctx context.Context, id int) (*domain.Sue, error)
	MarkFinishedByID(ctx context.Context, id int, status string) error
	MarkFinishedByTarget(ctx context.Context, targetType string, targetID int, status string) error
	LoadTargetSummary(ctx context.Context, targetType string, targetID int) (title, detail string, ok bool)
	ProcessViolation(ctx context.Context, sue *domain.Sue) (*domain.ViolationResult, error)
	CreateNotice(ctx context.Context, receiver int, noticeType, text string, target int) error
	PunishUser(ctx context.Context, userID int) (*domain.User, error)
	Create(ctx context.Context, sue *domain.Sue) error
}
