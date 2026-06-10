package repository

import (
	"context"

	"loginTest/domain"
	"loginTest/dto"
)

// PostRepository 帖子持久化接口，所有方法带 context 以支持超时与取消。
type PostRepository interface {
	FindByID(ctx context.Context, id int) (*domain.Post, error)
	List(ctx context.Context, filter domain.BrowseFilter) ([]*domain.Post, int64, error)
	ListBrowse(ctx context.Context, filter domain.BrowseFilter) ([]*domain.Post, error)
	Count(ctx context.Context, filter domain.BrowseFilter) (int64, error)
	UpdateBrowseNum(ctx context.Context, id int, browseNum int64) error
	LoadDetailDTO(ctx context.Context, postID int) (*dto.PostDetailCacheDTO, error)
	LikedSet(ctx context.Context, userID int, postIDs []int) (map[int]struct{}, error)
	SavedSet(ctx context.Context, userID int, postIDs []int) (map[int]struct{}, error)
	AuthorsByIDs(ctx context.Context, userIDs []int) (map[int]*domain.User, error)
	IsLiked(ctx context.Context, userID, postID int) (bool, error)
	IsSaved(ctx context.Context, userID, postID int) (bool, error)
}
