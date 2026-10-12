package service

import (
	"context"

	"loginTest/domain"
	"loginTest/repository"
)

type rankService struct {
	repo repository.RankRepository
}

func NewRankService(repo repository.RankRepository) RankService {
	return &rankService{repo: repo}
}

func (s *rankService) AdjustUserReceivedLike(ctx context.Context, authorUserID, delta int) {
	s.repo.AdjustUserReceivedLike(ctx, authorUserID, delta)
}

func (s *rankService) PostCoinRanking(ctx context.Context, limit int) ([]domain.PostCoinRankItem, error) {
	return s.repo.PostCoinRanking(ctx, limit)
}

func (s *rankService) UserCoinRanking(ctx context.Context, limit int) ([]domain.UserCoinRankItem, error) {
	return s.repo.UserCoinRanking(ctx, limit)
}

func (s *rankService) OnPostDeleted(ctx context.Context, post *domain.Post) {
	s.repo.OnPostDeleted(ctx, post)
}

func (s *rankService) RebuildRankStats(ctx context.Context) {
	s.repo.RebuildRankStats(ctx)
}
