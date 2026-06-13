package service

import (
	"context"
	"errors"

	"loginTest/domain"
	"loginTest/repository"
)

type voteService struct {
	votes repository.VoteRepository
	posts repository.PostRepository
	ranks RankService
}

func NewVoteService(votes repository.VoteRepository, posts repository.PostRepository, ranks RankService) VoteService {
	return &voteService{votes: votes, posts: posts, ranks: ranks}
}

func (s *voteService) SearchByTitle(ctx context.Context, keyword string, viewerUserID int) ([]domain.VoteSearchItem, error) {
	return s.votes.SearchByTitle(ctx, keyword, viewerUserID)
}

func (s *voteService) SubmitVote(ctx context.Context, userID, postID int) error {
	post, err := s.posts.FindByID(ctx, postID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound
		}
		return err
	}
	if !canViewPost(post, userID) {
		return domain.ErrNotFound
	}
	voted, err := s.votes.HasVoted(ctx, userID, postID)
	if err != nil {
		return err
	}
	if voted {
		return domain.ErrConflict
	}
	if err := s.votes.SubmitVote(ctx, userID, postID); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return domain.ErrConflict
		}
		return err
	}
	s.ranks.AdjustPostVoteNum(ctx, postID, 1)
	s.ranks.AdjustUserReceivedVote(ctx, post.UserID, 1)
	return nil
}

func (s *voteService) HasVoted(ctx context.Context, userID, postID int) (bool, error) {
	post, err := s.posts.FindByID(ctx, postID)
	if err != nil {
		return false, err
	}
	if !canViewPost(post, userID) {
		return false, domain.ErrNotFound
	}
	return s.votes.HasVoted(ctx, userID, postID)
}

func (s *voteService) GetPostRanking(ctx context.Context) ([]domain.PostRankItem, error) {
	return s.votes.GetPostRanking(ctx)
}

func (s *voteService) GetUserRanking(ctx context.Context) ([]domain.UserRankItem, error) {
	return s.votes.GetUserRanking(ctx)
}
