package repository

import (
	"context"

	"loginTest/domain"
)

// ResumeRepository 简历持久化接口。
type ResumeRepository interface {
	CountByUser(ctx context.Context, userID int) (int64, error)
	Create(ctx context.Context, resume *domain.Resume) error
	List(ctx context.Context, filter domain.ResumeFilter) ([]*domain.Resume, error)
	FindByID(ctx context.Context, id int) (*domain.Resume, error)
	IncrViewNum(ctx context.Context, id int) error
	IncrDownloadNum(ctx context.Context, id int) error
	UpdateMeta(ctx context.Context, id int, title, description string) error
	SoftDelete(ctx context.Context, id int) error
	UsersByIDs(ctx context.Context, ids []int) (map[int]*domain.User, error)
}
