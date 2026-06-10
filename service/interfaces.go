package service

import (
	"context"

	"loginTest/domain"
	"loginTest/repository"
)

// PostService 帖子领域服务。
type PostService interface {
	CountBrowse(ctx context.Context, filter domain.BrowseFilter) (int64, error)
	FindByID(ctx context.Context, postID, viewerUserID int) (*domain.Post, error)
	IncrBrowse(ctx context.Context, postID int) int64
}

// UserService 用户领域服务。
type UserService interface {
	GetByID(ctx context.Context, id int) (*domain.User, error)
}

// CommentService 评论领域服务（Phase 2 骨架，待从 commentController 迁移）。
type CommentService interface {
	ListByPost(ctx context.Context, postID int, postType string, limit, offset int) ([]*domain.Comment, int64, error)
}

// RankService 排行统计服务。
type RankService interface {
	AdjustUserReceivedLike(ctx context.Context, authorUserID, delta int)
	AdjustUserReceivedVote(ctx context.Context, authorUserID, delta int)
	AdjustPostVoteNum(ctx context.Context, postID, delta int)
	OnPostDeleted(ctx context.Context, post *domain.Post)
	RebuildRankStats(ctx context.Context)
}

// ChatService 聊天领域服务（Phase 2 骨架，待从 chat.go 迁移）。
type ChatService interface {
	GetHistory(ctx context.Context, userID, peerID, limit, offset int) ([]*repository.ChatMessage, error)
}
