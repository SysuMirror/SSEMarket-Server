package repository

import (
	"context"

	"loginTest/domain"
)

// CommentRepository 评论持久化接口。
type CommentRepository interface {
	ListPcommentsByPostID(ctx context.Context, postID int) ([]*domain.Pcomment, error)
	FindPcommentByID(ctx context.Context, id int) (*domain.Pcomment, error)
	CreatePcomment(ctx context.Context, c *domain.Pcomment) error
	DeletePcomment(ctx context.Context, id int) error
	CountCcommentsByPcommentID(ctx context.Context, pcommentID int) (int64, error)

	ListCcommentsByPcommentID(ctx context.Context, pcommentID int) ([]*domain.Ccomment, error)
	FindCcommentByID(ctx context.Context, id int) (*domain.Ccomment, error)
	CreateCcomment(ctx context.Context, c *domain.Ccomment) error
	DeleteCcomment(ctx context.Context, id int) error

	PcommentLikedSet(ctx context.Context, userID int, pcommentIDs []int) (map[int]bool, error)
	PcommentDeniedSet(ctx context.Context, userID int, pcommentIDs []int) (map[int]bool, error)
	CcommentLikedSet(ctx context.Context, userID int, ccommentIDs []int) (map[int]bool, error)
	CcommentDeniedSet(ctx context.Context, userID int, ccommentIDs []int) (map[int]bool, error)
	CcommentReaction(ctx context.Context, userID, ccommentID int) (liked, denied bool, err error)

	FindPcommentLike(ctx context.Context, userID, pcommentID int) (bool, error)
	FindPcommentDeny(ctx context.Context, userID, pcommentID int) (bool, error)
	CreatePcommentLike(ctx context.Context, userID, pcommentID int) error
	DeletePcommentLike(ctx context.Context, userID, pcommentID int) error
	CreatePcommentDeny(ctx context.Context, userID, pcommentID int) error
	DeletePcommentDeny(ctx context.Context, userID, pcommentID int) error
	IncrPcommentLikeNum(ctx context.Context, pcommentID, delta int) error
	IncrPcommentDenyNum(ctx context.Context, pcommentID, delta int) error

	FindCcommentLike(ctx context.Context, userID, ccommentID int) (bool, error)
	FindCcommentDeny(ctx context.Context, userID, ccommentID int) (bool, error)
	CreateCcommentLike(ctx context.Context, userID, ccommentID int) error
	DeleteCcommentLike(ctx context.Context, userID, ccommentID int) error
	CreateCcommentDeny(ctx context.Context, userID, ccommentID int) error
	DeleteCcommentDeny(ctx context.Context, userID, ccommentID int) error
	IncrCcommentLikeNum(ctx context.Context, ccommentID, delta int) error
	IncrCcommentDenyNum(ctx context.Context, ccommentID, delta int) error

	UsersByIDs(ctx context.Context, ids []int) (map[int]*domain.User, error)
	FindUserByPhone(ctx context.Context, phone string) (*domain.User, error)
	FindUserByID(ctx context.Context, id int) (*domain.User, error)
	FindUsersByExactNames(ctx context.Context, names []string) ([]*domain.User, error)

	FindPostByID(ctx context.Context, id int) (*domain.Post, error)
	IncrPostCommentNum(ctx context.Context, postID, delta int) error
	AdjustPostHeat(ctx context.Context, postID int, delta float64) error

	CreateNotice(ctx context.Context, receiver, sender int, noticeType, text string, target int) error
}
