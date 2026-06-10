package service

import (
	"context"

	"loginTest/domain"
	"loginTest/repository"
)

type postService struct {
	posts repository.PostRepository
}

func NewPostService(posts repository.PostRepository) PostService {
	return &postService{posts: posts}
}

func (s *postService) CountBrowse(ctx context.Context, filter domain.BrowseFilter) (int64, error) {
	return s.posts.Count(ctx, filter)
}

func (s *postService) FindByID(ctx context.Context, postID, viewerUserID int) (*domain.Post, error) {
	post, err := s.posts.FindByID(ctx, postID)
	if err != nil {
		return nil, err
	}
	if post.IsPrivate && post.UserID != viewerUserID {
		return nil, domain.ErrNotFound
	}
	return post, nil
}

func (s *postService) IncrBrowse(ctx context.Context, postID int) int64 {
	if HotData == nil {
		return 0
	}
	return HotData.IncrBrowseCount(postID)
}
