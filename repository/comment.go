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
	// CreatePcommentAndAdjustPost 在同一事务内创建一级评论并更新帖子评论数/热度。
	CreatePcommentAndAdjustPost(ctx context.Context, c *domain.Pcomment, commentDelta int, heatDelta float64) error
	// CreateCcommentAndAdjustPost 在同一事务内创建楼中楼并更新帖子评论数/热度。
	CreateCcommentAndAdjustPost(ctx context.Context, c *domain.Ccomment, postID, commentDelta int, heatDelta float64) error
	// DeletePcommentAndAdjustPost 在同一事务内删除一级评论并回退帖子评论数/热度。
	DeletePcommentAndAdjustPost(ctx context.Context, id, postID, commentDelta int, heatDelta float64) error
	// DeleteCcommentAndAdjustPost 在同一事务内删除楼中楼并回退帖子评论数/热度。
	DeleteCcommentAndAdjustPost(ctx context.Context, id, postID, commentDelta int, heatDelta float64) error

	PcommentLikedSet(ctx context.Context, userID int, pcommentIDs []int) (map[int]bool, error)
	PcommentDeniedSet(ctx context.Context, userID int, pcommentIDs []int) (map[int]bool, error)
	CcommentLikedSet(ctx context.Context, userID int, ccommentIDs []int) (map[int]bool, error)
	CcommentDeniedSet(ctx context.Context, userID int, ccommentIDs []int) (map[int]bool, error)
	CcommentReaction(ctx context.Context, userID, ccommentID int) (liked, denied bool, err error)

	FindPcommentLike(ctx context.Context, userID, pcommentID int) (bool, error)
	FindPcommentDeny(ctx context.Context, userID, pcommentID int) (bool, error)

	FindCcommentLike(ctx context.Context, userID, ccommentID int) (bool, error)
	FindCcommentDeny(ctx context.Context, userID, ccommentID int) (bool, error)
	// TogglePcommentLikeOnce 在同一事务内切换一级评论点赞：已赞则取消，已踩则改为赞。
	TogglePcommentLikeOnce(ctx context.Context, userID, pcommentID int) error
	// TogglePcommentDenyOnce 在同一事务内切换一级评论点踩：已踩则取消，已赞则改为踩。
	TogglePcommentDenyOnce(ctx context.Context, userID, pcommentID int) error
	// ToggleCcommentLikeOnce 在同一事务内切换楼中楼点赞：已赞则取消，已踩则改为赞。
	ToggleCcommentLikeOnce(ctx context.Context, userID, ccommentID int) error
	// ToggleCcommentDenyOnce 在同一事务内切换楼中楼点踩：已踩则取消，已赞则改为踩。
	ToggleCcommentDenyOnce(ctx context.Context, userID, ccommentID int) error

	UsersByIDs(ctx context.Context, ids []int) (map[int]*domain.User, error)
	FindUserByPhone(ctx context.Context, phone string) (*domain.User, error)
	FindUserByID(ctx context.Context, id int) (*domain.User, error)
	FindUsersByExactNames(ctx context.Context, names []string) ([]*domain.User, error)

	FindPostByID(ctx context.Context, id int) (*domain.Post, error)
	IncrPostCommentNum(ctx context.Context, postID, delta int) error
	AdjustPostHeat(ctx context.Context, postID int, delta float64) error

	CreateNotice(ctx context.Context, receiver, sender int, noticeType, text string, target int) error
}
