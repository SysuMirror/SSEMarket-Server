package repository

import (
	"context"

	"loginTest/domain"
)

// RatingRepository 评分专区持久化接口。
type RatingRepository interface {
	ListCategories(ctx context.Context) ([]*domain.RatingCategory, error)
	FindCategoryByName(ctx context.Context, name string) (*domain.RatingCategory, error)
	FindCategoryByID(ctx context.Context, id int) (*domain.RatingCategory, error)
	MaxCategorySortOrder(ctx context.Context) (int, error)
	CreateCategory(ctx context.Context, cat *domain.RatingCategory) error
	CreateCategoryWithFeedPost(ctx context.Context, cat *domain.RatingCategory, user *domain.User, feedTitle string) error

	ListPosts(ctx context.Context, filter domain.RatingBrowseFilter) ([]*domain.RatingPost, error)
	CountPosts(ctx context.Context, filter domain.RatingBrowseFilter) (int64, error)
	FindPostByID(ctx context.Context, id int) (*domain.RatingPost, error)
	CreatePost(ctx context.Context, post *domain.RatingPost) error
	IncrCategoryPostCount(ctx context.Context, categoryID, delta int) error
	IncrPostBrowse(ctx context.Context, postID int) error
	DeletePostCascade(ctx context.Context, postID, categoryID int) error

	FindPostLike(ctx context.Context, postID, userID int) (int, error)
	TogglePostLike(ctx context.Context, postID, userID int) error

	FindUserStar(ctx context.Context, postID, userID int) (int, error)
	UpsertStar(ctx context.Context, postID, userID, starNum int) error

	ListCommentsByPost(ctx context.Context, postID int) ([]*domain.RatingComment, error)
	FindCommentByID(ctx context.Context, id int) (*domain.RatingComment, error)
	CreateComment(ctx context.Context, c *domain.RatingComment) error
	IncrPostCommentNum(ctx context.Context, postID int) error
	ToggleCommentLike(ctx context.Context, commentID, userID int) error

	CategoriesByIDs(ctx context.Context, ids []int) (map[int]*domain.RatingCategory, error)
	UsersByIDs(ctx context.Context, ids []int) (map[int]*domain.User, error)
	PostLikedSet(ctx context.Context, userID int, postIDs []int) (map[int]struct{}, error)
	StarsByPostIDs(ctx context.Context, postIDs []int) (map[int][]int, error)
	CommentLikedSet(ctx context.Context, userID int, commentIDs []int) (map[int]bool, error)
	CategoryIDsByNameLike(ctx context.Context, pattern string) ([]int, error)

	CreateNotice(ctx context.Context, receiver, sender int, noticeType, text string, target int) error
	FindUserByID(ctx context.Context, id int) (*domain.User, error)
}
