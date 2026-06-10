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

func (s *rankService) AdjustUserReceivedVote(ctx context.Context, authorUserID, delta int) {
	s.repo.AdjustUserReceivedVote(ctx, authorUserID, delta)
}

func (s *rankService) AdjustPostVoteNum(ctx context.Context, postID, delta int) {
	s.repo.AdjustPostVoteNum(ctx, postID, delta)
}

func (s *rankService) OnPostDeleted(ctx context.Context, post *domain.Post) {
	s.repo.OnPostDeleted(ctx, post)
}

func (s *rankService) RebuildRankStats(ctx context.Context) {
	s.repo.RebuildRankStats(ctx)
}
