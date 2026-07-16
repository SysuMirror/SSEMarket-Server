package repository

import (
	"context"

	"loginTest/domain"
)

// VoteRepository 投票持久化接口。
type VoteRepository interface {
	SearchByTitle(ctx context.Context, keyword string, viewerUserID int) ([]domain.VoteSearchItem, error)
	SubmitVote(ctx context.Context, userID, postID int) error
	CancelVote(ctx context.Context, userID, postID int) error
	HasVoted(ctx context.Context, userID, postID int) (bool, error)
	GetPostRanking(ctx context.Context) ([]domain.PostRankItem, error)
	GetUserRanking(ctx context.Context) ([]domain.UserRankItem, error)
}
