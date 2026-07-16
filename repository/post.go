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
	IsWatched(ctx context.Context, userID, postID int) (bool, error)
	CreateWatch(ctx context.Context, userID, postID int) error
	DeleteWatch(ctx context.Context, userID, postID int) error
	ListWatcherUserIDsByPostID(ctx context.Context, postID int) ([]int, error)
	Create(ctx context.Context, post *domain.Post) error
	Delete(ctx context.Context, postID int) error
	UpdatePrivacy(ctx context.Context, postID int, isPrivate bool) error
	UpdateLikeNum(ctx context.Context, postID int, delta int) error
	UpdateHeat(ctx context.Context, postID int, delta float64) error
	HasBrowsed(ctx context.Context, userID, postID int) (bool, error)
	CreateBrowse(ctx context.Context, userID, postID int) error
	DeleteSave(ctx context.Context, userID, postID int) error
	CreateSave(ctx context.Context, userID, postID int) error
	DeleteLike(ctx context.Context, userID, postID int) error
	CreateLike(ctx context.Context, userID, postID int) error
	ListUserProfile(ctx context.Context, targetUserID, viewerUserID, limit, offset int) ([]*domain.Post, error)
	CountUserProfile(ctx context.Context, targetUserID, viewerUserID int) (int64, error)
	FindPublicByID(ctx context.Context, postID int) (*domain.Post, error)
	ListAllTags(ctx context.Context) ([]*domain.Tag, error)
	FindTagByName(ctx context.Context, name string) (*domain.Tag, error)
	CreatePostNotice(ctx context.Context, receiver, sender int, noticeType, text string, target int) error
	CreateFeedback(ctx context.Context, text, attachment string) (*domain.Feedback, error)
	ListAllFeedback(ctx context.Context) ([]*domain.Feedback, error)
	// ListViewerPrivatePostsRecent 当前用户最近私密帖（首页合并用）。
	ListViewerPrivatePostsRecent(ctx context.Context, userID, limit int) ([]*domain.Post, error)
}
