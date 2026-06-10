package service

import (
	"context"

	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"

	"gorm.io/gorm"
)

type commentService struct {
	comments repository.CommentRepository
}

func NewCommentService(comments repository.CommentRepository) CommentService {
	return &commentService{comments: comments}
}

func (s *commentService) ListByPost(ctx context.Context, postID int, postType string, limit, offset int) ([]*domain.Comment, int64, error) {
	return s.comments.ListByPostID(ctx, postID, postType, limit, offset)
}

type rankService struct {
	db *gorm.DB
}

func NewRankService(db *gorm.DB) RankService {
	return &rankService{db: db}
}

func (s *rankService) AdjustUserReceivedLike(_ context.Context, authorUserID, delta int) {
	AdjustUserReceivedLike(s.db, authorUserID, delta)
}

func (s *rankService) AdjustUserReceivedVote(_ context.Context, authorUserID, delta int) {
	AdjustUserReceivedVote(s.db, authorUserID, delta)
}

func (s *rankService) AdjustPostVoteNum(_ context.Context, postID, delta int) {
	AdjustPostVoteNum(s.db, postID, delta)
}

func (s *rankService) OnPostDeleted(_ context.Context, post *domain.Post) {
	if post == nil {
		return
	}
	OnPostDeleted(s.db, &model.Post{
		PostID:  post.ID,
		UserID:  post.UserID,
		LikeNum: post.LikeNum,
	})
}

func (s *rankService) RebuildRankStats(_ context.Context) {
	RebuildRankStats(s.db)
}

type chatService struct {
	chats repository.ChatRepository
}

func NewChatService(chats repository.ChatRepository) ChatService {
	return &chatService{chats: chats}
}

func (s *chatService) GetHistory(ctx context.Context, userID, peerID, limit, offset int) ([]*repository.ChatMessage, error) {
	return s.chats.ListHistory(ctx, userID, peerID, limit, offset)
}
